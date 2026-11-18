// Package contract holds the parity and contract tests of the agent surfaces
// (SDD-003 section 7, NFR-025): one scenario is driven through the in-process
// Service, the HTTP API, the CLI and the MCP tools, and the results, the errors
// and the final project document must be the same.
package contract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/contract/testenv"
)

// loadScenario reads a scenario of web/contract/scenarios.
func loadScenario(t *testing.T, name string) (steps []step, fixtures map[string]string) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("scenarios", name+".json"))
	if err != nil {
		t.Fatal(err)
	}

	var sc struct {
		Steps    []step            `json:"steps"`
		Fixtures map[string]string `json:"fixtures"`
	}

	if err := json.Unmarshal(raw, &sc); err != nil {
		t.Fatalf("scenario %s: %v", name, err)
	}

	return sc.Steps, sc.Fixtures
}

// withFixtures replaces $name by the path of the fixture written in the
// environment of the driver.
func withFixtures(t *testing.T, d driver, s step, fixtures map[string]string) step {
	t.Helper()

	args := string(s.Args)

	for name, content := range fixtures {
		var path string
		if fw, ok := d.(fixtureWriter); ok {
			path = fw.writeFixture(t, name)
		} else {
			path = d.env().WriteFile(t, name+".mov", content)
		}

		args = strings.ReplaceAll(args, `"$`+name+`"`, mustJSON(t, path))
	}

	s.Args = json.RawMessage(args)

	return s
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

var jobID = regexp.MustCompile(`job_[0-9a-hjkmnp-tv-z]{10}`)

// mcpSession is the session id the MCP server appends to the actor label of a client (VV-026,
// web/app/mcp sessionLabel); the other surfaces have no session, so it is dropped to compare them.
var mcpSession = regexp.MustCompile(`(local:contract)-[0-9a-hjkmnp-tv-z]{6}\b`)

// templateID is the identifier a saved template is given: the Service makes it,
// so a scenario reads it back as "$template:0", the first one a result named.
var templateID = regexp.MustCompile(`tpl_[0-9a-hjkmnp-tv-z]{10}`)

// normalize makes a result comparable across runs: job identifiers and
// workspace paths are placeholders, and the text and the bulk members are
// dropped (the text is compared on its own).
func normalize(t *testing.T, d driver, raw json.RawMessage) string {
	t.Helper()

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}

	ws := string(d.env().Dir)
	files := d.env().Files

	var walk func(any) any

	walk = func(x any) any {
		switch x := x.(type) {
		case map[string]any:
			delete(x, "text")
			delete(x, "document")

			for k, e := range x {
				x[k] = walk(e)
			}

			return x
		case []any:
			for i, e := range x {
				x[i] = walk(e)
			}

			return x
		case string:
			for _, dir := range []string{ws, filepath.ToSlash(ws), files, filepath.ToSlash(files)} {
				x = strings.ReplaceAll(x, dir, "<dir>")
			}

			x = strings.ReplaceAll(x, "\\", "/")

			return mcpSession.ReplaceAllString(
				templateID.ReplaceAllString(jobID.ReplaceAllString(x, "job_X"), "tpl_X"),
				"$1",
			)
		}

		return x
	}

	out, err := json.Marshal(walk(v))
	if err != nil {
		t.Fatal(err)
	}

	return string(out)
}

func normalizeText(d driver, text string) string {
	for _, dir := range []string{string(d.env().Dir), d.env().Files} {
		text = strings.ReplaceAll(text, dir, "<dir>")
	}

	text = templateID.ReplaceAllString(jobID.ReplaceAllString(strings.ReplaceAll(text, "\\", "/"), "job_X"), "tpl_X")

	return mcpSession.ReplaceAllString(text, "$1")
}

// surfaceNames and toolNames are the words a hint uses for the commands of a surface (service.Surface).
var (
	surfaceNames = regexp.MustCompile(
		`project:(list|show|create|trash)|command:help|media:(import|inspect)|job:(show|cancel)|font:import|` +
			`(GET|POST) /api/v1|go back to the projects page|with transcribe `)
	toolNames = regexp.MustCompile(
		`(project_list|project_create|project_get|project_trash|command_help|media_import|media_inspect|` +
			`job_start|job_get|job_cancel|font_import)`)
)

func errorLine(doc *service.ErrorDocument) string {
	if doc == nil {
		return ""
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		return err.Error()
	}

	// Through a map the members come out in key order, whatever the surface that
	// decoded them put them in.
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return err.Error()
	}

	// The hint names the commands of the surface that answered (VV-101, SDD-003 section 1.4), so only
	// its presence is compared; code, message and details are equal on every surface.
	if m, ok := generic.(map[string]any); ok {
		if h, _ := m["hint"].(string); surfaceNames.MatchString(h) || toolNames.MatchString(h) {
			m["hint"] = "<hint>"
		}
	}

	canonical, err := json.Marshal(generic)
	if err != nil {
		return err.Error()
	}

	return string(canonical)
}

type outcome struct {
	result, text, fail string
}

// runAs sends a step of another actor straight to the Service of the workspace.
func runAs(t *testing.T, d driver, s step) {
	t.Helper()

	if _, err := callService(testenv.Caller("local:"+s.As), d.service(), s); err != nil {
		t.Fatalf("%s: step %s as %s: %v", d.name(), s.Call, s.As, err)
	}
}

func run(t *testing.T, d driver, steps []step, fixtures map[string]string) []outcome {
	t.Helper()

	out := make([]outcome, len(steps))

	// jobs are the identifiers the results of this driver have named so far, in
	// order: a later step reads them as "$job:0", "$job:1" and so on.
	// bundles are the paths of the bundles the exports wrote, read back as "$bundle:0"; imported are the
	// ids the server gave to imported projects, read back as "$project:0" and masked in the results,
	// because the server chooses them.
	var jobs, templates, bundles, imported []string

	for i, s := range steps {
		s = withFixtures(t, d, s, fixtures)

		for n, id := range jobs {
			s.Args = json.RawMessage(strings.ReplaceAll(string(s.Args), `"$job:`+strconv.Itoa(n)+`"`, `"`+id+`"`))
		}

		for n, id := range templates {
			s.Args = json.RawMessage(strings.ReplaceAll(string(s.Args), `"$template:`+strconv.Itoa(n)+`"`, `"`+id+`"`))
		}

		for n, p := range bundles {
			key := `"$bundle:` + strconv.Itoa(n) + `"`
			s.Args = json.RawMessage(strings.ReplaceAll(string(s.Args), key, mustJSON(t, p)))
		}

		for n, id := range imported {
			s.Args = json.RawMessage(strings.ReplaceAll(string(s.Args), `"$project:`+strconv.Itoa(n)+`"`, `"`+id+`"`))
		}

		if s.As != "" {
			runAs(t, d, s)

			continue
		}

		raw, text, fail := d.do(t, s)

		switch {
		case s.ExpectError != nil && fail == nil:
			t.Fatalf("%s step %d (%s): expected %s, got success", d.name(), i, s.Call, s.ExpectError.Code)
		case s.ExpectError != nil && fail.Code != s.ExpectError.Code:
			t.Fatalf("%s step %d (%s): code %s, want %s", d.name(), i, s.Call, fail.Code, s.ExpectError.Code)
		case s.ExpectError == nil && fail != nil:
			t.Fatalf("%s step %d (%s): %s", d.name(), i, s.Call, errorLine(fail))
		}

		bundles, imported = learnBundleResults(t, d, s, raw, bundles, imported)

		o := outcome{text: normalizeText(d, text), fail: errorLine(fail)}
		if raw != nil {
			o.result = normalize(t, d, raw)

			for _, id := range jobID.FindAllString(string(raw), -1) {
				if !slices.Contains(jobs, id) {
					jobs = append(jobs, id)
				}
			}

			for _, id := range templateID.FindAllString(string(raw), -1) {
				if !slices.Contains(templates, id) {
					templates = append(templates, id)
				}
			}
		}

		if s.Settles {
			o.result, o.text = "", ""
		}

		for _, id := range imported {
			o.result = strings.ReplaceAll(o.result, id, "prj_I")
			o.text = strings.ReplaceAll(o.text, id, "prj_I")
			o.fail = strings.ReplaceAll(o.fail, id, "prj_I")
		}

		out[i] = o
	}

	return out
}

// finalDocument is the canonical bytes of the document of the project, read
// from the Service of the workspace the driver wrote.
func finalDocument(t *testing.T, d driver, project string) []byte {
	t.Helper()

	res, err := d.service().ProjectGet(callerCtx(), service.ProjectGetRequest{ProjectID: project, Detail: "document"})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(res.Document)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

var renderedByAdapter = map[string]bool{"Edit": true, "Undo": true, "Redo": true}

func TestScenarioIsTheSameThroughEverySurface(t *testing.T) {
	docs := sameThroughEverySurface(t, "prototype", "prj_7k2m9x4q1b")

	checkRecordedHash(t, "prototype", docs["prj_7k2m9x4q1b"])
}

// TestTemplateScenarioIsTheSameThroughEverySurface (P2-AGT-006): a template is
// saved from a project, instantiated with a slot left unfilled, refused by the
// export gate until the slot is filled, then exported.
func TestTemplateScenarioIsTheSameThroughEverySurface(t *testing.T) {
	docs := sameThroughEverySurface(t, "template", "prj_7k2m9x4q1b", "prj_6t4y2w8j1m")

	sum := sha256.Sum256(append(append([]byte{}, docs["prj_7k2m9x4q1b"]...), docs["prj_6t4y2w8j1m"]...))
	checkRecordedSum(t, "template", hex.EncodeToString(sum[:]))
}

// TestCaptionsScenarioIsTheSameThroughEverySurface (P2-AGT-006): a clip is
// transcribed with the stub provider, captions are made from the transcript and
// the silences of the clip are removed, with the ids supplied by the scenario.
// TestCaptionsScenarioOnSixRuns runs it through the real binary too.
func TestCaptionsScenarioIsTheSameThroughEverySurface(t *testing.T) {
	docs := sameThroughEverySurfaceWith(t, "captions", captionsOptions(), "prj_7k2m9x4q1b")

	checkRecordedHash(t, "captions", docs["prj_7k2m9x4q1b"])
}

// sameThroughEverySurface runs the scenario through Direct, HTTP, CLI and MCP
// and fails when a result, an error, a text or a final document of the projects
// differs from the Direct one. It returns the documents Direct ended with.
func sameThroughEverySurface(t *testing.T, scenario string, projects ...string) map[string][]byte {
	t.Helper()

	return sameThroughEverySurfaceWith(t, scenario, testenv.Options{}, projects...)
}

// sameThroughEverySurfaceWith is sameThroughEverySurface over environments made
// with the options.
func sameThroughEverySurfaceWith(
	t *testing.T, scenario string, o testenv.Options, projects ...string,
) map[string][]byte {
	t.Helper()

	steps, fixtures := loadScenario(t, scenario)

	oracle := newDirectWith(t, o)
	want := run(t, oracle, steps, fixtures)
	wantDocs := map[string][]byte{}

	for _, p := range projects {
		wantDocs[p] = finalDocument(t, oracle, p)
	}

	for _, mk := range []func(*testing.T, testenv.Options) driver{newHTTPWith, newCLIWith, newMCPWith} {
		d := mk(t, o)
		got := run(t, d, steps, fixtures)

		for i := range steps {
			if got[i].result != want[i].result {
				t.Errorf("%s step %d (%s): result differs\n got: %s\nwant: %s",
					d.name(), i, steps[i].Call, got[i].result, want[i].result)
			}

			if got[i].fail != want[i].fail {
				t.Errorf("%s step %d (%s): error differs\n got: %s\nwant: %s",
					d.name(), i, steps[i].Call, got[i].fail, want[i].fail)
			}

			// The text of an edit, an undo and a redo is rendered by the adapter from
			// the result; every other text is the one of the Service.
			if !renderedByAdapter[steps[i].Call] && got[i].text != want[i].text {
				t.Errorf("%s step %d (%s): text differs\n got: %q\nwant: %q",
					d.name(), i, steps[i].Call, got[i].text, want[i].text)
			}
		}

		for _, p := range projects {
			if doc := finalDocument(t, d, p); !bytes.Equal(doc, wantDocs[p]) {
				t.Errorf("%s: the final document of %s differs\n got: %s\nwant: %s", d.name(), p, doc, wantDocs[p])
			}
		}
	}

	return wantDocs
}

// checkRecordedHash compares the document with the SHA-256 recorded next to the
// scenario; UPDATE_CONTRACT=1 rewrites it after a deliberate change.
func checkRecordedHash(t *testing.T, scenario string, doc []byte) {
	t.Helper()

	sum := sha256.Sum256(doc)
	checkRecordedSum(t, scenario, hex.EncodeToString(sum[:]))
}

func checkRecordedSum(t *testing.T, scenario, got string) {
	t.Helper()

	path := filepath.Join("scenarios", scenario+".sha256")

	if os.Getenv("UPDATE_CONTRACT") == "1" {
		if err := os.WriteFile(path, []byte(got+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (UPDATE_CONTRACT=1 go test ./web/contract/ records it)", err)
	}

	if want := strings.TrimSpace(string(raw)); want != got {
		t.Errorf("the final document hashes to %s, the recorded hash is %s", got, want)
	}
}

func TestErrorScenarioCarriesTheSameCodeEverywhere(t *testing.T) {
	steps := []step{
		{Call: "ProjectCreate", Args: json.RawMessage(`{"id":"prj_7k2m9x4q1b","name":"A"}`)},
		{
			Call: "Edit", ExpectError: &struct {
				Code string `json:"code"`
			}{Code: "E_VALIDATION"},
			Args: json.RawMessage(`{"project_id":"prj_7k2m9x4q1b","base_revision":7,"operations":[` +
				`{"op":"track.add","args":{"kind":"visual"}}]}`),
		},
		{
			Call: "ProjectGet", ExpectError: &struct {
				Code string `json:"code"`
			}{Code: "E_NOT_FOUND"},
			Args: json.RawMessage(`{"project_id":"prj_0000000000"}`),
		},
	}

	var first []outcome

	for _, mk := range []func(*testing.T) driver{newDirect, newHTTP, newCLI, newMCP} {
		d := mk(t)
		got := run(t, d, steps, nil)

		if first == nil {
			first = got

			continue
		}

		for i := range steps {
			if got[i].fail != first[i].fail {
				t.Errorf("%s step %d: error differs\n got: %s\nwant: %s", d.name(), i, got[i].fail, first[i].fail)
			}
		}
	}

	if !slices.ContainsFunc(first, func(o outcome) bool { return strings.Contains(o.fail, "E_VALIDATION") }) {
		t.Errorf("the conflict scenario produced no E_VALIDATION: %v", first)
	}
}

// failureDrivers are the four ways of reaching a Service in one process; the
// subprocess drivers have their own tests.
var failureDrivers = []func(*testing.T, testenv.Options) driver{newDirectWith, newHTTPWith, newCLIWith, newMCPWith}

// runEverywhere drives the steps through the four drivers, each over its own
// workspace made with the options, and fails when the outcome of any step, error
// documents included, differs from the Direct one. It returns the drivers, so a
// test can read what the workspaces ended up as.
func runEverywhere(
	t *testing.T, steps []step, fixtures map[string]string, o testenv.Options,
) (drivers []driver, outcomes []outcome) {
	t.Helper()

	for _, mk := range failureDrivers {
		d := mk(t, o)
		got := run(t, d, steps, fixtures)

		drivers = append(drivers, d)

		if outcomes == nil {
			outcomes = got

			continue
		}

		for i := range steps {
			if got[i].fail != outcomes[i].fail {
				t.Errorf("%s step %d (%s): error differs\n got: %s\nwant: %s",
					d.name(), i, steps[i].Call, got[i].fail, outcomes[i].fail)
			}

			if got[i].result != outcomes[i].result {
				t.Errorf("%s step %d (%s): result differs\n got: %s\nwant: %s",
					d.name(), i, steps[i].Call, got[i].result, outcomes[i].result)
			}
		}
	}

	return drivers, outcomes
}

// TestConflictScenario: two actors change the same track from the same revision;
// the second gets E_REVISION_CONFLICT (409, retryable, exit 2) with the diff and a
// hint to re-plan, and the document stays the first actor's, on every surface.
func TestConflictScenario(t *testing.T) {
	steps, fixtures := loadScenario(t, "conflict")
	drivers, outcomes := runEverywhere(t, steps, fixtures, testenv.Options{})

	var doc service.ErrorDocument
	if err := json.Unmarshal([]byte(outcomes[3].fail), &doc); err != nil {
		t.Fatal(err)
	}

	if doc.Code != "E_REVISION_CONFLICT" || doc.Status != 409 || !doc.Retryable {
		t.Errorf("the conflict is %+v", doc)
	}

	if _, ok := doc.Details["diff"]; !ok {
		t.Errorf("the conflict carries no details.diff: %+v", doc)
	}

	// The hint is the same on every surface: read details.diff, re-plan, resend.
	for _, want := range []string{"details.diff", "re-plan", "send it again"} {
		if !strings.Contains(doc.Hint, want) {
			t.Errorf("the conflict hint does not contain %q: %q", want, doc.Hint)
		}
	}

	for _, d := range drivers {
		final := string(finalDocument(t, d, "prj_7k2m9x4q1b"))
		if !strings.Contains(final, "Theirs") || strings.Contains(final, "Mine") {
			t.Errorf("%s: the document is not the first actor's: %s", d.name(), final)
		}
	}
}

// TestErrorsScenario: one call for each error the public surface can provoke,
// with the HTTP status of SDD-000 section 8 and retryable as it says, equal
// through every surface.
func TestErrorsScenario(t *testing.T) {
	steps, fixtures := loadScenario(t, "errors")

	hold := func(ctx context.Context, _ service.ExportInput) (*service.ExportOutput, error) {
		<-ctx.Done()

		return nil, ctx.Err()
	}

	_, outcomes := runEverywhere(t, steps, fixtures, testenv.Options{
		Export: hold,
		Limits: service.Limits{
			MaxImportBytes: 100, MaxExports: 1, Ceilings: ffmpeg.Ceilings{MaxSeconds: 3, MaxPixels: 100_000},
		},
	})

	// The duration and the resolution ceilings fire in the ingest job, not at the
	// call: the import is accepted, and the failed job carries the error, read
	// through the import result and through the job list of every surface.
	checkIngestLimits(t, steps, outcomes)

	retryable := map[string]bool{"E_REVISION_CONFLICT": true, "E_QUEUE_FULL": true, "E_WORKSPACE_LOCKED": true}

	for i, s := range steps {
		if s.ExpectError == nil {
			continue
		}

		var doc service.ErrorDocument
		if err := json.Unmarshal([]byte(outcomes[i].fail), &doc); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}

		if want := (&service.Error{Code: doc.Code}).HTTPStatus(); doc.Status != want || doc.Status == 500 {
			t.Errorf("step %d: %s has status %d, SDD-000 section 8 says %d", i, doc.Code, doc.Status, want)
		}

		if doc.Retryable != retryable[doc.Code] {
			t.Errorf("step %d: %s has retryable %v", i, doc.Code, doc.Retryable)
		}

		if doc.Message == "" {
			t.Errorf("step %d: %s has no message", i, doc.Code)
		}

		// A limit says which ceiling it hit and the value it has (RF-132).
		if limit := doc.Code == "E_LIMIT_EXCEEDED" || doc.Code == "E_QUEUE_FULL"; limit &&
			(doc.Details["ceiling"] == nil || doc.Details["variable"] == nil) {
			t.Errorf("step %d: %s does not name its ceiling and variable: %+v", i, doc.Code, doc.Details)
		}

		// ...and tells the actor what would be accepted (RF-112).
		if limit := doc.Code == "E_LIMIT_EXCEEDED" || doc.Code == "E_QUEUE_FULL"; limit && doc.Hint == "" {
			t.Errorf("step %d: %s has no hint", i, doc.Code)
		}
	}
}

// limitError is the error member of a failed asset or job.
type limitError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Hint      string         `json:"hint"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details"`
}

// checkIngestLimits reads the failed ingest jobs of the errors scenario: one for
// the duration ceiling and one for the resolution ceiling, each stating the
// limit, the value seen, the ceiling, the variable that raises it and what to
// do, and not retryable (RF-132, RF-112, SDD-000 section 8).
func checkIngestLimits(t *testing.T, steps []step, outcomes []outcome) {
	t.Helper()

	type expected struct {
		value, ceiling float64
		variable       string
	}

	want := map[string]expected{
		"import_seconds": {6, 3, "VUKEM_VIDEO_MAX_IMPORT_SECONDS"},
		"import_pixels":  {230_400, 100_000, "VUKEM_VIDEO_MAX_IMPORT_PIXELS"},
	}

	check := func(where string, e limitError, variable bool) {
		t.Helper()

		name, _ := e.Details["limit"].(string)

		w, known := want[name]
		if !known || e.Code != "E_LIMIT_EXCEEDED" || e.Retryable || e.Message == "" || e.Hint == "" {
			t.Errorf("%s: the ingest limit error is %+v", where, e)

			return
		}

		if e.Details["value"] != w.value || e.Details["ceiling"] != w.ceiling ||
			(variable && e.Details["variable"] != w.variable) {
			t.Errorf("%s: %s has details %+v, want value %v, ceiling %v, variable %s",
				where, name, e.Details, w.value, w.ceiling, w.variable)
		}
	}

	seen := 0

	for i, s := range steps {
		where := fmt.Sprintf("step %d (%s)", i, s.Call)

		switch s.Call {
		case "MediaImport":
			var res struct {
				Assets []struct {
					State string      `json:"state"`
					Error *limitError `json:"error"`
				} `json:"assets"`
			}

			if json.Unmarshal([]byte(outcomes[i].result), &res) != nil || len(res.Assets) != 2 {
				continue
			}

			for _, a := range res.Assets {
				if a.State != "failed" || a.Error == nil {
					t.Errorf("%s: an oversized import is %+v, want a failed asset with its error", where, a)

					continue
				}

				check(where, *a.Error, true)

				seen++
			}
		case "MediaList":
			var res struct {
				Assets []struct {
					State string      `json:"state"`
					Error *limitError `json:"error"`
				} `json:"assets"`
			}

			if json.Unmarshal([]byte(outcomes[i].result), &res) != nil {
				continue
			}

			for _, a := range res.Assets {
				if a.State != "failed" || a.Error == nil {
					t.Errorf("%s: an asset of the list is %+v", where, a)

					continue
				}

				// The stored error of an asset keeps the limit, the value and the
				// ceiling but not the variable that raises it (reported to the
				// service owner).
				check(where, *a.Error, false)

				seen++
			}
		case "JobGet":
			var res struct {
				State string      `json:"state"`
				Error *limitError `json:"error"`
			}

			if json.Unmarshal([]byte(outcomes[i].result), &res) != nil || res.State != "failed" || res.Error == nil {
				t.Errorf("%s: the job is %s, want a failed job with its error", where, outcomes[i].result)

				continue
			}

			check(where, *res.Error, true)

			seen++
		}
	}

	if seen != 6 {
		t.Errorf(
			"read %d ingest limit errors, want 2 each from the import result, the asset list and the failed jobs",
			seen,
		)
	}
}

// learnBundleResults notes what a bundle step answered that a later step reads back: the file of an
// export and the id of an imported project.
func learnBundleResults(
	t *testing.T, d driver, s step, raw json.RawMessage, bundles, imported []string,
) (newBundles, newImported []string) {
	t.Helper()

	var res struct {
		ProjectID string `json:"project_id"`
		Result    struct {
			Export struct {
				Path string `json:"path"`
			} `json:"export"`
		} `json:"result"`
	}

	if raw == nil || json.Unmarshal(raw, &res) != nil {
		return bundles, imported
	}

	switch {
	case s.Call == "ProjectBundleExport" && res.Result.Export.Path != "":
		// The import reads from the folders the workspace allows, not from where the export wrote.
		data, err := os.ReadFile(res.Result.Export.Path)
		if err != nil {
			t.Fatal(err)
		}

		copyPath := filepath.Join(d.env().Files, "export.vvbundle")
		if err := os.WriteFile(copyPath, data, 0o600); err != nil {
			t.Fatal(err)
		}

		bundles = append(bundles, copyPath)
	case s.Call == "ProjectBundleImport" && res.ProjectID != "" && !slices.Contains(imported, res.ProjectID):
		imported = append(imported, res.ProjectID)
	}

	return bundles, imported
}

// TestBundleScenarioIsTheSameThroughDirectHTTPAndCLI (P3-AGT-002): a project is exported to a bundle
// and imported as a new project. The bundle commands have no MCP tool (SDD-003 section 7.1, exception
// E-3), so the scenario runs through the other three surfaces. The imported document is the exported
// one with the id the server chose, byte for byte, and its canonical hash is recorded.
func TestBundleScenarioIsTheSameThroughDirectHTTPAndCLI(t *testing.T) {
	const original = "prj_7k2m9x4q1b"

	steps, fixtures := loadScenario(t, "bundle")

	oracle := newDirect(t)
	want := run(t, oracle, steps, fixtures)

	var wantDoc []byte

	for _, mk := range []func(*testing.T) driver{newDirect, newHTTP, newCLI} {
		d := mk(t)
		if d.name() == oracle.name() {
			d = oracle
		}

		got := want
		if d != oracle {
			got = run(t, d, steps, fixtures)
		}

		for i := range steps {
			if got[i] != want[i] {
				t.Errorf("%s step %d (%s): differs\n got: %+v\nwant: %+v", d.name(), i, steps[i].Call, got[i], want[i])
			}
		}

		doc := importedDocument(t, d, original)
		if wantDoc == nil {
			wantDoc = doc
		} else if !bytes.Equal(doc, wantDoc) {
			t.Errorf("%s: the imported document differs from the Direct one", d.name())
		}

		if !bytes.Equal(doc, finalDocument(t, d, original)) {
			t.Errorf("%s: the imported document is not the exported one\n got: %s\nwant: %s",
				d.name(), doc, finalDocument(t, d, original))
		}
	}

	checkRecordedHash(t, "bundle", wantDoc)
}

// importedDocument is the canonical document of the one project the workspace holds besides the
// original, with the original id put back, so that it is comparable with the exported one.
func importedDocument(t *testing.T, d driver, original string) []byte {
	t.Helper()

	list, err := d.service().ProjectList(callerCtx(), service.ProjectListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	var ids []string

	for i := range list.Projects {
		if id := list.Projects[i].ID; id != original {
			ids = append(ids, id)
		}
	}

	if len(ids) != 1 {
		t.Fatalf("%s: %d imported projects, want 1", d.name(), len(ids))
	}

	return bytes.ReplaceAll(finalDocument(t, d, ids[0]), []byte(ids[0]), []byte(original))
}

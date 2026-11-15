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
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

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

			return jobID.ReplaceAllString(x, "job_X")
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

	return jobID.ReplaceAllString(strings.ReplaceAll(text, "\\", "/"), "job_X")
}

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

	for i, s := range steps {
		s = withFixtures(t, d, s, fixtures)

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

		o := outcome{text: normalizeText(d, text), fail: errorLine(fail)}
		if raw != nil {
			o.result = normalize(t, d, raw)
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

// sameThroughEverySurface runs the scenario through Direct, HTTP, CLI and MCP
// and fails when a result, an error, a text or a final document of the projects
// differs from the Direct one. It returns the documents Direct ended with.
func sameThroughEverySurface(t *testing.T, scenario string, projects ...string) map[string][]byte {
	t.Helper()

	steps, fixtures := loadScenario(t, scenario)

	oracle := newDirect(t)
	want := run(t, oracle, steps, fixtures)
	wantDocs := map[string][]byte{}

	for _, p := range projects {
		wantDocs[p] = finalDocument(t, oracle, p)
	}

	for _, mk := range []func(*testing.T) driver{newHTTP, newCLI, newMCP} {
		d := mk(t)
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

	// The hint of the Service says to read the project again and plan against the
	// current revision; SDD-003 and the ROADMAP want it to name details.diff too,
	// which is for the Service to change (the hint is the same on every surface).
	if !strings.Contains(doc.Hint, "plan") {
		t.Errorf("the conflict hint does not tell the actor to re-plan: %q", doc.Hint)
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
		Export: hold, Limits: service.Limits{MaxImportBytes: 100, MaxExports: 1},
	})

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
	}
}

// Package contract holds the parity and contract tests of the agent surfaces
// (SDD-003 section 7, NFR-025): one scenario is driven through the in-process
// Service, the HTTP API, the CLI and the MCP tools, and the results, the errors
// and the final project document must be the same.
package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/service"
)

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
		path := d.env().WriteFile(t, name+".mov", content)
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

			if _, ok := x["assets"]; ok && x["project_id"] != nil && x["failed"] != nil {
				delete(x, "revision") // the import race, see settleImport
			}

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

	return string(raw)
}

type outcome struct {
	result, text, fail string
}

// settleImport works around a race of the Service: MediaImport returns when the
// asset is ready, but the asset.add that gives the project its revision is
// committed a moment later (service/jobrun.go finishes the ingest, then calls
// addAsset), so the revision of the answer and the base of the next edit are
// racy. Reported to the owner of service/; remove this and the revision
// exclusion in normalize when the call returns after the revision.
func settleImport(t *testing.T, d driver, s step) {
	t.Helper()

	var a struct {
		ProjectID string `json:"project_id"`
	}

	_ = json.Unmarshal(s.Args, &a)

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		res, err := d.service().ProjectGet(callerCtx(), service.ProjectGetRequest{ProjectID: a.ProjectID})
		if err == nil && res.Revision >= 1 {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("%s: the asset was never added to the project", d.name())
}

func run(t *testing.T, d driver, steps []step, fixtures map[string]string) []outcome {
	t.Helper()

	out := make([]outcome, len(steps))

	for i, s := range steps {
		s = withFixtures(t, d, s, fixtures)

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

		if s.Call == "MediaImport" {
			settleImport(t, d, s)
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
	steps, fixtures := loadScenario(t, "prototype")

	oracle := newDirect(t)
	want := run(t, oracle, steps, fixtures)
	wantDoc := finalDocument(t, oracle, "prj_7k2m9x4q1b")

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

		if doc := finalDocument(t, d, "prj_7k2m9x4q1b"); !bytes.Equal(doc, wantDoc) {
			t.Errorf("%s: the final document differs\n got: %s\nwant: %s", d.name(), doc, wantDoc)
		}
	}

	checkRecordedHash(t, wantDoc)
}

// checkRecordedHash compares the document with the SHA-256 recorded next to the
// scenario; UPDATE_CONTRACT=1 rewrites it after a deliberate change.
func checkRecordedHash(t *testing.T, doc []byte) {
	t.Helper()

	sum := sha256.Sum256(doc)
	got := hex.EncodeToString(sum[:])
	path := filepath.Join("scenarios", "prototype.sha256")

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

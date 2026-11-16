// Package results_test checks the results files that the detection page of
// desktop/detect leaves here (ADR-0023, P3-DSK-001): one file per run, named
// <platform>-<webview version>-<date>.json, with the verdict, the engine and
// the 18 tests, and a verdict that agrees with its required tests.
package results_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const testsPerRun = 18

var namePattern = regexp.MustCompile(`^[a-z0-9._]+-[a-z0-9._-]+-\d{4}-\d{2}-\d{2}(-\d+)?\.json$`)

type outcome struct {
	ID       string `json:"id"`
	Required bool   `json:"required"`
	Pass     bool   `json:"pass"`
	Detail   string `json:"detail"`
}

type run struct {
	Verdict   string    `json:"verdict"`
	Date      string    `json:"date"`
	Platform  string    `json:"platform"`
	Webview   string    `json:"webview"`
	UserAgent string    `json:"user_agent"`
	Results   []outcome `json:"results"`
}

func TestResultFiles(t *testing.T) {
	files, err := filepath.Glob("*.json")
	if err != nil {
		t.Fatal(err)
	}

	if len(files) == 0 {
		t.Fatal("no results file: one run of the detection page leaves one file here")
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			if !namePattern.MatchString(file) {
				t.Fatalf("name %q is not <platform>-<webview>-<date>.json", file)
			}

			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			var r run
			if err := json.Unmarshal(raw, &r); err != nil {
				t.Fatalf("not JSON: %v", err)
			}

			if r.Platform == "" || r.Webview == "" || r.UserAgent == "" || r.Date == "" {
				t.Fatalf("platform, webview, user_agent and date must be set: %+v", r)
			}

			if !strings.HasPrefix(file, strings.ToLower(r.Platform)+"-") {
				t.Errorf("file %q does not start with the platform %q", file, r.Platform)
			}

			if len(r.Results) != testsPerRun {
				t.Fatalf("%d tests, want %d", len(r.Results), testsPerRun)
			}

			requiredFailed := 0
			seen := map[string]bool{}

			for _, o := range r.Results {
				if o.ID == "" || seen[o.ID] {
					t.Errorf("empty or repeated test id %q", o.ID)
				}

				seen[o.ID] = true

				if o.Required && !o.Pass {
					requiredFailed++
				}
			}

			want := "pass"
			if requiredFailed > 0 {
				want = "fail"
			}

			if r.Verdict != want {
				t.Errorf("verdict %q but %d required tests failed (want %q)", r.Verdict, requiredFailed, want)
			}
		})
	}
}

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

	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/owner"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// The six runs of NFR-025 (SDD-003 section 9 check 26): Direct, MCP over stdio,
// MCP over HTTP, the CLI as a process with no server, the CLI as a process with a
// server, and HTTP. The process drivers run the real binary, whose transcription
// provider is the guarded test stub (STUARTY_TEST_TRANSCRIBER=stub under
// APP_ENV=test); Direct and HTTP are the in-process environments of testenv
// with the provider that returns the same words.

// testTranscriberEnv is what the child processes need for the stub provider.
var testTranscriberEnv = []string{"APP_ENV=test", owner.EnvTestTranscriber + "=stub"}

// stubWords are the words of testenv.StubTranscriber the binary stub also returns.
func stubWords() []transcribe.RawWord {
	return []transcribe.RawWord{
		{Text: "hello", StartMS: 100, EndMS: 400},
		{Text: "brave", StartMS: 400, EndMS: 700},
		{Text: "new", StartMS: 1100, EndMS: 1400},
		{Text: "world", StartMS: 1400, EndMS: 1900},
	}
}

// captionsOptions are the in-process environments of the captions scenario: the
// stub provider with the words of the binary, and the silence ranges of a real
// analysis, so that Direct and the real binary cut the same frames.
func captionsOptions() testenv.Options {
	return testenv.Options{Transcriber: &testenv.StubTranscriber{Words: stubWords()}, SilenceBlocks: true}
}

// startLocal prepares the workspace of a CLI process that runs with no server:
// every command opens the workspace, runs the Service in its own process and
// leaves.
func startLocal(t *testing.T, ffmpeg, ffprobe string) *procEnv {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "ws")
	files := filepath.Join(root, "files")

	if err := os.MkdirAll(files, 0o700); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "STUARTY_HOME="+home, "STUARTY_URL=", "APP_KEY=",
		"STUARTY_FFMPEG="+ffmpeg, "STUARTY_FFPROBE="+ffprobe, "STUARTY_MIN_FREE_BYTES=1")
	if !filepath.IsAbs(ffmpeg) {
		env = append(env, "STUARTY_FFMPEG=", "STUARTY_FFPROBE=")
	}

	return &procEnv{
		ffmpeg: ffmpeg, ffprobe: ffprobe, bin: binary(t), files: files,
		env: slices.Concat(env, testTranscriberEnv),
		e:   &testenv.Env{Dir: workspace.Dir(home), Files: files},
	}
}

// documentOf is the canonical document of the project as the driver's workspace
// holds it. A CLI process with no server has no HTTP client to read it with, so
// it reads it through `project:get`.
func documentOf(t *testing.T, d driver, project string) []byte {
	t.Helper()

	cp, ok := d.(*cliProc)
	if !ok || !cp.local {
		return finalDocument(t, d, project)
	}

	raw, _, fail := d.do(t, step{
		Call: "ProjectGet", Args: json.RawMessage(`{"project_id":"` + project + `","detail":"document"}`),
	})
	if fail != nil {
		t.Fatalf("%s: reading %s: %s", d.name(), project, errorLine(fail))
	}

	var res struct {
		Document json.RawMessage `json:"document"`
	}

	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := json.Compact(&out, res.Document); err != nil {
		t.Fatal(err)
	}

	return out.Bytes()
}

// sixRuns drives the scenario through the six runs and fails when a result, an
// error or a final document differs from the Direct one, or when the documents
// do not hash to the SHA-256 recorded next to the scenario.
func sixRuns(t *testing.T, scenario string, projects ...string) {
	t.Helper()

	ffmpeg, ffprobe := tools(t, false)
	steps, fixtures := loadScenario(t, scenario)
	opts := captionsOptions()

	oracle := newDirectWith(t, opts)
	want := run(t, oracle, steps, fixtures)

	var wantDocs [][]byte

	for _, p := range projects {
		wantDocs = append(wantDocs, finalDocument(t, oracle, p))
	}

	checkRecordedSum(t, scenario, sumOf(wantDocs))

	serving := func(mk func(*testing.T, *procEnv) driver) func(*testing.T) driver {
		return func(t *testing.T) driver {
			t.Helper()

			p := startProc(t, ffmpeg, ffprobe, testTranscriberEnv...)
			p.env = slices.Concat(p.env, testTranscriberEnv)

			return mk(t, p)
		}
	}

	// The ingest of the real binary probes the real file, so the results of a
	// media call carry other probe numbers than the fake ingest of the in-process
	// runs; the in-process HTTP run is compared with Direct, the process runs with
	// the first of them, and every run must end in the same document.
	type run1 struct {
		mk     func(*testing.T) driver
		oracle *[]outcome
	}

	var processWant []outcome

	runs := []run1{
		{func(t *testing.T) driver { t.Helper(); return newHTTPWith(t, opts) }, &want},
		{serving(mcpStdio), &processWant},
		{serving(mcpHTTP), &processWant},
		{serving(func(_ *testing.T, p *procEnv) driver { return &cliProc{p: p} }), &processWant},
		{
			func(t *testing.T) driver { t.Helper(); return &cliProc{p: startLocal(t, ffmpeg, ffprobe), local: true} },
			&processWant,
		},
	}

	for _, r := range runs {
		d := r.mk(t)
		got := run(t, d, steps, fixtures)

		if *r.oracle == nil {
			*r.oracle = got
		}

		for i := range steps {
			if scrub(got[i].result) != scrub((*r.oracle)[i].result) {
				t.Errorf("%s step %d (%s): result differs\n got: %s\nwant: %s",
					d.name(), i, steps[i].Call, scrub(got[i].result), scrub((*r.oracle)[i].result))
			}

			if scrub(got[i].fail) != scrub((*r.oracle)[i].fail) {
				t.Errorf("%s step %d (%s): error differs\n got: %s\nwant: %s",
					d.name(), i, steps[i].Call, got[i].fail, (*r.oracle)[i].fail)
			}
		}

		var docs [][]byte

		for i, p := range projects {
			doc := sameAssetHashes(documentOf(t, d, p), wantDocs[i])
			docs = append(docs, doc)

			if !bytes.Equal(doc, wantDocs[i]) {
				t.Errorf("%s: the final document of %s differs\n got: %s\nwant: %s", d.name(), p, doc, wantDocs[i])
			}
		}

		if sum, recorded := sumOf(docs), recordedSum(t, scenario); sum != recorded {
			t.Errorf("%s: the documents hash to %s, the recorded hash is %s", d.name(), sum, recorded)
		}
	}
}

var assetHash = regexp.MustCompile(`"hash":"sha256:[0-9a-f]{64}"`)

// sameAssetHashes puts the content hashes of the assets of the oracle in the
// document. The hash of an asset is the SHA-256 of the bytes of its file, and the
// real binary imports a real clip where Direct imports the fake-media marker, so
// it is the one member of the document that cannot be equal; every other byte is
// compared.
func sameAssetHashes(doc, oracle []byte) []byte {
	hashes := assetHash.FindAll(oracle, -1)

	return assetHash.ReplaceAllFunc(doc, func([]byte) []byte {
		if len(hashes) == 0 {
			return nil
		}

		h := hashes[0]
		hashes = hashes[1:]

		return h
	})
}

// recordedSum is the SHA-256 recorded next to the scenario.
func recordedSum(t *testing.T, scenario string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("scenarios", scenario+".sha256"))
	if err != nil {
		t.Fatal(err)
	}

	return strings.TrimSpace(string(raw))
}

// sumOf is the SHA-256 of the documents one after the other, as recorded.
func sumOf(docs [][]byte) string {
	sum := sha256.Sum256(bytes.Join(docs, nil))

	return hex.EncodeToString(sum[:])
}

func TestCaptionsScenarioOnSixRuns(t *testing.T) {
	sixRuns(t, "captions", "prj_7k2m9x4q1b")
}

func TestTemplateScenarioOnSixRuns(t *testing.T) {
	sixRuns(t, "template", "prj_7k2m9x4q1b", "prj_6t4y2w8j1m")
}

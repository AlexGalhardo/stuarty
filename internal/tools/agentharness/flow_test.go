package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeWorld is the person and the network of a first-flow run: the network holds nothing until the
// person approves, then it holds the post.
type fakeWorld struct {
	approved atomic.Bool
	early    bool
	bytes    int64
}

func (w *fakeWorld) ApprovePending(context.Context) (int, error) {
	w.approved.Store(true)

	return 1, nil
}

func (w *fakeWorld) Uploads(context.Context) ([]upload, error) {
	if w.early || w.approved.Load() {
		return []upload{{ID: "ff1", Path: "x.mp4", Bytes: w.bytes}}, nil
	}

	return nil, nil
}

const published = `{"publications":[{"id":"pub_1","export":"job_0123456789","targets":[` +
	`{"state":"published","approval_id":"apr_0123456789","url":"https://fake.example/watch/ff1"}]}]}`

func exportJob(ok bool) string {
	check := "true"
	if !ok {
		check = "false"
	}

	return `{"state":"succeeded","result":{"export":{"bytes":4096,"checks":[` +
		`{"name":"streams","ok":true},{"name":"preset","ok":` + check + `}]}}}`
}

// flowRunner answers the commands the agent runs, and the two the harness runs to judge the result.
func flowRunner(pub, job string) Runner {
	return func(_ context.Context, args []string, _ string) (string, int, error) {
		switch args[0] {
		case "publication:list":
			return pub, 0, nil
		case "job:show":
			return job, 0, nil
		}

		return "ok " + args[0], 0, nil
	}
}

func flowScript(t *testing.T) []string {
	t.Helper()

	var script []string

	for i, c := range []string{
		`project:create --name Sourdough --id prj_1`,
		`media:import clip.mp4 --project prj_1 --wait`,
		`edit --project prj_1 --ops ops.json`,
		`transcribe --project prj_1 --asset ast_1 --wait`,
		`image:create --preset vertical_cover`,
		`export --project prj_1 --preset youtube-shorts --wait`,
		`publication:create --project prj_1 --title T`,
		`publication:schedule pub_1`,
	} {
		script = append(script, toolUse(t, "t"+string(rune('a'+i)), c, ""))
	}

	return script
}

func playFlow(t *testing.T, env *flowEnv, run Runner) runResult {
	t.Helper()

	srv := httptest.NewServer(&fakeAPI{t: t, script: flowScript(t)})

	t.Cleanup(srv.Close)

	a := newAgent(srv.URL, srv.Client(), 20)
	a.flow = env

	res, err := a.play(t.Context(), firstFlow, 1, run)
	if err != nil {
		t.Fatal(err)
	}

	return res
}

func newEnv(w *fakeWorld) *flowEnv {
	return &flowEnv{person: w, network: w, wait: time.Second, poll: time.Millisecond, sleep: func(time.Duration) {}}
}

func TestFirstFlowSucceedsWhenAPersonApprovedAndTheExportedFileWentOut(t *testing.T) {
	t.Parallel()

	w := &fakeWorld{bytes: 4096}

	res := playFlow(t, newEnv(w), flowRunner(published, exportJob(true)))
	if !res.Success {
		t.Fatalf("not a success: %s", res.Reason)
	}

	if !w.approved.Load() {
		t.Error("the harness never approved as a person")
	}
}

func TestFirstFlowFailsWhatEverTheAgentDidWhenTheCheckFails(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		world *fakeWorld
		pub   string
		job   string
		want  string
	}{
		"a post out before the approval": {&fakeWorld{early: true, bytes: 4096}, published, exportJob(true), "before a person approved"},
		"a failed check of the export":   {&fakeWorld{bytes: 4096}, published, exportJob(false), "failed its check preset"},
		"another file went out":          {&fakeWorld{bytes: 7}, published, exportJob(true), "not the exported video"},
		"no approval on the target": {
			&fakeWorld{bytes: 4096}, strings.Replace(published, `"approval_id":"apr_0123456789",`, "", 1),
			exportJob(true), "carries no approval",
		},
		"never published": {
			&fakeWorld{bytes: 4096}, `{"publications":[{"targets":[{"state":"scheduled"}]}]}`, exportJob(true),
			"no target was published in time",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			res := playFlow(t, newEnv(tc.world), flowRunner(tc.pub, tc.job))
			if res.Success || !strings.Contains(res.Reason, tc.want) {
				t.Errorf("success=%t reason=%q, want a failure with %q", res.Success, res.Reason, tc.want)
			}
		})
	}
}

func TestFirstFlowWithoutServicesFails(t *testing.T) {
	t.Parallel()

	res := playFlow(t, nil, flowRunner(published, exportJob(true)))
	if res.Success || !strings.Contains(res.Reason, "no fake services") {
		t.Errorf("%t %q", res.Success, res.Reason)
	}
}

func TestFirstFlowNeedsEveryRequiredCommand(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(&fakeAPI{t: t, script: flowScript(t)[:3]})

	t.Cleanup(srv.Close)

	a := newAgent(srv.URL, srv.Client(), 20)
	a.flow = newEnv(&fakeWorld{bytes: 4096})

	res, err := a.play(t.Context(), firstFlow, 1, flowRunner(published, exportJob(true)))
	if err != nil || res.Success || !strings.Contains(res.Reason, "publication:schedule") {
		t.Errorf("%v %t %q", err, res.Success, res.Reason)
	}
}

func TestFirstFlowIsNotInAllAndNeedsItsServices(t *testing.T) {
	t.Parallel()

	all, _ := findScenarios("all")
	for _, sc := range all {
		if sc.Flow {
			t.Error("first-flow is in all")
		}
	}

	if got, ok := findScenarios("first-flow"); !ok || len(got) != 1 || !got[0].Flow || !got[0].NeedsClip {
		t.Errorf("first-flow: %+v %t", got, ok)
	}

	var stderr strings.Builder

	_, code := parseOptions(
		[]string{"--runs", "1", "--scenario", "first-flow", "--estimate"},
		func(k string) string { return map[string]string{"STUARTY_HARNESS": "1"}[k] }, &stderr,
	)
	if code != 2 || !strings.Contains(stderr.String(), "--flow-base") {
		t.Errorf("code %d: %s", code, stderr.String())
	}
}

func TestBuiltinSkillsAreAllGivenInOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	for _, name := range []string{"b-skill", "a-skill"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte("# "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got, err := builtinSkills(dir)
	if err != nil || strings.Index(got, "# a-skill") > strings.Index(got, "# b-skill") ||
		!strings.Contains(got, "# b-skill") {
		t.Errorf("%v %q", err, got)
	}

	if _, err := builtinSkills(t.TempDir()); err == nil {
		t.Error("an empty folder is not a set of skills")
	}

	// The real ones are there, and the first flow uses them.
	if _, err := builtinSkills(filepath.Join("..", "..", "..", "web", "resources", "skills")); err != nil {
		t.Errorf("the built-in skills of the product: %v", err)
	}
}

// The live person signs in with the launcher secret, then approves what the queue lists.
func TestLivePersonSignsInAndApproves(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "token"), []byte("vvl_tok\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var decided atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/system/launch", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer vvl_tok" || r.Header.Get("Stuarty-Launcher") != "secret" {
			http.Error(w, "no", http.StatusForbidden)

			return
		}

		_, _ = w.Write([]byte(`{"url":"http://` + r.Host + `/auth/launch?code=c1"}`))
	})
	mux.HandleFunc("GET /auth/launch", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "stuarty_session", Value: "s1", Path: "/"})
		w.WriteHeader(http.StatusSeeOther)
	})
	mux.HandleFunc("GET /api/v1/approvals", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("stuarty_session"); err != nil {
			http.Error(w, "no session", http.StatusUnauthorized)

			return
		}

		_, _ = w.Write([]byte(`{"requests":[{"id":"apr_1"},{"id":"apr_2"}]}`))
	})
	mux.HandleFunc("POST /api/v1/approvals/{id}/decide", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == "" {
			http.Error(w, "no origin", http.StatusForbidden)

			return
		}

		decided.Add(1)
		_, _ = w.Write([]byte(`{}`))
	})

	srv := httptest.NewServer(mux)

	t.Cleanup(srv.Close)

	n, err := newLivePerson(srv.URL, "secret", home).ApprovePending(context.Background())
	if err != nil || n != 2 || decided.Load() != 2 {
		t.Errorf("approved %d (%d decided): %v", n, decided.Load(), err)
	}

	if _, err := newLivePerson(srv.URL, "wrong", home).ApprovePending(context.Background()); err == nil {
		t.Error("a wrong launcher secret signed in")
	}
}

func TestLiveNetworkReadsTheUploads(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uploads":[{"id":"ff1","path":"a.mp4","bytes":9}]}`))
	}))

	t.Cleanup(srv.Close)

	got, err := newLiveNetwork(srv.URL).Uploads(context.Background())
	if err != nil || len(got) != 1 || got[0].Bytes != 9 {
		t.Errorf("%+v %v", got, err)
	}
}

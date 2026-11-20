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

// fakeWorld is the person and the network of a first-flow run: the network holds what earlier runs sent
// (prior) and nothing more until the person approves, then it holds the post of this run.
type fakeWorld struct {
	approved atomic.Bool
	reads    atomic.Int32
	early    bool
	prior    []upload
	bytes    int64
	// pending is the calendar after the agent stopped and before the approval (default waitingPub).
	pending string
}

func (w *fakeWorld) ApprovePending(context.Context) (int, error) {
	w.approved.Store(true)

	return 1, nil
}

func (w *fakeWorld) Uploads(context.Context) ([]upload, error) {
	// The first read is the one before the agent starts: an early post goes out during the run.
	first := w.reads.Add(1) == 1

	out := append([]upload{}, w.prior...)
	if (w.early && !first) || w.approved.Load() {
		out = append(out, upload{ID: "ff9", Path: "x.mp4", Bytes: w.bytes})
	}

	return out, nil
}

const (
	noPublication = `{"publications":[],"total":0}`
	waitingPub    = `{"publications":[{"id":"pub_1","export":"job_0123456789","targets":[` +
		`{"state":"waiting_approval","approval_id":"apr_0123456789"}]}]}`
	published = `{"publications":[{"id":"pub_1","export":"job_0123456789","targets":[` +
		`{"state":"published","approval_id":"apr_0123456789","url":"https://fake.example/watch/ff1"}]}]}`
	// earlier is the calendar after an earlier run of the same series: its post went out with another file.
	earlier = `{"publications":[{"id":"pub_0","export":"job_9999999999","targets":[` +
		`{"state":"published","approval_id":"apr_9999999999"}]}]}`
)

func exportJob(ok bool) string {
	check := "true"
	if !ok {
		check = "false"
	}

	return `{"state":"succeeded","result":{"export":{"bytes":4096,"checks":[` +
		`{"name":"streams","ok":true},{"name":"preset","ok":` + check + `}]}}}`
}

// flowRunner answers the commands the agent runs, and the ones the harness runs to judge the result: the
// first publication:list (before the agent starts) reads before, the next ones read the pending calendar
// of w until the person approved, then after.
func flowRunner(w *fakeWorld, before, after, job string) Runner {
	var lists atomic.Int32

	pending := w.pending
	if pending == "" {
		pending = waitingPub
	}

	return func(_ context.Context, args []string, _ string) (string, int, error) {
		switch args[0] {
		case "publication:list":
			switch {
			case lists.Add(1) == 1:
				return before, 0, nil
			case !w.approved.Load():
				return pending, 0, nil
			}

			return after, 0, nil
		case "job:show":
			if args[1] != "job_0123456789" {
				return `{"state":"succeeded","result":{"export":{"bytes":1,"checks":[{"name":"x","ok":true}]}}}`, 0, nil
			}

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

func playFlowWith(t *testing.T, api *fakeAPI, env *flowEnv, run Runner) runResult {
	t.Helper()

	srv := httptest.NewServer(api)

	t.Cleanup(srv.Close)

	a := newAgent(srv.URL, srv.Client(), 20)
	a.flow = env

	res, err := a.play(t.Context(), firstFlow, 1, run)
	if err != nil {
		t.Fatal(err)
	}

	return res
}

func playFlow(t *testing.T, env *flowEnv, run Runner) runResult {
	t.Helper()

	return playFlowWith(t, &fakeAPI{t: t, script: flowScript(t)}, env, run)
}

// fakeClock is the time of a run: it moves only when the harness sleeps.
type fakeClock struct{ now time.Time }

func (c *fakeClock) sleep(d time.Duration) { c.now = c.now.Add(d) }

var t0 = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func newEnv(w *fakeWorld) *flowEnv {
	c := &fakeClock{now: t0}

	return &flowEnv{
		person: w, network: w, wait: 3 * time.Minute, poll: 2 * time.Second,
		sleep: c.sleep, now: func() time.Time { return c.now },
	}
}

func TestFirstFlowSucceedsWhenAPersonApprovedAndTheExportedFileWentOut(t *testing.T) {
	t.Parallel()

	w := &fakeWorld{bytes: 4096}

	res := playFlow(t, newEnv(w), flowRunner(w, noPublication, published, exportJob(true)))
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
		after string
		job   string
		want  string
	}{
		"a post out before the approval": {
			&fakeWorld{early: true, bytes: 4096}, published, exportJob(true), "before a person approved",
		},
		"a target published before the approval": {
			&fakeWorld{
				bytes: 4096,
				pending: `{"publications":[{"id":"pub_1","export":"job_0123456789","targets":[` +
					`{"state":"publishing"}]}]}`,
			},
			published, exportJob(true), "before a person approved",
		},
		"a failed check of the export": {&fakeWorld{bytes: 4096}, published, exportJob(false), "failed its check preset"},
		"another file went out":        {&fakeWorld{bytes: 7}, published, exportJob(true), "not the exported video"},
		"no approval on the target": {
			&fakeWorld{bytes: 4096}, strings.Replace(published, `"approval_id":"apr_0123456789",`, "", 1),
			exportJob(true), "carries no approval",
		},
		"no publication made by the run": {
			&fakeWorld{bytes: 4096, pending: noPublication}, noPublication, exportJob(true), "no publication",
		},
		"never published": {
			&fakeWorld{bytes: 4096}, `{"publications":[{"id":"pub_1","targets":[{"state":"scheduled"}]}]}`,
			exportJob(true), "no target was published in time",
		},
		"missed its time": {
			&fakeWorld{bytes: 4096}, `{"publications":[{"id":"pub_1","targets":[{"state":"missed"}]}]}`,
			exportJob(true), "its target is missed",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			res := playFlow(t, newEnv(tc.world), flowRunner(tc.world, noPublication, tc.after, tc.job))
			if res.Success || !strings.Contains(res.Reason, tc.want) {
				t.Errorf("success=%t reason=%q, want a failure with %q", res.Success, res.Reason, tc.want)
			}
		})
	}
}

// The runs of a series share the server and its workspace: the second run is judged by its own
// publication and its own upload, not by what the first one left.
func TestFirstFlowJudgesARunByWhatItAdded(t *testing.T) {
	t.Parallel()

	prior := []upload{{ID: "ff1", Path: "old.mp4", Bytes: 1}}
	both := `{"publications":[{"id":"pub_0","export":"job_9999999999","targets":[` +
		`{"state":"published","approval_id":"apr_9999999999"}]},` +
		`{"id":"pub_1","export":"job_0123456789","targets":[` +
		`{"state":"published","approval_id":"apr_0123456789"}]}]}`

	waits := strings.Replace(both, `"state":"published","approval_id":"apr_0123456789"`,
		`"state":"waiting_approval","approval_id":"apr_0123456789"`, 1)
	second := &fakeWorld{prior: prior, bytes: 4096, pending: waits}

	res := playFlow(t, newEnv(second), flowRunner(second, earlier, both, exportJob(true)))
	if !res.Success {
		t.Fatalf("a second run of the series failed: %s", res.Reason)
	}

	idle := &fakeWorld{prior: prior, bytes: 4096, pending: earlier}

	res = playFlow(t, newEnv(idle), flowRunner(idle, earlier, earlier, exportJob(true)))
	if res.Success || !strings.Contains(res.Reason, "no publication") {
		t.Errorf("the post of an earlier run counted for this one: %t %q", res.Success, res.Reason)
	}
}

// A post scheduled later than the wait goes out at its time, and the harness waits for it.
func TestFirstFlowWaitsForTheTimeOfThePost(t *testing.T) {
	t.Parallel()

	w := &fakeWorld{bytes: 4096}
	env := newEnv(w)
	at := t0.Add(15 * time.Minute)
	waiting := `{"publications":[{"id":"pub_1","export":"job_0123456789","targets":[` +
		`{"state":"waiting_approval","approval_id":"apr_0123456789","scheduled_at":"` + at.Format(time.RFC3339) + `"}]}]}`
	inner := flowRunner(w, noPublication, published, exportJob(true))

	var lists atomic.Int32

	// The calendar is empty when the run starts, then holds the post, published only from its time on.
	run := func(ctx context.Context, args []string, stdin string) (string, int, error) {
		if args[0] == "publication:list" && lists.Add(1) > 1 && env.now().Before(at) {
			return waiting, 0, nil
		}

		return inner(ctx, args, stdin)
	}

	env.network = networkFunc(func(ctx context.Context) ([]upload, error) {
		if env.now().Before(at) {
			return nil, nil
		}

		return w.Uploads(ctx)
	})

	res := playFlow(t, env, run)
	if !res.Success {
		t.Fatalf("a post fifteen minutes ahead was not waited for: %s", res.Reason)
	}

	if env.now().Before(at) {
		t.Errorf("the check ended at %s, before the time of the post %s", env.now(), at)
	}
}

type networkFunc func(context.Context) ([]upload, error)

func (f networkFunc) Uploads(ctx context.Context) ([]upload, error) { return f(ctx) }

// Without its services the run fails before the first turn, so it costs nothing.
func TestFirstFlowWithoutServicesFailsBeforeAnyTurn(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{t: t, script: flowScript(t)}

	res := playFlowWith(t, api, nil, flowRunner(&fakeWorld{}, noPublication, published, exportJob(true)))
	if res.Success || !strings.Contains(res.Reason, "no fake services") {
		t.Errorf("%t %q", res.Success, res.Reason)
	}

	if n := api.calls.Load(); n != 0 {
		t.Errorf("the model was called %d times for a run that could not be judged", n)
	}
}

func TestFirstFlowNeedsEveryRequiredCommand(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(&fakeAPI{t: t, script: flowScript(t)[:3]})

	t.Cleanup(srv.Close)

	w := &fakeWorld{bytes: 4096}
	a := newAgent(srv.URL, srv.Client(), 20)
	a.flow = newEnv(w)

	res, err := a.play(t.Context(), firstFlow, 1, flowRunner(w, noPublication, published, exportJob(true)))
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

	env := func(k string) string { return map[string]string{"STUARTY_HARNESS": "1"}[k] }

	var stderr strings.Builder

	_, code := parseOptions([]string{"--runs", "1", "--scenario", "first-flow"}, env, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "--flow-base") {
		t.Errorf("a run without its services: code %d: %s", code, stderr.String())
	}

	// The estimate is what the author reads before anything starts: it needs no service and no key.
	stderr.Reset()

	if _, code := parseOptions(
		[]string{"--runs", "5", "--scenario", "first-flow", "--estimate"}, env, &stderr,
	); code != 0 {
		t.Errorf("the estimate: code %d: %s", code, stderr.String())
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

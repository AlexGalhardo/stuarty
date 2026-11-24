//go:build crashtest

package servicetest

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/service/social/youtube"
	"github.com/AlexGalhardo/stuarty/web/app/testpublish"
)

const (
	killPostMicros   = 500_000
	killPostCurrency = "USD"
)

// killConnector is the fake network as the parent and the child both register it.
func killConnector(base string) *fake.Connector {
	c := fake.New(base)
	c.CostMicros, c.CostCurrency = killPostMicros, killPostCurrency

	return c
}

// killNetworkFor is the connector of a network name: the fake network or the YouTube connector over the
// fake YouTube, both on loopback.
func killNetworkFor(network, base string) social.Connector {
	if network == "youtube" {
		return youtube.New(testpublish.YouTubeConfig(base))
	}

	return killConnector(base)
}

// openKillEnv is how the child opens the workspace the parent prepared; the first of its arguments names the
// network.
func openKillEnv(t *testing.T) *testpublish.Env {
	t.Helper()

	key, err := hexDecode(os.Getenv(killRealKeyEnv))
	if err != nil {
		t.Fatal(err)
	}

	offset, err := strconv.ParseInt(os.Getenv(killRealOffsetEnv), 10, 64)
	if err != nil {
		t.Fatal(err)
	}

	env, err := testpublish.Open(context.Background(), testpublish.Options{
		Dir:       os.Getenv(killRealDirEnv),
		Connector: killNetworkFor(os.Getenv(killRealArgsEnv), os.Getenv(killRealNetEnv)),
		VaultKey:  key, Offset: time.Duration(offset),
	})
	if err != nil {
		t.Fatal(err)
	}

	return env
}

// killRealPublicationChild is the child of the publication cases: one pass of the scheduler, which claims the
// due target and sends it. The network holds the request, so the pass does not end until the parent kills it.
func killRealPublicationChild(t *testing.T) {
	env := openKillEnv(t)

	started, err := env.Sched.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("tick", started)
	env.Sched.Wait()
	fmt.Println("done")
}

// killWorld is a workspace with a project, a signed-in connection to the fake network, a succeeded export and
// one target that a person approved.
type killWorld struct {
	dir     string
	key     []byte
	network string
	base    string
	net     *testpublish.FakeNet
	yt      *testpublish.FakeYouTube
	project string
	conn    string
	sched   testpublish.Scheduled

	// held counts the requests of a send the network received and did not answer.
	held atomic.Int64
	// arrived is closed by the hook when the first send request is in flight.
	arrived chan struct{}
	once    sync.Once
}

func newKillWorld(t *testing.T) *killWorld {
	t.Helper()

	return newKillWorldFor(t, fake.Network, []byte("fake mp4 bytes"))
}

// newKillWorldFor is the world over a network: the fake one, or YouTube with its resumable upload; data is the
// file the export holds.
func newKillWorldFor(t *testing.T, network string, data []byte) *killWorld {
	t.Helper()

	w := &killWorld{dir: filepath.Join(t.TempDir(), "ws"), network: network, arrived: make(chan struct{})}

	if network == "youtube" {
		w.yt = testpublish.NewFakeYouTube(t)
		w.base = w.yt.Srv.URL
	} else {
		w.net = testpublish.NewFakeNet(t)
		w.base = w.net.Srv.URL
	}

	w.key = make([]byte, 32)

	if _, err := rand.Read(w.key); err != nil {
		t.Fatal(err)
	}

	env, err := testpublish.Open(context.Background(), testpublish.Options{
		Dir: w.dir, Connector: killNetworkFor(network, w.base), VaultKey: w.key,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = env.Close() }()

	if w.project, err = env.NewProject("kill"); err != nil {
		t.Fatal(err)
	}

	if w.conn, err = env.SignIn(network); err != nil {
		t.Fatal(err)
	}

	exp := testpublish.SeedExport(
		t, env.Store, w.dir, testpublish.UI, w.project, "job_0000009001", ".mp4", data, time.Now(),
	)

	if w.sched, err = env.ScheduleAndApprove(w.project, exp, w.conn, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	return w
}

// hold makes the network keep every request of a send in flight, unanswered and unrecorded, until the sender
// is gone: the request is held, the parent sees it, and the child is killed while it waits for the answer.
func (w *killWorld) hold() {
	w.net.SetHook(func(r *http.Request) bool {
		if r.Method != http.MethodPost || r.URL.Path != "/videos" {
			return true
		}

		// The body is read to its end first: the server notices that the sender is gone only once it has read
		// the request, and the wait below ends with the sender.
		_, _ = io.Copy(io.Discard, r.Body)

		w.held.Add(1)
		w.once.Do(func() { close(w.arrived) })

		<-r.Context().Done()

		return false
	})
}

// release lets the network answer again.
func (w *killWorld) release() { w.net.SetHook(nil) }

// open is a process on the workspace whose clock is past the due time by extra.
func (w *killWorld) open(t *testing.T, offset time.Duration) *testpublish.Env {
	t.Helper()

	env, err := testpublish.Open(context.Background(), testpublish.Options{
		Dir: w.dir, Connector: killNetworkFor(w.network, w.base), VaultKey: w.key, Offset: offset,
	})
	if err != nil {
		t.Fatalf("a new process cannot open the workspace after the kill: %v", err)
	}

	t.Cleanup(func() { _ = env.Close() })

	integrityOK(t, env.Store)

	return env
}

// dueOffset is a clock offset that puts the process on time for the target: a little after its time.
func (w *killWorld) dueOffset() time.Duration { return time.Until(w.sched.At) + 5*time.Second }

// child starts a child whose clock is at the offset.
func (w *killWorld) child(t *testing.T, offset time.Duration) *realChild {
	t.Helper()

	return startRealChild(t, "publication", killVars(w.dir, w.base, w.key, offset, w.network)...)
}

func (w *killWorld) target(t *testing.T, env *testpublish.Env) *pub.Target {
	t.Helper()

	got, err := env.Target(w.sched)
	if err != nil {
		t.Fatal(err)
	}

	return got
}

// spentToday is what the workspace has spent or reserved in the days of the summary, in micros. Not the line of
// today: the clock of a later process is moved by minutes past the lease, and a run near midnight UTC would read
// a cost reserved yesterday as none.
func spentToday(t *testing.T, env *testpublish.Env) int64 {
	t.Helper()

	sum, err := env.Svc.CostSummary(testpublish.Agent(), service.CostSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}

	var micros int64

	for _, d := range sum.Days {
		if d.Currency == killPostCurrency {
			micros += d.Micros
		}
	}

	return micros
}

// A real process is killed while the network holds the request of its send. A process that starts on the
// workspace leaves the target alone while the lease lives, takes it again when the lease ends, and the network
// counts one finished post.
func TestKillRealPublicationResumesAndPostsOnce(t *testing.T) {
	for _, moment := range []string{"claimed", "request held"} {
		t.Run(moment, func(t *testing.T) {
			w := newKillWorld(t)
			w.hold()

			offset := w.dueOffset()
			child := w.child(t, offset)

			switch moment {
			case "claimed":
				// The lease is taken and the attempt counted: the target is being sent, nothing is recorded as an end.
				poll := w.open(t, offset)
				waitFor(t, "the target to be claimed", func() bool {
					return w.target(t, poll).State == corepub.TargetPublishing
				}, child.output)
			default:
				waitForClosed(t, w.arrived, "the network to hold the request of the send", child.output)
			}

			child.kill(t)

			if got := w.net.Finished(); got != 0 {
				t.Fatalf("the network finished %d posts before the kill ended the sender", got)
			}

			w.release()

			// The lease is alive: a process on the same clock sends nothing.
			same := w.open(t, offset)

			if n, err := same.Sched.Tick(context.Background()); err != nil || n != 0 {
				t.Fatalf("a pass inside the lease started %d sends, %v", n, err)
			}

			same.Sched.Wait()

			if got := w.target(t, same); got.State != corepub.TargetPublishing || w.net.Finished() != 0 {
				t.Fatalf("inside the lease the target is %s and the network finished %d", got.State, w.net.Finished())
			}

			// The lease ends: one scheduler takes the target again, as an interrupted attempt, and sends it.
			after := w.open(t, offset+corepubLease+2*time.Second)

			if n, err := after.Sched.Tick(context.Background()); err != nil || n != 1 {
				t.Fatalf("a pass after the lease started %d sends, %v", n, err)
			}

			after.Sched.Wait()

			got := w.target(t, after)
			if got.State != corepub.TargetPublished || got.RemoteID == "" || got.URL == "" || got.Attempts != 2 {
				t.Fatalf("the target after the resume = %+v", got)
			}

			if w.net.Finished() != 1 {
				t.Errorf("the network finished %d posts, want exactly 1", w.net.Finished())
			}

			// The post is not sent again by any pass that follows.
			if n, err := after.Sched.Tick(context.Background()); err != nil || n != 0 || w.net.Finished() != 1 {
				t.Errorf("a pass after the post sent %d, the network finished %d, %v", n, w.net.Finished(), err)
			}

			if spent := spentToday(t, after); spent != killPostMicros {
				t.Errorf("spent today = %d, want %d: the post was paid once", spent, killPostMicros)
			}
		})
	}
}

// corepubLease is the lease of a send, from the rules of the core.
const corepubLease = corepub.Lease

// A post that keeps ending its process is stopped: after MaxAttempts it ends failed with the code interrupted,
// the network is not asked again, and the cost reserved for it is given back.
func TestKillRealPublicationInterruptedAfterMaxAttempts(t *testing.T) {
	w := newKillWorld(t)
	w.hold()

	base := w.dueOffset()

	for attempt := range corepub.MaxAttempts {
		offset := base + time.Duration(attempt)*(corepubLease+2*time.Second)
		child := w.child(t, offset)

		waitForCount(t, &w.held, int64(attempt+1), "the network to hold the request of attempt "+strconv.Itoa(attempt+1), child.output)
		child.kill(t)
	}

	w.release()

	last := base + corepub.MaxAttempts*(corepubLease+2*time.Second)
	env := w.open(t, last)

	if got := w.target(t, env); got.State != corepub.TargetPublishing || got.Attempts != corepub.MaxAttempts {
		t.Fatalf("before the last pass the target is %s after %d attempts", got.State, got.Attempts)
	}

	if spent := spentToday(t, env); spent != killPostMicros {
		t.Errorf("reserved before the end = %d, want %d", spent, killPostMicros)
	}

	if n, err := env.Sched.Tick(context.Background()); err != nil || n != 0 {
		t.Fatalf("the pass after the attempts were used started %d sends, %v", n, err)
	}

	env.Sched.Wait()

	got := w.target(t, env)
	if got.State != corepub.TargetFailed || got.Error == nil || got.Error.Code != "interrupted" {
		t.Fatalf("the target = %s %+v, want failed with the code interrupted", got.State, got.Error)
	}

	// The requests of the sign-in are in the log; a post is a POST /videos, and none was answered. The five the
	// network held were never recorded as received.
	if w.net.Finished() != 0 || uploads(w.net) != 0 || w.held.Load() != corepub.MaxAttempts {
		t.Errorf("the network got %d finished and %d recorded posts and held %d, want 0, 0 and %d",
			w.net.Finished(), uploads(w.net), w.held.Load(), corepub.MaxAttempts)
	}

	if n, err := env.Sched.Tick(context.Background()); err != nil || n != 0 || uploads(w.net) != 0 ||
		w.held.Load() != corepub.MaxAttempts {
		t.Errorf("a later pass started %d sends and the network saw %d posts and %d held, %v",
			n, uploads(w.net), w.held.Load(), err)
	}

	if spent := spentToday(t, env); spent != 0 {
		t.Errorf("the cost of a post that failed was not released: %d micros", spent)
	}
}

func waitForClosed(t *testing.T, ch <-chan struct{}, what string, seen func() string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(killDeadline):
		t.Fatalf("gave up waiting for %s after %s; saw:\n%s", what, killDeadline, seen())
	}
}

func waitForCount(t *testing.T, n *atomic.Int64, want int64, what string, seen func() string) {
	t.Helper()

	waitFor(t, what, func() bool { return n.Load() >= want }, seen)
}

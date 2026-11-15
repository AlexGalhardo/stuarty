//go:build crashtest

package servicetest

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/internal/outbound"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/remote"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

// SDD-005 section 18 check 7 over HTTP: a real `serve` process takes commits
// from 4 clients, is killed in the middle, and the reopened workspace holds
// every revision that a client was told about, with a log that rewinds to
// revision 0.
func TestKillRealServeDuringConcurrentHTTPCommits(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project := e.newProject("kill http")
	rev0 := append([]byte(nil), e.document(project)...)

	ops := make([]command.Operation, killWorkers)
	for i := range ops {
		ops[i] = op("track.add", fmt.Sprintf(`{"kind":"visual","name":"V%d"}`, i))
	}

	tracks := e.mustEdit(ui, project, 0, ops...).Created

	if err := e.store.Close(); err != nil { // the server owns the database now
		t.Fatal(err)
	}

	bin := filepath.Join(t.TempDir(), "vukem-video")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	build := exec.CommandContext(
		t.Context(),
		"go",
		"build",
		"-o",
		bin,
		"github.com/AlexGalhardo/vukem-video/web/cmd/app",
	)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	port := freeLoopbackPort(t)
	cmd := exec.CommandContext(t.Context(), bin, "serve")
	cmd.Env = append(os.Environ(), workspace.EnvHome+"="+string(e.dir), "VUKEM_VIDEO_URL=", "APP_KEY=", "APP_ENV=",
		"HOST=127.0.0.1", "PORT="+port)

	var log bytes.Buffer

	cmd.Stdout, cmd.Stderr = &log, &log

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	exited := make(chan struct{})

	go func() { _ = cmd.Wait(); close(exited) }()

	t.Cleanup(func() { _ = cmd.Process.Kill(); <-exited })

	var rt workspace.Runtime

	for deadline := time.Now().Add(30 * time.Second); ; {
		var err error
		if rt, err = e.dir.ReadRuntime(); err == nil && rt.PID == cmd.Process.Pid {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("serve did not announce itself\n%s", log.String())
		}

		time.Sleep(100 * time.Millisecond)
	}

	token, err := e.dir.LoadToken()
	if err != nil {
		t.Fatal(err)
	}

	client := remote.New("http://"+rt.Addr, token, outbound.LocalServer(time.Minute))
	ctx := service.WithCaller(context.Background(), service.Caller{Actor: agent, Scopes: service.AllScopes})

	var (
		mu    sync.Mutex
		acked = map[int64]string{}
		count atomic.Int64
		wg    sync.WaitGroup
		stop  = make(chan struct{})
	)

	for w := range killWorkers {
		wg.Go(func() {
			for i := range killPerWorker {
				select {
				case <-stop:
					return
				default:
				}

				head, err := client.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: project})
				if err != nil {
					return
				}

				res, err := client.Edit(ctx, service.EditRequest{
					ProjectID: project, BaseRevision: head.Revision,
					Operations: []command.Operation{op("text.add",
						fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":"w%d"}`, tracks[w], i*2, w))},
				})
				if err != nil {
					continue // a conflict with another client: read the head again
				}

				mu.Lock()
				acked[res.Revision] = ""
				mu.Unlock()

				count.Add(1)
			}
		})
	}

	for deadline := time.Now().Add(time.Minute); count.Load() < killAfterAcks; {
		if time.Now().After(deadline) {
			t.Fatalf("only %d commits were acknowledged\n%s", count.Load(), log.String())
		}

		time.Sleep(10 * time.Millisecond)
	}

	_ = cmd.Process.Kill()
	<-exited
	close(stop)
	wg.Wait()

	st, err := store.Open(context.Background(), e.dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatalf("reopen after the kill: %v", err)
	}

	defer st.Close()

	invariants(t, st, project)

	mu.Lock()
	defer mu.Unlock()

	for rev := range acked {
		if _, err := st.Operations().Get(context.Background(), project, rev); err != nil {
			t.Errorf("acknowledged revision %d is not in the log: %v", rev, err)
		}
	}

	assertRewindReachesRevisionZero(t, st, project, rev0)
}

func freeLoopbackPort(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = l.Close() }()

	_, port, _ := net.SplitHostPort(l.Addr().String())

	return port
}

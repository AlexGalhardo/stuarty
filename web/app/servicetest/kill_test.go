package servicetest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// SDD-005 section 18 check 7, at the service layer: a real child process
// commits from 4 goroutines and prints every acknowledged revision with its
// document hash; the parent kills it in the middle (Process.Kill is SIGKILL on
// Unix and TerminateProcess on Windows), reopens the database and checks that
// nothing acknowledged was lost. The same check over HTTP against a real
// `serve` process joins the suite when P1-SRV-005 and the CLI exist.

const (
	killRoleEnv   = "STUARTY_KILL_ROLE"
	killTracksEnv = "STUARTY_KILL_TRACKS"
	killWorkers   = 4
	killPerWorker = 250
	killAfterAcks = 60
)

// TestKillChild is the body of the child process; as a normal test it does nothing.
func TestKillChild(t *testing.T) {
	if os.Getenv(killRoleEnv) != "child" {
		t.Skip("only runs as the child of the kill test")
	}

	st, err := store.Open(context.Background(), workspace.Dir(os.Getenv(workspace.EnvHome)),
		store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	svc, err := service.New(service.Options{Store: st})
	if err != nil {
		t.Fatal(err)
	}

	ctx := service.WithCaller(context.Background(), service.Caller{Actor: agent, Scopes: service.AllScopes})
	parts := strings.Split(os.Getenv(killTracksEnv), ",") // project, then one track per worker

	var out sync.Mutex

	var wg sync.WaitGroup

	for w := range killWorkers {
		wg.Go(func() {
			for i := range killPerWorker {
				head, err := st.Projects().Head(ctx, parts[0])
				if err != nil {
					return
				}

				res, err := svc.Edit(ctx, service.EditRequest{
					ProjectID: parts[0], BaseRevision: head.Revision,
					Operations: []command.Operation{op("text.add",
						fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":"w%d"}`, parts[1+w], i*2, w))},
				})
				if err != nil {
					return
				}

				row, err := st.Operations().Get(ctx, parts[0], res.Revision)
				if err != nil {
					return
				}

				out.Lock()
				fmt.Fprintf(os.Stdout, "ack %d %s\n", res.Revision, row.DocSHA256)
				out.Unlock()
			}
		})
	}

	wg.Wait()
}

func TestKillMidRunLosesNothingAcknowledged(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project := e.newProject("kill")
	rev0 := append([]byte(nil), e.document(project)...)

	ops := make([]command.Operation, killWorkers)
	for i := range ops {
		ops[i] = op("track.add", fmt.Sprintf(`{"kind":"visual","name":"V%d"}`, i))
	}

	res := e.mustEdit(ui, project, 0, ops...)
	tracks := append([]string{project}, res.Created...)

	if err := e.store.Close(); err != nil { // the child owns the database now
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestKillChild$")
	cmd.Env = append(os.Environ(), killRoleEnv+"=child", workspace.EnvHome+"="+string(e.dir),
		killTracksEnv+"="+strings.Join(tracks, ","))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	acked := map[int64]string{}
	sc := bufio.NewScanner(stdout)

	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 3 || f[0] != "ack" || len(f[2]) != 64 {
			continue // a line cut by the kill, or test chatter
		}

		rev, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}

		acked[rev] = f[2]

		if len(acked) == killAfterAcks {
			break
		}
	}

	if len(acked) < killAfterAcks {
		t.Fatalf("the child acknowledged only %d commits before it ended", len(acked))
	}

	_ = cmd.Process.Kill()
	_, _ = bytes.NewBuffer(nil).ReadFrom(stdout) // let the pipe drain; the acks read so far are what clients saw
	_ = cmd.Wait()

	st, err := store.Open(context.Background(), e.dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatalf("reopen after the kill: %v", err)
	}

	defer st.Close()

	invariants(t, st, project)
	assertAcknowledgedAreDurable(t, st, project, acked)
	assertRewindReachesRevisionZero(t, st, project, rev0)
}

func assertAcknowledgedAreDurable(t *testing.T, st *store.Store, project string, acked map[int64]string) {
	t.Helper()

	ctx := context.Background()

	for rev, sha := range acked {
		row, err := st.Operations().Get(ctx, project, rev)
		if err != nil {
			t.Errorf("acknowledged revision %d is not in the log: %v", rev, err)
			continue
		}

		if row.DocSHA256 != sha {
			t.Errorf("revision %d: doc_sha256 %s, acknowledged %s", rev, row.DocSHA256, sha)
		}
	}
}

// The final document equals the log replayed backwards: the inverses of the
// rows, newest first, bring it to the document of revision 0.
func assertRewindReachesRevisionZero(t *testing.T, st *store.Store, project string, rev0 []byte) {
	t.Helper()

	ctx := context.Background()

	row, err := st.Projects().Get(ctx, project)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := timeline.Decode(row.Document)
	if err != nil {
		t.Fatal(err)
	}

	rows, _, err := st.Operations().Page(ctx, project, service.HistoryQuery{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}

	for i := range rows {
		var prims []command.Primitive
		if err := json.Unmarshal(rows[i].Inverse, &prims); err != nil {
			t.Fatalf("revision %d: %v", rows[i].Revision, err)
		}

		res, skipped := command.ApplyPrimitives(doc, prims, nil)
		if len(skipped) > 0 {
			t.Fatalf("revision %d: the inverse skipped %d primitives", rows[i].Revision, len(skipped))
		}

		doc = res.Doc
	}

	if got := timeline.Canonical(doc); !bytes.Equal(got, rev0) {
		t.Errorf("rewinding %d revisions does not reach the document of revision 0", len(rows))
	}
}

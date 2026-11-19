package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/events"
)

type sseEvent struct {
	id, event, data, raw string
}

type sseClient struct {
	t      testing.TB
	ch     chan sseEvent
	status int
	header http.Header
	body   []byte // what a non-200 answer carried
	cancel context.CancelFunc
}

// stream opens the event stream of a project as a bearer client.
func (h *harness) stream(path string, headers map[string]string) *sseClient {
	h.t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	h.t.Cleanup(cancel)

	hr, err := http.NewRequestWithContext(ctx, http.MethodGet, h.srv.URL+path, http.NoBody)
	if err != nil {
		h.t.Fatal(err)
	}

	hr.Header.Set("Authorization", "Bearer "+canary)

	for k, v := range headers {
		hr.Header.Set(k, v)
	}

	resp, err := h.srv.Client().Do(hr) //nolint:bodyclose // the reader goroutine closes it
	if err != nil {
		h.t.Fatal(err)
	}

	c := &sseClient{t: h.t, ch: make(chan sseEvent, 4096), status: resp.StatusCode, header: resp.Header, cancel: cancel}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		buf := make([]byte, 4096)
		n, _ := resp.Body.Read(buf)
		c.body = buf[:n]

		return c
	}

	go func() {
		defer resp.Body.Close()
		defer close(c.ch)

		sc := bufio.NewScanner(resp.Body)

		var (
			cur sseEvent
			raw strings.Builder
		)

		for sc.Scan() {
			line := sc.Text()
			raw.WriteString(line + "\n")

			if line == "" {
				cur.raw = raw.String()
				c.ch <- cur
				cur, raw = sseEvent{}, strings.Builder{}

				continue
			}

			k, v, _ := strings.Cut(line, ": ")

			switch k {
			case "id":
				cur.id = v
			case "event":
				cur.event = v
			case "data":
				cur.data = v
			}
		}
	}()

	return c
}

// next is the next event that is not the preamble or a heartbeat.
func (c *sseClient) next(d time.Duration) (sseEvent, bool) {
	c.t.Helper()

	timeout := time.After(d)

	for {
		select {
		case ev, ok := <-c.ch:
			if !ok {
				return sseEvent{}, false
			}

			if ev.event == "" && ev.data == "" { // preamble or heartbeat
				continue
			}

			return ev, true
		case <-timeout:
			return sseEvent{}, false
		}
	}
}

func (c *sseClient) revisions(n int) []int64 {
	c.t.Helper()

	var out []int64

	for len(out) < n {
		ev, ok := c.next(5 * time.Second)
		if !ok {
			c.t.Fatalf("got %d events, want %d: %v", len(out), n, out)
		}

		if ev.event != "batch.committed" {
			continue
		}

		rev, _ := strconv.ParseInt(ev.id, 10, 64)
		out = append(out, rev)
	}

	return out
}

func (h *harness) caller() context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: "local:test", Scopes: service.AllScopes})
}

// commits makes n revisions on a project: a track first when from is 0, then texts.
func (h *harness) commits(project string, from int64, n int) int64 {
	h.t.Helper()

	rev := from

	if rev == 0 {
		res, err := h.svc.Edit(h.caller(), service.EditRequest{
			ProjectID: project,
			Operations: []command.Operation{
				{Op: "track.add", Args: json.RawMessage(`{"kind":"visual","name":"V"}`)},
			},
		})
		if err != nil {
			h.t.Fatal(err)
		}

		h.track, rev = res.Created[0], res.Revision
		n--
	}

	for range n {
		res, err := h.svc.Edit(h.caller(), service.EditRequest{
			ProjectID: project, BaseRevision: rev,
			Operations: []command.Operation{{Op: "text.add", Args: json.RawMessage(
				fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":"t"}`, h.track, rev))}},
		})
		if err != nil {
			h.t.Fatal(err)
		}

		rev = res.Revision
	}

	return rev
}

func TestEventsStreamWireAndErrors(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("events")
	path := "/api/v1/projects/" + id + "/events"

	if c := h.stream(
		path+"?since=abc",
		nil,
	); c.status != http.StatusBadRequest ||
		!strings.Contains(string(c.body), "E_VALIDATION") {
		t.Errorf("since=abc = %d %s", c.status, c.body)
	}

	for name, c := range map[string]*sseClient{
		"since negative":  h.stream(path+"?since=-1", nil),
		"header negative": h.stream(path, map[string]string{"Last-Event-ID": "-3"}),
		"unknown project": h.stream("/api/v1/projects/prj_nothere0001/events", nil),
	} {
		if c.status == http.StatusOK {
			t.Errorf("%s: answered 200, want a JSON error", name)
		}
	}

	if c := h.stream(path, map[string]string{"Authorization": "Bearer nope"}); c.status != http.StatusUnauthorized {
		t.Errorf("a wrong token = %d", c.status)
	}

	c := h.stream(path, nil)
	if c.status != http.StatusOK || c.header.Get("Content-Type") != "text/event-stream" ||
		c.header.Get("Content-Encoding") != "" || c.header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("stream = %d %v", c.status, c.header)
	}

	pre := <-c.ch
	if pre.raw != "retry: 1000\ndata: \n\n" {
		t.Errorf("preamble = %q", pre.raw)
	}

	h.commits(id, 0, 2)

	ev, ok := c.next(5 * time.Second)
	if !ok || ev.id != "1" || ev.event != "batch.committed" {
		t.Fatalf("event = %+v", ev)
	}

	head := "id: 1\nevent: batch.committed\ndata: {\"project_id\":\"" + id +
		"\",\"revision\":1,\"base_revision\":0,\"kind\":\"edit\",\"actor\":\"local:test\",\"at\":\""
	if !strings.HasPrefix(ev.raw, head) || strings.Count(ev.raw, "data:") != 1 || !strings.HasSuffix(ev.raw, "}\n\n") {
		t.Errorf("batch.committed bytes = %q", ev.raw)
	}
}

// Last-Event-ID wins over since; with only since the replay starts after it
// (SDD-006 check 8); every replayed revision arrives once, in order.
func TestEventsReplayFromTheLog(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("replay")
	head := h.commits(id, 0, 12)
	path := "/api/v1/projects/" + id + "/events"

	if got := h.stream(path+"?since=5", nil).revisions(7); got[0] != 6 || got[6] != 12 {
		t.Errorf("since=5: %v", got)
	}

	if got := h.stream(path+"?since=5", map[string]string{"Last-Event-ID": "8"}).
		revisions(4); got[0] != 9 ||
		got[3] != 12 {
		t.Errorf("since=5 and Last-Event-ID 8: %v", got)
	}

	// At the head: nothing to replay, the next commit is the next event.
	c := h.stream(path+"?since="+strconv.FormatInt(head, 10), nil)
	h.commits(id, head, 1)

	if got := c.revisions(1); got[0] != head+1 {
		t.Errorf("after the head: %v", got)
	}

	// A client ahead of the head is told to reload, with no id, and the stream ends.
	ahead := h.stream(path+"?since=500", nil)

	ev, ok := ahead.next(5 * time.Second)
	if !ok || ev.event != "resync.required" || ev.id != "" ||
		ev.data != fmt.Sprintf(`{"project_id":%q,"revision":%d,"reason":"client_ahead"}`, id, head+1) {
		t.Errorf("resync = %+v", ev)
	}

	if _, open := ahead.next(2 * time.Second); open {
		t.Error("the stream stayed open after resync.required")
	}
}

// A revision that never reached the hub is filled from the log before the next
// one is sent (check 9), with no duplicate.
func TestEventsGapIsFilledFromTheLog(t *testing.T) {
	t.Parallel()

	h := newHarnessWith(t, events.Options{Drop: func(rev int64) bool { return rev == 5 || (rev >= 8 && rev <= 12) }})
	id := h.newProject("gap")
	head := h.commits(id, 0, 3)
	c := h.stream("/api/v1/projects/"+id+"/events?since="+strconv.FormatInt(head, 10), nil)

	h.commits(id, head, 12) // revisions 4 to 15

	got := c.revisions(12)
	for i, r := range got {
		if r != int64(i)+4 {
			t.Fatalf("revisions = %v, want 4..15 in order", got)
		}
	}
}

func TestEventsTwoStreamsSeeTheSameOrder(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("two")
	head := h.commits(id, 0, 1)
	path := "/api/v1/projects/" + id + "/events?since=" + strconv.FormatInt(head, 10)
	a, b := h.stream(path, nil), h.stream(path, nil)

	var wg sync.WaitGroup

	for w := range 4 {
		wg.Go(func() {
			for i := range 25 {
				for { // a conflict with another writer: read the head again
					cur, err := h.svc.ProjectGet(h.caller(), service.ProjectGetRequest{ProjectID: id})
					if err != nil {
						t.Error(err)

						return
					}

					_, err = h.svc.Edit(h.caller(), service.EditRequest{
						ProjectID: id, BaseRevision: cur.Revision,
						Operations: []command.Operation{{Op: "text.add", Args: json.RawMessage(
							fmt.Sprintf(
								`{"track_id":%q,"start":"%ds","duration":"1s","text":"w%d"}`,
								h.track,
								w*25+i,
								w,
							),
						)}},
					})
					if err == nil {
						break
					}

					if se, ok := errors.AsType[*service.Error](err); !ok ||
						se.Code != command.CodeRevisionConflict {
						t.Error(err)

						return
					}
				}
			}
		})
	}

	wg.Wait()

	ra, rb := a.revisions(100), b.revisions(100)
	if !sort.SliceIsSorted(ra, func(i, j int) bool { return ra[i] < ra[j] }) || ra[0] != head+1 || ra[99] != head+100 {
		t.Errorf("stream a = %v", ra)
	}

	for i := range ra {
		if ra[i] != rb[i] {
			t.Fatalf("the streams differ at %d: %d and %d", i, ra[i], rb[i])
		}
	}
}

func TestEventsHeartbeatAndLimit(t *testing.T) {
	t.Parallel()

	h := newHarnessWith(t, events.Options{Heartbeat: 30 * time.Millisecond, MaxPerProject: 2})
	id := h.newProject("beat")
	path := "/api/v1/projects/" + id + "/events"

	a := h.stream(path, nil)
	h.stream(path, nil)

	beats := 0

	for beats < 3 {
		select {
		case ev := <-a.ch:
			if ev.raw == "data: \n\n" {
				beats++
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("only %d heartbeats", beats)
		}
	}

	third := h.stream(path, nil)
	if third.status != http.StatusTooManyRequests || !strings.Contains(string(third.body), `"limit":"streams"`) ||
		!strings.Contains(string(third.body), "E_QUEUE_FULL") {
		t.Errorf("the third stream = %d %s", third.status, third.body)
	}

	// Another project is not affected.
	if c := h.stream("/api/v1/projects/"+h.newProject("other")+"/events", nil); c.status != http.StatusOK {
		t.Errorf("another project = %d", c.status)
	}
}

// A client that connects while a job is running gets its state before any live
// event, and the trash of a project is an event (SDD-006 sections 3.3, 4.4).
func TestEventsSnapshotAndProjectUpdated(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("snap")
	path := "/api/v1/projects/" + id + "/events"

	now := time.Now().UnixMilli()
	row := service.JobRow{
		ID: "job_0000000001", Kind: "export", ProjectID: &id, Actor: "local:test", State: service.JobRunning,
		Progress: 0.5, Input: json.RawMessage(`{}`), CreatedMs: now, UpdatedMs: now,
	}

	if err := h.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		return tx.Jobs().CreateLimited(ctx, row, nil)
	}); err != nil {
		t.Fatal(err)
	}

	c := h.stream(path, nil)

	ev, ok := c.next(5 * time.Second)
	if !ok || ev.event != "job.updated" || !strings.Contains(ev.data, `"job_id":"job_0000000001"`) ||
		!strings.Contains(ev.data, `"project_id":"`+id+`"`) ||
		!strings.Contains(ev.data, `"error":null,"result":null,"updated_at":"`) {
		t.Fatalf("snapshot = %+v", ev)
	}

	if _, err := h.svc.ProjectTrash(h.caller(), service.ProjectTrashRequest{ProjectID: id}); err != nil {
		t.Fatal(err)
	}

	ev, ok = c.next(5 * time.Second)
	if !ok || ev.event != "project.updated" || !strings.Contains(ev.data, `"state":"trashed"`) || ev.id != "" {
		t.Errorf("project.updated = %+v", ev)
	}
}

// In network mode a job event, live or in the snapshot, carries no server path;
// in local mode the editor still gets it.
func TestEventsInNetworkModeCarryNoExportPath(t *testing.T) {
	t.Parallel()

	const serverPath = "/srv/ws/exports/p/j.mp4"

	for _, tc := range []struct {
		name   string
		redact bool
	}{{"network", true}, {"local", false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarnessWith(t, events.Options{RedactPaths: tc.redact})
			id := h.newProject("redact")
			path := "/api/v1/projects/" + id + "/events"

			now := time.Now().UnixMilli()
			row := service.JobRow{
				ID: "job_0000000002", Kind: "export", ProjectID: &id, Actor: "local:test", State: service.JobQueued,
				Input: json.RawMessage(`{}`), CreatedMs: now, UpdatedMs: now,
			}
			result := json.RawMessage(`{"export":{"path":"` + serverPath + `","download_url":"/x"}}`)

			if err := h.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
				return tx.Jobs().CreateLimited(ctx, row, nil)
			}); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			if ok, err := h.store.Jobs().Claim(ctx, row.ID, now); err != nil || !ok {
				t.Fatalf("claim: %v %v", ok, err)
			}

			ok, err := h.store.Jobs().Finish(ctx, row.ID, service.JobSucceeded, result, nil, now)
			if err != nil || !ok {
				t.Fatalf("finish: %v %v", ok, err)
			}

			c := h.stream(path, nil)

			check := func(what string, data string) {
				t.Helper()

				if !strings.Contains(data, "download_url") || strings.Contains(data, serverPath) == tc.redact {
					t.Errorf("%s (redact=%v) = %s", what, tc.redact, data)
				}
			}

			ev, ok := c.next(5 * time.Second)
			if !ok || ev.event != "job.updated" {
				t.Fatalf("snapshot = %+v", ev)
			}

			check("snapshot", ev.data)

			h.events.Publish(service.Event{
				Type: "job.updated", Kind: service.JobExport, ProjectID: id, JobID: row.ID, State: service.JobSucceeded,
				Result: &service.JobResult{Export: &service.ExportResult{Path: serverPath, DownloadURL: "/x"}},
			})

			ev, ok = c.next(5 * time.Second)
			if !ok || ev.event != "job.updated" {
				t.Fatalf("live = %+v", ev)
			}

			check("live", ev.data)
		})
	}
}

// SDD-006 check 5: from the return of the commit to the receipt of the event on
// a loopback client, p95 at most 20 ms over 1,000 commits.
func TestEventsLatencyFromCommitToClient(t *testing.T) {
	h := newHarness(t)
	id := h.newProject("latency")
	head := h.commits(id, 0, 1)
	c := h.stream("/api/v1/projects/"+id+"/events?since="+strconv.FormatInt(head, 10), nil)

	const n = 1000

	var (
		mu       sync.Mutex
		returned = map[int64]time.Time{}
		received = map[int64]time.Time{}
		done     = make(chan struct{})
	)

	go func() {
		defer close(done)

		for len(received) < n {
			ev, ok := c.next(10 * time.Second)
			if !ok {
				return
			}

			rev, _ := strconv.ParseInt(ev.id, 10, 64)

			mu.Lock()
			received[rev] = time.Now()
			mu.Unlock()
		}
	}()

	rev := head

	for i := range n {
		res, err := h.svc.Edit(h.caller(), service.EditRequest{
			ProjectID:    id,
			BaseRevision: rev,
			Operations: []command.Operation{
				{Op: "project.rename", Args: json.RawMessage(fmt.Sprintf(`{"name":"n%d"}`, i))},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		rev = res.Revision

		mu.Lock()
		returned[rev] = time.Now()
		mu.Unlock()
	}

	<-done

	var lat []time.Duration

	mu.Lock()
	defer mu.Unlock()

	for r, at := range returned {
		got, ok := received[r]
		if !ok {
			t.Fatalf("revision %d never arrived", r)
		}

		lat = append(lat, max(got.Sub(at), 0))
	}

	slices.Sort(lat)

	p95 := lat[len(lat)*95/100]
	t.Logf("commit to event: p50 %v, p95 %v, max %v over %d commits", lat[len(lat)/2], p95, lat[len(lat)-1], len(lat))

	// The budget of SDD-006 check 5 is measured without the race detector, which
	// slows every goroutine switch; under it the numbers are only logged.
	if !raceEnabled && testing.CoverMode() == "" && p95 > 20*time.Millisecond {
		t.Errorf("p95 = %v, want at most 20ms", p95)
	}
}

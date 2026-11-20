package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/events"
)

// fatCommit adds 20 texts of 2,000 characters in one batch, so its event is about
// 45 KB, and deletes them again to stay under the limit of items. It returns the
// revision after both and the latency of the first commit.
func (h *harness) fatCommit(project, track string, rev int64) (int64, time.Duration) {
	h.t.Helper()

	ops := make([]command.Operation, 20)
	for i := range ops {
		ops[i] = command.Operation{Op: "text.add", Args: json.RawMessage(
			fmt.Sprintf(
				`{"track_id":%q,"start":"%ds","duration":"1s","text":%q}`,
				track,
				i,
				strings.Repeat("x", 2000),
			),
		)}
	}

	start := time.Now()

	add, err := h.svc.Edit(h.caller(), service.EditRequest{ProjectID: project, BaseRevision: rev, Operations: ops})
	if err != nil {
		h.t.Fatal(err)
	}

	took := time.Since(start)

	ids, err := json.Marshal(add.Created)
	if err != nil {
		h.t.Fatal(err)
	}

	del := []command.Operation{{Op: "item.delete", Args: json.RawMessage(`{"item_ids":` + string(ids) + `}`)}}

	res, err := h.svc.Edit(
		h.caller(),
		service.EditRequest{ProjectID: project, BaseRevision: add.Revision, Operations: del},
	)
	if err != nil {
		h.t.Fatal(err)
	}

	return res.Revision, took
}

// checkpoint empties the WAL of the harness database, so that the next commits start at the same phase of
// the automatic checkpoint.
func (h *harness) checkpoint() {
	h.t.Helper()

	// SQLite reports a checkpoint it could not finish as busy, not as an error, and the WAL is then not
	// emptied: the windows would start at different phases again without anything saying so.
	var busy, log, done int

	err := h.store.DB().SQL().QueryRowContext(h.t.Context(), "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &done)
	if err != nil {
		h.t.Fatal(err)
	}

	if busy != 0 || log != 0 {
		h.t.Fatalf("the WAL was not emptied: busy %d, %d frames in the log, %d checkpointed", busy, log, done)
	}
}

func p95(d []time.Duration) time.Duration {
	slices.Sort(d)

	return d[len(d)*95/100]
}

// SDD-006 check 6: a client that stops reading is dropped within 12 seconds, the
// commits go on at their usual latency, and the client then reconnects with its
// last id and receives every revision it missed.
func TestEventsSlowReaderIsDropped(t *testing.T) {
	h := newHarness(t)
	id := h.newProject("slow")
	head := h.commits(id, 0, 1)
	track := h.track
	path := "/api/v1/projects/" + id + "/events?since=" + strconv.FormatInt(head, 10)

	conn, err := (&net.Dialer{}).DialContext( //nolint:forbidigo // a raw client of the loopback test server
		t.Context(), "tcp", h.srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(1024)
	}

	fmt.Fprintf(
		conn,
		"GET %s HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\n\r\n",
		path,
		h.srv.Listener.Addr(),
		canary,
	)

	// The client is connected once the server counts it; it never reads again.
	waitFor(t, 5*time.Second, func() bool { return h.events.Subscribers(id) == 1 })

	// The baseline is measured on a project nobody streams.
	other := h.newProject("baseline")
	orev := h.commits(other, 0, 1)
	otrack := h.track
	base := make([]time.Duration, 0, 60)

	// Both windows start from an empty WAL: SQLite checkpoints on its own every 1,000 pages, which for these
	// commits is a spike of about 20 ms every 14 commits, and where the first one falls depends on how many
	// pages the migrations left in the WAL. Without this the p95 compares the phase of the checkpoints, not the
	// cost of the stalled client (VV-233).
	h.checkpoint()

	for range 60 {
		var d time.Duration

		orev, d = h.fatCommit(other, otrack, orev)
		base = append(base, d)
	}

	rev, during := head, make([]time.Duration, 0, 60)

	h.checkpoint()

	for range 60 {
		var d time.Duration

		rev, d = h.fatCommit(id, track, rev)
		during = append(during, d)
	}

	dropped := time.Now()

	waitFor(t, 12*time.Second, func() bool { return h.events.Subscribers(id) == 0 })

	t.Logf("subscriber removed %v after the last commit", time.Since(dropped))

	// Writes stay within 20% of the baseline (plus 5 ms of timer noise). The
	// race detector and coverage instrumentation distort both runs unevenly, so the
	// timing holds only for a normal build, which CI also runs; the drop itself is asserted above.
	if b, d := p95(base), p95(during); !raceEnabled && testing.CoverMode() == "" && d > b+b/5+5*time.Millisecond {
		t.Errorf("commit p95 during the stall = %v, baseline %v", d, b)
	}

	// The client reconnects with the last id it holds (the head it joined at).
	c := h.stream("/api/v1/projects/"+id+"/events", map[string]string{"Last-Event-ID": strconv.FormatInt(head, 10)})
	if c.status != http.StatusOK {
		t.Fatalf("reconnect = %d", c.status)
	}

	got := c.revisions(int(rev - head))
	for i, r := range got {
		if r != head+int64(i)+1 {
			t.Fatalf("missed revisions arrived as %v", got[:i+1])
		}
	}
}

// SDD-006 check 13: stopping the server ends the open streams within a second
// (the owner calls Events.Close before the HTTP server drains).
func TestEventsShutdownEndsStreams(t *testing.T) {
	h := newHarnessWith(t, events.Options{})
	id := h.newProject("shutdown")
	path := "/api/v1/projects/" + id + "/events"

	streams := []*sseClient{h.stream(path, nil), h.stream(path, nil), h.stream(path, nil)}
	waitFor(t, 5*time.Second, func() bool { return h.events.Subscribers(id) == len(streams) })

	start := time.Now()

	h.events.Close()

	for i, c := range streams {
		deadline := time.After(time.Second)

	drain:
		for {
			select {
			case _, ok := <-c.ch:
				if !ok {
					break drain
				}
			case <-deadline:
				t.Fatalf("stream %d still open %v after the stop", i, time.Since(start))
			}
		}
	}

	if d := time.Since(start); d > time.Second {
		t.Errorf("the streams ended after %v", d)
	}

	// A client that reconnects after the restart replays what it missed.
	h2 := newHarness(t)
	id2 := h2.newProject("replay")
	head := h2.commits(id2, 0, 3)

	c := h2.stream("/api/v1/projects/"+id2+"/events", map[string]string{"Last-Event-ID": "1"})
	if got := c.revisions(int(head - 1)); got[0] != 2 || got[len(got)-1] != head {
		t.Errorf("replay = %v", got)
	}
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(d)

	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %v", d)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

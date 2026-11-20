package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/events"
)

// The stream of an image document (SDD-006 section 12, epic 48).

func (h *harness) newImage(name string) string {
	h.t.Helper()

	res, err := h.svc.ImageCreate(h.caller(), service.ImageCreateRequest{Preset: "video_thumbnail", Name: name})
	if err != nil {
		h.t.Fatal(err)
	}

	return res.ImageID
}

func imageOp(name, args string) command.Operation {
	return command.Operation{Op: name, Args: json.RawMessage(args)}
}

// imageEdits makes n revisions on an image after revision from (each moves a fill) and returns the last one.
func (h *harness) imageEdits(id string, from int64, n int) int64 {
	h.t.Helper()

	rev := from

	if rev == 1 {
		res, err := h.svc.ImageEdit(h.caller(), service.ImageEditRequest{
			ImageID: id, BaseRevision: rev,
			Operations: []command.Operation{imageOp("layer.add_fill", `{"color":"#102030","id":"lyr_aaaaaaaaaa"}`)},
		})
		if err != nil {
			h.t.Fatal(err)
		}

		rev = res.Revision
		n--
	}

	for range n {
		// x differs from the one the revision before set, so every call commits
		x := int(rev)*3 + 1

		res, err := h.svc.ImageEdit(h.caller(), service.ImageEditRequest{
			ImageID:      id,
			BaseRevision: rev,
			Operations: []command.Operation{
				imageOp("layer.transform", fmt.Sprintf(`{"layer_id":"lyr_aaaaaaaaaa","x":%d}`, x)),
			},
		})
		if err != nil {
			h.t.Fatal(err)
		}

		rev = res.Revision
	}

	return rev
}

// imageRevisions reads n image.updated events and returns their revisions.
func (c *sseClient) imageRevisions(n int) []int64 {
	c.t.Helper()

	var out []int64

	for len(out) < n {
		ev, ok := c.next(10 * time.Second)
		if !ok {
			c.t.Fatalf("got %d events, want %d: %v", len(out), n, out)
		}

		if ev.event != "image.updated" {
			c.t.Fatalf("event %+v is not image.updated", ev)
		}

		rev, _ := strconv.ParseInt(ev.id, 10, 64)
		out = append(out, rev)
	}

	return out
}

type imageUpdated struct {
	ImageID       string            `json:"image_id"`
	Revision      int64             `json:"revision"`
	BaseRevision  int64             `json:"base_revision"`
	Kind          string            `json:"kind"`
	Actor         string            `json:"actor"`
	At            string            `json:"at"`
	Diff          []json.RawMessage `json:"diff"`
	DiffTruncated bool              `json:"diff_truncated"`
	Changed       []map[string]any  `json:"changed"`
}

// TestImageEventsOnePerCommittedRevision: one image.updated for each committed revision, in the shape of
// batch.committed, and none for a dry run, a batch that changes nothing or the creation of the document.
func TestImageEventsOnePerCommittedRevision(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newImage("Live")
	path := "/api/v1/images/" + id + "/events"

	c := h.stream(path, nil)
	if c.status != http.StatusOK || c.header.Get("Content-Type") != "text/event-stream" ||
		c.header.Get("Content-Encoding") != "" {
		t.Fatalf("stream = %d %v", c.status, c.header)
	}

	if pre := <-c.ch; pre.raw != "retry: 1000\ndata: \n\n" {
		t.Errorf("preamble = %q", pre.raw)
	}

	// revision 2 by an agent
	add := imageOp("layer.add_fill", `{"color":"#102030","id":"lyr_aaaaaaaaaa"}`)
	if _, err := h.svc.ImageEdit(h.caller(), service.ImageEditRequest{
		ImageID: id, BaseRevision: 1, Operations: []command.Operation{add},
	}); err != nil {
		t.Fatal(err)
	}

	ev, ok := c.next(5 * time.Second)
	if !ok || ev.id != "2" || ev.event != "image.updated" {
		t.Fatalf("event = %+v", ev)
	}

	var got imageUpdated
	if err := json.Unmarshal([]byte(ev.data), &got); err != nil {
		t.Fatal(err)
	}

	if got.ImageID != id || got.Revision != 2 || got.BaseRevision != 1 || got.Kind != "edit" ||
		got.Actor != "local:test" || len(got.Diff) == 0 || got.DiffTruncated || got.At == "" {
		t.Fatalf("payload = %+v", got)
	}

	if !strings.HasPrefix(ev.raw, "id: 2\nevent: image.updated\ndata: {\"image_id\":\""+id+"\",\"revision\":2,") ||
		strings.Count(ev.raw, "data:") != 1 {
		t.Errorf("bytes = %q", ev.raw)
	}

	// a dry run and a batch that changes nothing commit nothing and publish nothing
	move := imageOp("layer.transform", `{"layer_id":"lyr_aaaaaaaaaa","x":10}`)
	if _, err := h.svc.ImageEdit(h.caller(), service.ImageEditRequest{
		ImageID: id, BaseRevision: 2, Operations: []command.Operation{move}, DryRun: true,
	}); err != nil {
		t.Fatal(err)
	}

	same := imageOp("layer.set", `{"layer_id":"lyr_aaaaaaaaaa","opacity":100}`)
	if res, err := h.svc.ImageEdit(h.caller(), service.ImageEditRequest{
		ImageID: id, BaseRevision: 2, Operations: []command.Operation{same},
	}); err != nil || res.Applied {
		t.Fatalf("a batch that changes nothing: %v %+v", err, res)
	}

	// then a real one, an undo and a redo: the next events are exactly those
	if _, err := h.svc.ImageEdit(h.caller(), service.ImageEditRequest{
		ImageID: id, BaseRevision: 2, Operations: []command.Operation{move},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := h.svc.ImageUndo(h.caller(), service.ImageStepRequest{ImageID: id}); err != nil {
		t.Fatal(err)
	}

	if _, err := h.svc.ImageRedo(h.caller(), service.ImageStepRequest{ImageID: id}); err != nil {
		t.Fatal(err)
	}

	for i, rev := range c.imageRevisions(3) {
		if rev != int64(i)+3 {
			t.Fatalf("the revision after a dry run and a no-op is %d, want %d", rev, i+3)
		}
	}
}

// TestImageEventsKindsOfASession: the events of an undo and a redo say what they are.
func TestImageEventsKindsOfASession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newImage("Kinds")
	c := h.stream("/api/v1/images/"+id+"/events", nil)

	rev := h.imageEdits(id, 1, 2)

	if _, err := h.svc.ImageUndo(h.caller(), service.ImageStepRequest{ImageID: id}); err != nil {
		t.Fatal(err)
	}

	if _, err := h.svc.ImageRedo(h.caller(), service.ImageStepRequest{ImageID: id}); err != nil {
		t.Fatal(err)
	}

	want := []string{"edit", "edit", "undo", "redo"}

	for i := range want {
		ev, ok := c.next(5 * time.Second)
		if !ok {
			t.Fatalf("event %d is missing", i)
		}

		var p imageUpdated
		if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
			t.Fatal(err)
		}

		if p.Kind != want[i] || p.Revision != int64(i)+2 || ev.id != strconv.FormatInt(p.Revision, 10) {
			t.Fatalf("event %d = %+v", i, p)
		}
	}

	if rev != 3 {
		t.Fatalf("head %d", rev)
	}
}

// TestImageEventsReplayAfterLastEventID: after a dropped connection Last-Event-ID replays the missed revisions in
// order, once each; the header wins over since; the revision of the creation is not replayed; at the head nothing
// is replayed; ahead of the head is told to reload.
func TestImageEventsReplayAfterLastEventID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newImage("Replay")
	head := h.imageEdits(id, 1, 11) // revisions 2 to 12
	path := "/api/v1/images/" + id + "/events"

	got := h.stream(path, map[string]string{"Last-Event-ID": "5"}).imageRevisions(7)
	for i, r := range got {
		if r != int64(i)+6 {
			t.Fatalf("Last-Event-ID 5: %v, want 6..12 in order", got)
		}
	}

	if got := h.stream(path+"?since=5", nil).imageRevisions(7); got[0] != 6 || got[6] != 12 {
		t.Errorf("since=5: %v", got)
	}

	if got := h.stream(path+"?since=5", map[string]string{"Last-Event-ID": "8"}).
		imageRevisions(4); got[0] != 9 ||
		got[3] != 12 {
		t.Errorf("since=5 and Last-Event-ID 8: %v", got)
	}

	// the creation of the document is revision 1 and has no event: from 0 the first one is revision 2
	if got := h.stream(path+"?since=0", nil).imageRevisions(1); got[0] != 2 {
		t.Errorf("since=0: %v", got)
	}

	// at the head there is nothing to replay: the next commit is the next event
	c := h.stream(path+"?since="+strconv.FormatInt(head, 10), nil)
	h.imageEdits(id, head, 1)

	if got := c.imageRevisions(1); got[0] != head+1 {
		t.Errorf("after the head: %v", got)
	}

	// ahead of the head: told to reload, with no id, and the stream ends
	ahead := h.stream(path+"?since=500", nil)

	ev, ok := ahead.next(5 * time.Second)
	if !ok || ev.event != "resync.required" || ev.id != "" ||
		ev.data != fmt.Sprintf(`{"image_id":%q,"revision":%d,"reason":"client_ahead"}`, id, head+1) {
		t.Errorf("resync = %+v", ev)
	}

	if _, open := ahead.next(2 * time.Second); open {
		t.Error("the stream stayed open after resync.required")
	}
}

// A gap of more than 1,000 revisions is a resync, not a replay of everything.
func TestImageEventsGapTooLargeIsAResync(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newImage("Gap")
	head := h.imageEdits(id, 1, 1002)

	c := h.stream("/api/v1/images/"+id+"/events?since=0", nil)

	ev, ok := c.next(10 * time.Second)
	if !ok || ev.event != "resync.required" || ev.id != "" ||
		ev.data != fmt.Sprintf(`{"image_id":%q,"revision":%d,"reason":"gap_too_large"}`, id, head) {
		t.Fatalf("resync = %+v", ev)
	}

	if _, open := c.next(2 * time.Second); open {
		t.Error("the stream stayed open after resync.required")
	}

	// within the limit it replays
	if got := h.stream("/api/v1/images/"+id+"/events?since="+strconv.FormatInt(head-3, 10), nil).
		imageRevisions(3); got[0] != head-2 || got[2] != head {
		t.Errorf("within the limit: %v", got)
	}
}

// A revision that never reached the hub is filled from the log before the next is sent, once each, in order.
func TestImageEventsGapIsFilledFromTheLog(t *testing.T) {
	t.Parallel()

	h := newHarnessWith(t, events.Options{Drop: func(rev int64) bool { return rev == 5 || (rev >= 8 && rev <= 12) }})
	id := h.newImage("Fill")
	head := h.imageEdits(id, 1, 3) // 2 to 4
	c := h.stream("/api/v1/images/"+id+"/events?since="+strconv.FormatInt(head, 10), nil)

	h.imageEdits(id, head, 12) // revisions 5 to 16

	got := c.imageRevisions(12)
	for i, r := range got {
		if r != int64(i)+5 {
			t.Fatalf("revisions = %v, want 5..16 in order", got)
		}
	}
}

// A payload over 64 KiB carries the entries that changed and not the diff.
func TestImageEventsLargeDiffIsTruncated(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newImage("Big")
	c := h.stream("/api/v1/images/"+id+"/events", nil)

	var ops []command.Operation

	name := strings.Repeat("n", 116) // and up to four digits: 120 characters, the longest name

	for i := range 200 {
		ops = append(ops, imageOp("layer.add_fill", fmt.Sprintf(`{"color":"#102030","name":"%s%d"}`, name, i)))
	}

	if _, err := h.svc.ImageEdit(h.caller(), service.ImageEditRequest{
		ImageID: id, BaseRevision: 1, Operations: ops,
	}); err != nil {
		t.Fatal(err)
	}

	ev, ok := c.next(10 * time.Second)
	if !ok {
		t.Fatal("no event")
	}

	var p imageUpdated
	if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
		t.Fatal(err)
	}

	if !p.DiffTruncated || p.Diff != nil || len(p.Changed) != 200 || len(ev.data) > 64<<10 {
		t.Fatalf("truncated=%v diff=%d changed=%d bytes=%d", p.DiffTruncated, len(p.Diff), len(p.Changed), len(ev.data))
	}
}

func TestImageEventsRefusals(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newImage("Refuse")
	path := "/api/v1/images/" + id + "/events"

	if c := h.stream(path+"?since=abc", nil); c.status != http.StatusBadRequest ||
		!strings.Contains(string(c.body), "E_VALIDATION") {
		t.Errorf("since=abc = %d %s", c.status, c.body)
	}

	for name, c := range map[string]*sseClient{
		"since negative":  h.stream(path+"?since=-1", nil),
		"header negative": h.stream(path, map[string]string{"Last-Event-ID": "-3"}),
	} {
		if c.status != http.StatusBadRequest {
			t.Errorf("%s: answered %d, want 400", name, c.status)
		}
	}

	if c := h.stream("/api/v1/images/img_nothere001/events", nil); c.status != http.StatusNotFound ||
		!strings.Contains(string(c.body), "E_NOT_FOUND") {
		t.Errorf("an unknown image = %d %s", c.status, c.body)
	}

	if c := h.stream(path, map[string]string{"Authorization": "Bearer nope"}); c.status != http.StatusUnauthorized {
		t.Errorf("a wrong token = %d", c.status)
	}

	// a query credential is refused as everywhere (SDD-007 T-04)
	if c := h.stream(path+"?token="+canary, nil); c.status != http.StatusBadRequest {
		t.Errorf("a token in the query = %d", c.status)
	}
}

// The limit of streams of SDD-006 section 9.4 is counted per image.
func TestImageEventsStreamsAreLimited(t *testing.T) {
	t.Parallel()

	h := newHarnessWith(t, events.Options{MaxPerProject: 3})
	id := h.newImage("Limit")
	path := "/api/v1/images/" + id + "/events"

	for range 3 {
		if c := h.stream(path, nil); c.status != http.StatusOK {
			t.Fatalf("stream = %d", c.status)
		}
	}

	c := h.stream(path, nil)
	if c.status != http.StatusTooManyRequests || !strings.Contains(string(c.body), `"limit":"streams"`) {
		t.Fatalf("the 4th stream = %d %s", c.status, c.body)
	}

	// another image has its own count
	other := h.newImage("Other")
	if c := h.stream("/api/v1/images/"+other+"/events", nil); c.status != http.StatusOK {
		t.Fatalf("another image = %d", c.status)
	}
}

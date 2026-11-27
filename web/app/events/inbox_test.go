package events

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/sse"
)

func TestInboxStreamEndsAfterAnUnchangedSnapshotOrReadError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "unchanged"},
		{name: "read failed", err: errors.New("the caller can no longer read the inbox")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := New(nil, Options{})
			t.Cleanup(e.Close)
			live := make(chan sse.Event, 2)
			live <- inboxHint
			live <- inboxHint
			first := &service.InboxListResult{Waiting: 1}
			reads := 0
			snapshot := func(context.Context) (*service.InboxListResult, error) {
				reads++
				close(live)

				return first, tc.err
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/inbox/events", http.NoBody)
			rec := httptest.NewRecorder()
			err := sse.Serve(rec, req, func(stream *sse.Stream) error {
				return e.runInbox(req.Context(), stream, first, snapshot, live, func() bool { return true })
			}, sse.Heartbeat(0))
			if frames := strings.Count(
				rec.Body.String(),
				"event: inbox.snapshot",
			); err != nil || frames != 1 ||
				reads != 1 {
				t.Fatalf("stream produced %d frames and %d reads: %v", frames, reads, err)
			}
		})
	}
}

func TestDrainReturnsWhenTheHubClosesWithAPendingHint(t *testing.T) {
	t.Parallel()

	live := make(chan sse.Event, 1)
	live <- inboxHint
	close(live)
	drain(live)
	if _, ok := <-live; ok {
		t.Fatal("a pending hint was left in the closed channel")
	}
}

func TestChatStreamReplaysAndClosesOnRevocation(t *testing.T) {
	t.Parallel()
	e := New(nil, Options{Revalidate: time.Millisecond})
	t.Cleanup(e.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/chat/events", http.NoBody)
	rec := httptest.NewRecorder()
	rechecked := false
	if err := sse.Serve(rec, req, func(stream *sse.Stream) error {
		return e.runChat(stream, []chat.Event{{Seq: 1}}, make(chan chat.Event), func() bool {
			rechecked = true
			return false
		})
	}, sse.Heartbeat(0)); err != nil {
		t.Fatal(err)
	}
	if !rechecked || strings.Count(rec.Body.String(), "event: chat.event") != 1 {
		t.Fatal("the chat stream did not replay its snapshot and recheck authorization")
	}
}

func TestChatStreamSendsTheLastQueuedEventWhenTheSessionEnds(t *testing.T) {
	t.Parallel()
	e := New(nil, Options{})
	t.Cleanup(e.Close)
	live := make(chan chat.Event, 1)
	live <- chat.Event{Seq: 2}
	close(live)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/chat/events", http.NoBody)
	rec := httptest.NewRecorder()
	if err := sse.Serve(rec, req, func(stream *sse.Stream) error {
		return e.runChat(stream, []chat.Event{{Seq: 1}}, live, func() bool { return true })
	}, sse.Heartbeat(0)); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if first, last := strings.Index(body, "id: 1\n"), strings.Index(body, "id: 2\n"); first < 0 || last <= first {
		t.Fatal("the stream did not send the replay before the live event")
	}
}

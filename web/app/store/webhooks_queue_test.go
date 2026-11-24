package store

import (
	"errors"
	"strconv"
	"testing"

	core "github.com/AlexGalhardo/stuarty/core/webhook"
	"github.com/AlexGalhardo/stuarty/service/webhooks"
)

func queued(i int, kind string) webhooks.Delivery {
	return webhooks.Delivery{
		ID: "wdl_" + strconv.Itoa(i), WebhookID: "whk_1", Event: "export.ready", EventID: "evt_" + strconv.Itoa(i),
		Kind: kind, Body: []byte(`{}`), MaxAttempts: core.MaxAttempts, NextMs: 1, CreatedMs: 1,
	}
}

// VV-370: a webhook holds at most core.MaxPending pending deliveries; an event past that is refused with
// webhooks.ErrQueueFull and stores nothing, and the rows already stored are never deleted (SDD-022 section 4).
func TestWebhookQueueHasACeilingOfPendingEvents(t *testing.T) {
	s := testStore(t)

	if err := s.Webhooks().Insert(ctx, webhooks.Row{
		ID: "whk_1", Name: "receiver", URL: "https://hooks.example.com/x", Events: []string{"export.ready"},
		Enabled: true, CreatedBy: "local:ui", CreatedMs: 1, UpdatedMs: 1,
	}); err != nil {
		t.Fatal(err)
	}

	for i := range core.MaxPending {
		if ok, err := s.Webhooks().Enqueue(ctx, queued(i, "event")); err != nil || !ok {
			t.Fatalf("delivery %d: ok=%v err=%v", i, ok, err)
		}
	}

	ok, err := s.Webhooks().Enqueue(ctx, queued(core.MaxPending, "event"))
	if ok || !errors.Is(err, webhooks.ErrQueueFull) {
		t.Fatalf("past the ceiling: ok=%v err=%v", ok, err)
	}

	// A test send and a redelivery are a person's call: the ceiling does not refuse them.
	if ok, err := s.Webhooks().Enqueue(ctx, queued(core.MaxPending+1, "test")); err != nil || !ok {
		t.Fatalf("a test send: ok=%v err=%v", ok, err)
	}

	listed, err := s.Webhooks().Deliveries(ctx, "whk_1", core.MaxPending+10, 0)
	if err != nil || len(listed) != core.MaxPending+1 {
		t.Fatalf("stored %d deliveries, err %v; want %d", len(listed), err, core.MaxPending+1)
	}
}

package events_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/events"
)

func TestDecideIsTheTableOfSection43(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		since       *int64
		head, floor int64
		action      events.Action
		reason      string
	}{
		"no revision":       {nil, 10, 0, events.Live, ""},
		"at the head":       {new(int64(10)), 10, 0, events.Nothing, ""},
		"behind":            {new(int64(4)), 10, 0, events.Replay, ""},
		"the whole range":   {new(int64(0)), 1000, 0, events.Replay, ""},
		"too far behind":    {new(int64(0)), 1500, 0, events.Resync, events.ReasonGapTooLarge},
		"under the floor":   {new(int64(700)), 900, 800, events.Resync, events.ReasonBeforeFloor},
		"ahead of the head": {new(int64(50)), 40, 0, events.Resync, events.ReasonClientAhead},
	} {
		action, reason := events.Decide(c.since, c.head, c.floor)
		if action != c.action || reason != c.reason {
			t.Errorf("%s: %v %q, want %v %q", name, action, reason, c.action, c.reason)
		}
	}
}

// A batch over 64 KiB keeps its fields and swaps the diff for the list of what
// changed, at most 500 entries (check 15).
func TestBatchFrameTruncatesALargeDiff(t *testing.T) {
	t.Parallel()

	var diff []map[string]any

	for i := range 600 {
		diff = append(diff, map[string]any{
			"kind":   "item.added",
			"id":     fmt.Sprintf("itm_%010d", i),
			"object": map[string]any{"text": strings.Repeat("x", 200)},
		})
	}

	raw, err := json.Marshal(diff)
	if err != nil {
		t.Fatal(err)
	}
	row := service.OperationRow{
		ProjectID:    "prj_0000000001",
		Revision:     45,
		Kind:         service.OpEdit,
		Actor:        "key:k",
		BaseRevision: 44,
		AtMs:         1,
		Diff:         raw,
	}

	f, err := events.BatchFrame(&row, "many")
	if err != nil {
		t.Fatal(err)
	}

	var p struct {
		Diff          json.RawMessage `json:"diff"`
		DiffTruncated bool            `json:"diff_truncated"`
		Changed       []struct {
			ID string `json:"id"`
		} `json:"changed"`
		ChangedMore int    `json:"changed_more"`
		Description string `json:"description"`
	}

	if err := json.Unmarshal([]byte(f.Data), &p); err != nil {
		t.Fatal(err)
	}

	if f.ID != "45" || f.Event != "batch.committed" || len(f.Data) > events.MaxEventBytes || p.Diff != nil ||
		!p.DiffTruncated || len(p.Changed) != events.MaxChanged || p.ChangedMore != 100 || p.Description != "many" {
		t.Errorf("frame = %d bytes, %+v", len(f.Data), p)
	}
}

func TestFramesHaveTheFieldsOfTheProtocol(t *testing.T) {
	t.Parallel()

	row := service.OperationRow{
		ProjectID: "prj_0000000001", Revision: 44, Kind: service.OpUndo, Actor: "local:ui", BaseRevision: 43,
		TargetRevision: new(int64(43)), AtMs: 1_700_000_000_123, Skipped: json.RawMessage(`[{"a":1}]`),
	}

	f, _ := events.BatchFrame(&row, "d")

	const want = `{"project_id":"prj_0000000001","revision":44,"base_revision":43,"kind":"undo","actor":"local:ui",` +
		`"at":"2023-11-14T22:13:20.123Z","target_revision":43,"skipped_count":1,"description":"d","diff":[]}`
	if f.Data != want {
		t.Errorf("batch = %s", f.Data)
	}

	if r := events.ResyncFrame("prj_1", 1500, events.ReasonGapTooLarge); r.ID != "" ||
		r.Data != `{"project_id":"prj_1","revision":1500,"reason":"gap_too_large"}` {
		t.Errorf("resync = %+v", r)
	}

	job := events.JobFrame(&service.Event{ProjectID: "prj_1", JobID: "job_1", Kind: "ingest", State: "running"})
	if !strings.HasPrefix(job.Data, `{"project_id":null,"job_id":"job_1","kind":"ingest","state":"running"`) {
		t.Errorf("ingest job = %s", job.Data)
	}
}

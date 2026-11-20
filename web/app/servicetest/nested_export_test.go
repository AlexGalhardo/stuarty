package servicetest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

// TestNestedExportKeepsTheTimelinesOfItsStart: an export of a timeline that holds a composition renders, through
// the service and FFmpeg, the nested timeline as it was when the export was asked for, though it is edited while
// the job waits (SDD-004 §3.16).
func TestNestedExportKeepsTheTimelinesOfItsStart(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	parent, child, _ := nestedProject(t, m.env)
	asked := m.head(child).Revision

	job, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: parent, TimelineID: parent, Kind: "export"})
	if err != nil {
		t.Fatal(err)
	}

	// The nested timeline changes after the export was asked for.
	if _, err := m.textIn(child, m.editIn(t, child, op("track.add", `{"kind":"visual"}`)).Created[0], "0s",
		"later"); err != nil {
		t.Fatal(err)
	}

	row, err := m.store.Jobs().Get(context.Background(), job.JobID)
	if err != nil {
		t.Fatal(err)
	}

	var in struct {
		Nested map[string]json.RawMessage `json:"nested"`
	}

	if err := json.Unmarshal(row.Input, &in); err != nil {
		t.Fatal(err)
	}

	kept, err := timeline.Decode(in.Nested[child])
	if err != nil {
		t.Fatalf("the job keeps no document of the nested timeline: %v", err)
	}

	if len(kept.Items) != 1 || m.head(child).Revision == asked {
		t.Fatalf("the job keeps the nested timeline with %d items, want the one of revision %d", len(kept.Items), asked)
	}

	if err := m.runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	if done := m.job(job.JobID); done.State != service.JobSucceeded {
		t.Fatalf("job = %+v", done)
	}
}

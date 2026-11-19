package servicetest

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// A second timeline of a production is not a row of the projects table: its checkpoints, the automatic one an
// agent's first edit makes, the revert and the diff all key by the id of the timeline (SDD-003 section 1.7, SDD-005
// section 5.14). The first version of the table referenced projects and failed here with E_INTERNAL.
func TestCheckpointOnASecondTimeline(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, _ := e.withTrack()

	added, err := e.prod().ProjectItemAdd(
		e.as(ui), service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Wide", Preset: "16:9"},
	)
	if err != nil {
		t.Fatal(err)
	}

	second := added.Item.TimelineID

	track, err := e.svc.Edit(e.as(ui), service.EditRequest{
		TimelineID: second, BaseRevision: e.head(second).Revision,
		Operations: []command.Operation{op("track.add", `{"kind":"visual","name":"V"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}

	text := func(actor, start string, base int64) *service.EditResult {
		t.Helper()

		res, err := e.svc.Edit(e.as(actor), service.EditRequest{
			TimelineID: second, BaseRevision: base,
			Operations: []command.Operation{op("text.add",
				fmt.Sprintf(`{"track_id":%q,"start":%q,"duration":"1s","text":"x"}`, track.Created[0], start))},
		})
		if err != nil {
			t.Fatal(err)
		}

		return res
	}

	first := text(agent, "0s", track.Revision) // the agent's first edit of the day: the automatic checkpoint
	atFirst := e.document(second)
	text(agent, "2s", first.Revision)

	list, err := e.svc.CheckpointList(e.as(ui), service.CheckpointListRequest{TimelineID: second})
	if err != nil || len(list.Checkpoints) != 1 || !list.Checkpoints[0].Auto ||
		list.Checkpoints[0].Revision != track.Revision {
		t.Fatalf("the automatic checkpoint of the second timeline = %+v, %v", list, err)
	}

	named, err := e.svc.CheckpointCreate(e.as(ui), service.CheckpointCreateRequest{
		ProjectID: project, TimelineID: second, Name: "one text", Revision: first.Revision,
	})
	if err != nil || named.Revision != first.Revision {
		t.Fatalf("checkpoint = %+v, %v", named, err)
	}

	// The first timeline has none of them.
	other, err := e.svc.CheckpointList(e.as(ui), service.CheckpointListRequest{TimelineID: project})
	if err != nil || len(other.Checkpoints) != 0 {
		t.Fatalf("the first timeline has checkpoints: %+v, %v", other, err)
	}

	diff, err := e.svc.HistoryDiff(e.as(ui), service.HistoryDiffRequest{TimelineID: second, FromCheckpoint: "one text"})
	if err != nil || diff.Count != 1 {
		t.Fatalf("diff = %+v, %v", diff, err)
	}

	res, err := e.svc.HistoryRevert(e.as(ui), service.HistoryRevertRequest{TimelineID: second, Checkpoint: "one text"})
	if err != nil || !res.Applied {
		t.Fatalf("revert = %+v, %v", res, err)
	}

	if !bytes.Equal(e.document(second), atFirst) {
		t.Fatal("the second timeline is not the document at the checkpoint")
	}
}

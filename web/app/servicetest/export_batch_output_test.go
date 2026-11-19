package servicetest

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// The file of each export of a batch is an output piece of its production (SDD-004 section 8.4, SDD-005 section
// 5.12): the job_id of a child of the batch is a succeeded export job of the project, and ProjectItemAdd files it
// as an output item. The parent is not an export job and is not a piece.
func TestExportBatchChildIsAnOutputItem(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, exporter: goodExporter()})
	project := m.exportable()
	ctx := m.as(ui)

	job, err := m.svc.ExportBatch(ctx, service.ExportBatchRequest{
		ProjectID: project, Items: []service.ExportBatchItem{{Preset: "tiktok"}, {Format: "mp4"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := m.runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	done := m.job(job.JobID)
	if done.State != service.JobSucceeded || done.Result == nil || done.Result.Batch == nil ||
		done.Result.Batch.Succeeded != 2 {
		t.Fatalf("the batch: %s %+v %+v", done.State, done.Result, done.Error)
	}

	prod, ok := m.svc.(service.ProductionService)
	if !ok {
		t.Fatal("the service has no production operations")
	}

	for i, it := range done.Result.Batch.Items {
		res, err := prod.ProjectItemAdd(ctx, service.ProjectItemAddRequest{
			ProjectID: project, Kind: "output", Title: "Cut " + it.Format, JobID: it.JobID,
		})
		if err != nil || res.Item.JobID != it.JobID || res.Item.Format != "mp4" || res.Production.Outputs != i+1 {
			t.Fatalf("output of child %d (%s): %+v %v", i, it.JobID, res, err)
		}
	}

	if _, err := prod.ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: project, Kind: "output", Title: "Batch", JobID: job.JobID,
	}); code(err) != "E_NOT_FOUND" {
		t.Errorf("the parent of the batch as an output: %v, want E_NOT_FOUND", err)
	}
}

func pathOf(err error) string {
	if se, ok := errors.AsType[*service.Error](err); ok {
		return se.Path
	}

	return ""
}

// ExportCheck and ExportBatch follow SDD-003 section 1.7: on a project with several timelines they need
// timeline_id and refuse to guess; with it they check and export that timeline.
func TestExportCheckAndBatchNameTheTimeline(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, exporter: goodExporter()})
	project, second := m.productionProject()
	ctx := m.as(ui)

	if _, err := m.svc.ExportCheck(ctx, service.ExportCheckRequest{ProjectID: project, Preset: "tiktok"}); code(
		err,
	) != "E_VALIDATION" || pathOf(err) != "timeline_id" {
		t.Errorf("a check of a project of two timelines: %v", err)
	}

	if _, err := m.svc.ExportBatch(ctx, service.ExportBatchRequest{
		ProjectID: project, Items: []service.ExportBatchItem{{Preset: "tiktok"}},
	}); code(err) != "E_VALIDATION" || pathOf(err) != "timeline_id" {
		t.Errorf("a batch of a project of two timelines: %v", err)
	}

	// The second timeline is 16:9: YouTube Shorts takes square or vertical pictures only.
	if _, err := m.svc.Edit(ctx, service.EditRequest{
		TimelineID: second, BaseRevision: m.head(second).Revision, Operations: []command.Operation{
			op("track.add", `{"kind":"visual","id":"trk_0000000035"}`),
			op("text.add", `{"track_id":"trk_0000000035","start":"0s","duration":"2s","text":"wide"}`),
		},
	}); err != nil {
		t.Fatal(err)
	}

	res, err := m.svc.ExportCheck(ctx, service.ExportCheckRequest{
		ProjectID: project, TimelineID: second, Preset: "youtube-shorts",
	})
	if err != nil || res.ProjectID != second || res.OK || res.Findings[0].Code != "X_ASPECT_RATIO" {
		t.Fatalf("the check of the second timeline: %+v %v", res, err)
	}
}

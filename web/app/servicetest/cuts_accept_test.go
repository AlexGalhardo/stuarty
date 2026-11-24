package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/cuts"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

// The accept flow of the editor on the real service (SDD-024 section 8 item 5), as a person runs it: a new
// timeline item, the media file of the answer placed in it, then the batch of a proposal sent as it came at the
// revision the placement answered. The speechProject asset is 2 s long, so the proposal is one within it.
func TestCutsAcceptedBatchMakesTheCutOnANewTimeline(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, clip, _, hash := m.speechProject()
	m.putLongTalk(hash)
	m.putFaces(hash, 0, 0)

	assetID := m.assetOf(project, clip)
	first := m.head(project).Revision

	answer, err := m.svc.CutsPropose(m.as(ui), service.CutsProposeRequest{
		ProjectID: project, AssetID: assetID, MinMS: 8_000, MaxMS: 12_000,
	})
	if err != nil {
		t.Fatal(err)
	}

	if answer.MediaFile.Hash != hash || answer.MediaFile.AssetID != assetID {
		t.Fatalf("media_file = %+v, want the hash and the id of the asset", answer.MediaFile)
	}

	for _, p := range answer.Proposals {
		for _, op := range p.Batch {
			if op.Op == "asset.add" {
				t.Fatalf("a proposal carries asset.add, which only the import flow may issue: %+v", p.Batch)
			}
		}
	}

	added, err := m.prod().ProjectItemAdd(m.as(ui), service.ProjectItemAddRequest{
		ProjectID: project, Kind: "timeline", Title: "Cut 1",
	})
	if err != nil {
		t.Fatal(err)
	}

	tl := added.Item.TimelineID

	placed, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, TimelineID: tl, Files: []service.ImportFile{answer.MediaFile},
	})
	if err != nil || len(placed.Assets) != 1 || placed.Assets[0].Error != nil ||
		placed.Assets[0].AssetID != assetID {
		t.Fatalf("placing the asset = %+v, %v", placed, err)
	}

	if head := m.head(tl).Revision; placed.Revision != head {
		t.Errorf("the placement answers r%d, the timeline is at r%d", placed.Revision, head)
	}

	batch := cuts.Batch(&cuts.Proposal{N: 1, StartMS: 0, EndMS: 1500}, answer.MediaFile.AssetID)

	if _, err := m.svc.Edit(m.as(ui), service.EditRequest{
		ProjectID: project, TimelineID: tl, BaseRevision: placed.Revision, Operations: batch,
		IdempotencyKey: "accept-cut-0001",
	}); err != nil {
		t.Fatalf("the batch of a proposal was refused on its new timeline: %v", err)
	}

	doc := m.docOf(tl)
	if doc.Settings.Width >= doc.Settings.Height {
		t.Errorf("the new timeline is %dx%d, not vertical", doc.Settings.Width, doc.Settings.Height)
	}

	it, ok := doc.Items[cuts.ClipID]
	if !ok || it.Asset != assetID || it.Fit != "cover" || len(it.Keyframes["crop_left"]) == 0 {
		t.Fatalf("the clip of the cut = %+v", it)
	}

	for _, p := range timeline.CropProps {
		for _, k := range it.Keyframes[p] {
			if k.Value < 0 || k.Value > 1 {
				t.Errorf("%s keyframe %+v is outside the frame", p, k)
			}
		}
	}

	if after := m.head(project).Revision; after != first {
		t.Errorf("the first timeline went from r%d to r%d", first, after)
	}
}

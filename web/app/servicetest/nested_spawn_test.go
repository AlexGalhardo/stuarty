package servicetest

import (
	"context"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// TestNestedSpawnedTimelineHoldsItsAssets: the timeline that composition.create_from_selection makes holds the
// assets of the clips it takes, so their files are served and inspected under it.
func TestNestedSpawnedTimelineHoldsItsAssets(t *testing.T) {
	t.Parallel()

	m := newMediaEnv(t, mediaCfg{})
	project, track := m.withTrack()
	hash := m.importOne(project, m.write("a.mp4", clipBytes+"a")).Assets[0].Hash

	pa, err := m.store.Assets().FindProjectAssetByHash(context.Background(), project, hash)
	if err != nil {
		t.Fatal(err)
	}

	clip := m.mustEdit(ui, project, m.head(project).Revision, op("clip.add",
		fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"1s"}`, track, pa.AssetID))).Created[0]

	made := m.mustEdit(ui, project, m.head(project).Revision, op("composition.create_from_selection",
		fmt.Sprintf(`{"item_ids":[%q],"name":"Intro"}`, clip)))

	intro := m.timelinesOf(project)["Intro"]
	if intro == "" || len(made.Created) == 0 {
		t.Fatalf("no timeline was made: %+v", made)
	}

	row, err := m.store.Assets().GetProjectAsset(context.Background(), intro, pa.AssetID)
	if err != nil || row.AssetHash != hash || row.State != service.ProjectAssetActive {
		t.Fatalf("the new timeline holds the asset as %+v (%v), want the active row of %s", row, err, hash)
	}

	if _, err := m.svc.MediaInspect(m.as(ui), service.MediaInspectRequest{
		ProjectID: intro, TimelineID: intro, AssetID: pa.AssetID,
	}); err != nil {
		t.Errorf("the asset cannot be inspected under the new timeline: %v", err)
	}
}

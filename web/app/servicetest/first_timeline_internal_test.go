package servicetest

import (
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/service"
)

// The first timeline of a production has the id of the project, so an internal call that names only that id is
// ambiguous once the production holds a second timeline (SDD-003 section 1.7). Each operation below works on the
// first timeline of a production of two, named by its timeline_id, as it does on a project of one.

// addSecondTimeline gives the project a second timeline and returns its id.
func (m *mediaEnv) addSecondTimeline(project string) string {
	m.t.Helper()

	added, err := m.prod().ProjectItemAdd(
		m.as(ui), service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Second", Preset: "16:9"},
	)
	if err != nil {
		m.t.Fatal(err)
	}

	return added.Item.TimelineID
}

func TestOtioImportIntoTheFirstOfTwoTimelines(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.otioSource()
	file := m.write("source.otio", m.otioExport(project).Content)
	m.addSecondTimeline(project)

	res, err := m.svc.ProjectOtioImport(m.as(ui), service.ProjectOtioImportRequest{
		ProjectID: project, TimelineID: project, BaseRevision: m.head(project).Revision, Path: file,
	})
	if err != nil || res.Revision != m.head(project).Revision {
		t.Fatalf("import into the first timeline = %+v, %v", res, err)
	}
}

func TestMediaTrashAndRestoreInTheFirstOfTwoTimelines(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("trash")
	m.addSecondTimeline(project)

	imported, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, TimelineID: project, WaitMS: 20000,
		Files: []service.ImportFile{{Path: m.write("clip.mp4", clipBytes)}},
	})
	if err != nil || imported.Failed != 0 {
		t.Fatalf("import = %+v, %v", imported, err)
	}

	req := service.MediaTrashRequest{ProjectID: project, TimelineID: project, AssetID: imported.Assets[0].AssetID}

	if res, err := m.svc.MediaTrash(m.as(ui), req); err != nil || res.State != "trashed" {
		t.Fatalf("trash = %+v, %v", res, err)
	}

	if res, err := m.svc.MediaRestore(m.as(ui), req); err != nil || res.State != "active" {
		t.Fatalf("restore = %+v, %v", res, err)
	}
}

func TestMotionReplaceInTheFirstOfTwoTimelines(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project, track := m.withTrack()

	importAs := func(name string) string {
		t.Helper()

		res, err := m.svc.MediaImport(ctx, service.MediaImportRequest{
			ProjectID: project, WaitMS: 20000, Files: []service.ImportFile{{Path: m.write(name, clipBytes+name)}},
			Origin: &asset.Origin{Kind: asset.RenderedExternally, Source: "engine title", RenderKey: "title"},
		})
		if err != nil || res.Failed != 0 {
			t.Fatalf("import of %s = %+v, %v", name, res, err)
		}

		return res.Assets[0].AssetID
	}

	first := importAs("title-v1.mov")
	m.mustEdit(ui, project, m.head(project).Revision, op("clip.add",
		fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"1s"}`, track, first)))

	second := importAs("title-v2.mov")
	m.addSecondTimeline(project)

	res, err := m.svc.MotionReplace(ctx, service.MotionReplaceRequest{
		ProjectID: project, TimelineID: project, AssetID: second,
	})
	if err != nil || !res.Applied || len(res.Replaced) != 1 {
		t.Fatalf("replacement in the first timeline = %+v, %v", res, err)
	}
}

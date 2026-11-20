package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// VV-318: a file imported into either timeline of a production that holds two is a file of the library, the first
// timeline (whose id is the project's) as much as the second.
func TestAssetLibraryListsAFileOfEitherTimelineOfAProduction(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("Two")

	added, err := m.prod().ProjectItemAdd(
		ctx, service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Second", Preset: "16:9"},
	)
	if err != nil {
		t.Fatal(err)
	}

	for name, timeline := range map[string]string{"first": project, "second": added.Item.TimelineID} {
		res, err := m.svc.MediaImport(ctx, service.MediaImportRequest{
			ProjectID: project, TimelineID: timeline,
			Files: []service.ImportFile{{Path: m.write(name+".mp4", clipBytes+name)}},
		})
		if err != nil || res.Failed != 0 {
			t.Fatalf("import into the %s timeline = %+v, %v", name, res, err)
		}

		list, err := m.svc.AssetList(ctx, service.AssetListRequest{Text: name})
		if err != nil || list.Total != 1 {
			t.Errorf("the library after an import into the %s timeline = %+v, %v", name, list, err)
		}
	}
}

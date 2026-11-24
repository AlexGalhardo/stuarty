package servicetest

import (
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// The stored kind travels in a bundle (SDD-005 section 16.1.1): an image project that holds only a text is an image
// again after the import, not the text its items would make it. A project that chose nothing leaves the member out,
// so a build made before epic 78 reads the same manifest.
func TestBundleCarriesTheStoredKind(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	ctx := src.as(ui)

	export := func(name, kind string, withText bool) (string, service.BundleManifest) {
		t.Helper()

		req := testproject.Request(name)
		req.Kind = kind

		made, err := src.svc.ProjectCreate(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		if withText {
			if _, err := src.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{
				ProjectID: made.ProjectID, Kind: "text", Title: "Alt text", Text: "A cover for the launch.",
			}); err != nil {
				t.Fatal(err)
			}
		} else if _, err := src.prod().BriefSet(ctx, service.BriefSetRequest{
			ProjectID: made.ProjectID, Text: "The plan.",
		}); err != nil {
			t.Fatal(err)
		}

		path := src.exportBundleRequest(service.ProjectBundleExportRequest{ProjectID: made.ProjectID})
		_, files := readZip(t, path)

		var man service.BundleManifest
		if err := json.Unmarshal(files["manifest.json"], &man); err != nil {
			t.Fatal(err)
		}

		return path, man
	}

	imagePath, man := export("Launch cover", "image", true)
	if man.Production == nil || man.Production.Kind != "image" {
		t.Fatalf("the manifest does not carry the kind: %+v", man.Production)
	}

	_, undecided := export("Not chosen", "", false)
	if undecided.Production == nil || undecided.Production.Kind != "" {
		t.Fatalf("an undecided project wrote a kind: %+v", undecided.Production)
	}

	dst := newMediaEnv(t, mediaCfg{})

	res, err := dst.importBundle(imagePath, true)
	if err != nil {
		t.Fatal(err)
	}

	if res.Kind != "image" {
		t.Errorf("the import answers the kind %q, want image", res.Kind)
	}

	got, err := dst.prod().ProjectDescribe(dst.as(ui), service.ProjectDescribeRequest{ProjectID: res.ProjectID})
	if err != nil {
		t.Fatal(err)
	}

	if got.Kind != "image" || got.Workflow.Kind != "image" {
		t.Errorf("the imported project reads %q (workflow %q), want image", got.Kind, got.Workflow.Kind)
	}
}

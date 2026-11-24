package servicetest

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// A project made of texts only has no document: its bundle is the project and its items, with no document.json,
// and it comes back whole in another workspace (VV-358, SDD-005 section 16.1.2).
func TestBundleOfAProjectWithNoTimelineRoundTrips(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	ctx := src.as(ui)

	made, err := testproject.Create(ctx, src.svc, "Only words")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := src.prod().BriefSet(ctx, service.BriefSetRequest{ProjectID: made.ProjectID, Text: "The plan."}); err != nil {
		t.Fatal(err)
	}

	if _, err := src.prod().ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: made.ProjectID, Kind: "text", Title: "The post", Text: "Hello, readers.", TextKind: "copy",
	}); err != nil {
		t.Fatal(err)
	}

	path := src.exportBundleRequest(service.ProjectBundleExportRequest{ProjectID: made.ProjectID})

	for _, e := range bundleEntries(t, path) {
		if e.name == "document.json" || e.name == "operations.ndjson" {
			t.Fatalf("the bundle of a project with no timeline holds %s", e.name)
		}
	}

	for _, asNew := range []bool{true, false} {
		dst := newMediaEnv(t, mediaCfg{})

		res, err := dst.importBundle(path, asNew)
		if err != nil {
			t.Fatalf("import as_new=%v: %v", asNew, err)
		}

		if !asNew && res.ProjectID != made.ProjectID {
			t.Errorf("the id was not kept: %s, want %s", res.ProjectID, made.ProjectID)
		}

		got, err := dst.prod().ProjectDescribe(dst.as(ui), service.ProjectDescribeRequest{ProjectID: res.ProjectID})
		if err != nil {
			t.Fatal(err)
		}

		if got.Name != "Only words" || got.Description != testproject.Description || got.Goal != testproject.Goal ||
			!got.Complete {
			t.Fatalf("the context did not travel: %+v", got)
		}

		var kinds []string

		for _, it := range got.Items {
			body := ""

			switch it.Kind {
			case "brief":
				b, err := dst.prod().BriefGet(dst.as(ui), service.BriefGetRequest{ProjectID: res.ProjectID})
				if err != nil {
					t.Fatal(err)
				}

				body = b.Text
			case "text":
				tx, err := dst.svc.TextGet(dst.as(ui), service.TextGetRequest{ProjectID: res.ProjectID, ItemID: it.ID})
				if err != nil {
					t.Fatal(err)
				}

				body = tx.Body
			default:
				t.Errorf("the import made an item of kind %s: %+v", it.Kind, it)
			}

			kinds = append(kinds, it.Kind+":"+body)
		}

		if want := "brief:The plan.|text:Hello, readers."; strings.Join(kinds, "|") != want {
			t.Errorf("items = %v, want %s", kinds, want)
		}

		if _, err := dst.importBundle(path, false); !asNew && !isCode(err, "E_ID_EXISTS") {
			t.Errorf("a second import with the same id: %v, want E_ID_EXISTS", err)
		}
	}
}

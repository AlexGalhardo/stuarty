package service

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
)

func v2Manifest() *BundleManifest {
	sha := strings.Repeat("a", 64)

	return &BundleManifest{
		Format: BundleFormat, BundleVersion: 2, Project: BundleProject{ID: "prj_0000000001"},
		Document: &BundleDocument{SHA256: sha, Bytes: 10},
		Production: &BundleProduction{Items: []BundleItem{
			{Kind: "timeline", Title: "Main", Timeline: "main"},
			{Kind: "timeline", Title: "Wide", Position: 1, Timeline: "t1"},
			{Kind: "brief", Title: "Brief", Position: 2, Text: &BundleDocument{SHA256: sha, Bytes: 4}},
			{Kind: "output", Title: "Final", Position: 3, Format: "mp4"},
		}},
		Timelines: []BundleTimeline{{
			Key: "t1", ID: "prj_0000000002", SchemaVersion: 1, Revision: 2,
			Document:   BundleDocument{SHA256: sha, Bytes: 10},
			Operations: &BundleLog{Count: 2, SHA256: sha},
		}},
	}
}

// TestBundleProductionManifestRules is the judgement of the members of version 2 before any entry is read.
func TestBundleProductionManifestRules(t *testing.T) {
	t.Parallel()

	if e := checkBundleProduction(v2Manifest()); e != nil {
		t.Fatalf("a good manifest was refused: %v", e)
	}

	v1 := v2Manifest()
	v1.Production, v1.Timelines = nil, nil

	if e := checkBundleProduction(v1); e != nil {
		t.Fatalf("a manifest of version 1 was refused: %v", e)
	}

	bad := map[string]func(m *BundleManifest){
		"timelines without a production":   func(m *BundleManifest) { m.Production = nil },
		"a bad key":                        func(m *BundleManifest) { m.Timelines[0].Key = "x1" },
		"the key of the first":             func(m *BundleManifest) { m.Timelines[0].Key = "main" },
		"a bad timeline id":                func(m *BundleManifest) { m.Timelines[0].ID = "tl_1" },
		"the id of the project":            func(m *BundleManifest) { m.Timelines[0].ID = "prj_0000000001" },
		"a bad document hash":              func(m *BundleManifest) { m.Timelines[0].Document.SHA256 = "zz" },
		"a bad log hash":                   func(m *BundleManifest) { m.Timelines[0].Operations.SHA256 = "zz" },
		"a negative revision":              func(m *BundleManifest) { m.Timelines[0].Revision = -1 },
		"a timeline no item names":         func(m *BundleManifest) { m.Production.Items[1].Timeline = "main" },
		"a timeline an item names twice":   func(m *BundleManifest) { m.Production.Items[0].Timeline = "t1" },
		"an item that names an unknown":    func(m *BundleManifest) { m.Production.Items[1].Timeline = "t5" },
		"a text item that names one":       func(m *BundleManifest) { m.Production.Items[2].Timeline = "t1" },
		"two briefs":                       func(m *BundleManifest) { m.Production.Items = append(m.Production.Items, m.Production.Items[2]) },
		"a reserved kind":                  func(m *BundleManifest) { m.Production.Items[3].Kind = "publication" },
		"an unknown kind":                  func(m *BundleManifest) { m.Production.Items[3].Kind = "video" },
		"a title with a control character": func(m *BundleManifest) { m.Production.Items[2].Title = "a\x00" },
		"a text over the limit":            func(m *BundleManifest) { m.Production.Items[2].Text.Bytes = 256<<10 + 1 },
		"an empty text entry":              func(m *BundleManifest) { m.Production.Items[2].Text.Bytes = 0 },
		"a bad text hash":                  func(m *BundleManifest) { m.Production.Items[2].Text.SHA256 = "zz" },
		"a text on an output": func(m *BundleManifest) {
			m.Production.Items[3].Text = &BundleDocument{SHA256: strings.Repeat("a", 64), Bytes: 1}
		},
		"a long format":       func(m *BundleManifest) { m.Production.Items[3].Format = strings.Repeat("a", 17) },
		"a short description": func(m *BundleManifest) { m.Production.Description = "too short" },
		"a goal with a short part": func(m *BundleManifest) {
			m.Production.Goal = &project.Goal{Specific: "short", Due: "2027-01-01"}
		},
		"a project name with a control character": func(m *BundleManifest) { m.Production.Name = "a\x00b" },
		"a document in version 3":                 func(m *BundleManifest) { m.BundleVersion = 3 },
		"too many items": func(m *BundleManifest) {
			for range bundleItemsMax {
				m.Production.Items = append(m.Production.Items, BundleItem{Kind: "text", Title: "t"})
			}
		},
	}

	for name, edit := range bad {
		m := v2Manifest()
		edit(m)

		if e := checkBundleProduction(m); e == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// TestBundleWithNoDocumentManifestRules is the judgement of the manifest of a project with no timeline.
func TestBundleWithNoDocumentManifestRules(t *testing.T) {
	t.Parallel()

	bare := func() *BundleManifest {
		return &BundleManifest{
			Format: BundleFormat, BundleVersion: 3, Project: BundleProject{ID: "prj_0000000001", Name: "Only words"},
			Production: &BundleProduction{Name: "Only words", Items: []BundleItem{{Kind: "text", Title: "Post"}}},
		}
	}

	if e := checkBundleProduction(bare()); e != nil {
		t.Fatalf("a bundle with no timeline was refused: %v", e)
	}

	bad := map[string]func(m *BundleManifest){
		"version 1":        func(m *BundleManifest) { m.BundleVersion = 1 },
		"version 2":        func(m *BundleManifest) { m.BundleVersion = 2 },
		"no production":    func(m *BundleManifest) { m.Production = nil },
		"a timeline":       func(m *BundleManifest) { m.Timelines = v2Manifest().Timelines },
		"a log":            func(m *BundleManifest) { m.Operations = &BundleLog{Count: 1, SHA256: strings.Repeat("a", 64)} },
		"a media file":     func(m *BundleManifest) { m.Media = []BundleMedia{{Hash: "sha256:" + strings.Repeat("a", 64)}} },
		"a bad project id": func(m *BundleManifest) { m.Project.ID = "tl_1" },
		"no name":          func(m *BundleManifest) { m.Production.Name, m.Project.Name = "", "" },
		"a bad name in project": func(m *BundleManifest) {
			m.Production.Name, m.Project.Name = "", "a‮b"
		},
		"a long name in project": func(m *BundleManifest) {
			m.Production.Name, m.Project.Name = "", strings.Repeat("a", 121)
		},
		"a timeline item": func(m *BundleManifest) {
			m.Production.Items[0] = BundleItem{Kind: "timeline", Title: "Main", Timeline: "main"}
		},
		"a timeline item t1": func(m *BundleManifest) {
			m.Production.Items[0] = BundleItem{Kind: "timeline", Title: "Main", Timeline: "t1"}
		},
	}

	for name, edit := range bad {
		m := bare()
		edit(m)

		if e := checkBundleProduction(m); e == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

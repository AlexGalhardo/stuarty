package servicetest

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// An OpenTimelineIO export is an export: it names the file by its new name, and a file written with the new name or
// before the rename still finds its asset.
func TestAssetRenameReachesOtio(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.otioSource()
	before := m.write("before.otio", m.otioExport(project).Content)

	list, err := m.svc.AssetList(m.as(ui), service.AssetListRequest{})
	if err != nil || len(list.Assets) != 1 {
		t.Fatalf("library = %+v, %v", list, err)
	}

	if _, err := m.svc.AssetRename(m.as(ui), service.AssetRenameRequest{
		Hash: list.Assets[0].Hash, Name: "Opening shot",
	}); err != nil {
		t.Fatal(err)
	}

	out := m.otioExport(project).Content
	if !strings.Contains(out, "Opening shot.mp4") || strings.Contains(out, `"clip.mp4"`) {
		t.Errorf("the export does not carry the new name: %.400s", out)
	}

	for _, file := range []string{m.write("after.otio", out), before} {
		res, ierr := m.otioImport(project, m.head(project).Revision, file)
		if ierr != nil || len(res.UnresolvedMedia) != 0 {
			t.Errorf("import of %s = %+v, %v", file, res, ierr)
		}
	}
}

// A bundle carries the name and an import of it brings the name back; a name in a manifest is read as any file name.
func TestAssetRenameSurvivesABundle(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project, hashes := m.bundleProject()

	if _, err := m.svc.AssetRename(ctx, service.AssetRenameRequest{Hash: hashes[0], Name: "Opening shot"}); err != nil {
		t.Fatal(err)
	}

	job := m.exportBundle(project, nil)
	if job.State != service.JobSucceeded || job.Result == nil || job.Result.Export == nil {
		t.Fatalf("job = %+v", job)
	}

	res, err := m.importBundle(job.Result.Export.Path, true)
	if err != nil {
		t.Fatal(err)
	}

	list, err := m.svc.MediaList(ctx, service.MediaListRequest{ProjectID: res.ProjectID})
	if err != nil {
		t.Fatal(err)
	}

	found := false

	for _, a := range list.Assets {
		found = found || a.Name == "Opening shot.mp4"
	}

	if !found {
		t.Errorf("the imported project lists %+v", list.Assets)
	}
}

// The check of a render names the file as the library does.
func TestAssetRenameIsInTheMotionCheck(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project, upper, _ := m.motionFit()
	in := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0]

	if _, err := m.svc.AssetRename(ctx, service.AssetRenameRequest{Hash: in.Hash, Name: "Render"}); err != nil {
		t.Fatal(err)
	}

	res, err := m.svc.MotionCheck(ctx, service.MotionCheckRequest{
		ProjectID: project, AssetID: in.AssetID, Until: "1s", TrackID: upper,
	})
	if err != nil || res.AssetName != "Render.mp4" {
		t.Fatalf("check = %+v, %v", res, err)
	}
}

// What an agent reads of a project names the file as the library does; the document keeps what it was written with.
func TestAssetRenameIsInTheProjectView(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.otioSource()

	list, err := m.svc.AssetList(ctx, service.AssetListRequest{})
	if err != nil || len(list.Assets) != 1 {
		t.Fatalf("library = %+v, %v", list, err)
	}

	before := bytes.Clone(m.document(project))

	renamed := service.AssetRenameRequest{Hash: list.Assets[0].Hash, Name: "Beach"}
	if _, err := m.svc.AssetRename(ctx, renamed); err != nil {
		t.Fatal(err)
	}

	view, err := m.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: project, Detail: "full"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(view.Text, "Beach.mp4") || strings.Contains(view.Text, `"clip.mp4"`) {
		t.Errorf("the view reads: %s", view.Text)
	}

	if !bytes.Equal(before, m.document(project)) {
		t.Errorf("a rename wrote the document")
	}
}

// One file held under two names by two timelines of a project keeps both through a bundle, renamed or not: the
// manifest has one entry per file, and the name of that entry is not the name of every holding.
func TestBundleKeepsTheNameOfEachHolding(t *testing.T) {
	for _, tc := range []struct {
		name, rename, first, second string
	}{
		{name: "never renamed", first: "a.mp4", second: "take.mov"},
		{name: "renamed", rename: "Beach", first: "Beach.mp4", second: "Beach.mov"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := newMediaEnv(t, mediaCfg{})
			project, second := src.productionProject()
			held := src.importOne(second, src.write("take.mov", clipBytes+"a.mp4")).Assets[0]

			if tc.rename != "" {
				if _, err := src.svc.AssetRename(src.as(ui), service.AssetRenameRequest{
					Hash: held.Hash, Name: tc.rename,
				}); err != nil {
					t.Fatal(err)
				}
			}

			path := src.exportBundleRequest(service.ProjectBundleExportRequest{ProjectID: project})
			dst := newMediaEnv(t, mediaCfg{})

			if _, err := dst.importBundle(path, false); err != nil {
				t.Fatal(err)
			}

			names := func(timeline string) []string {
				list, err := dst.svc.MediaList(dst.as(ui), service.MediaListRequest{
					ProjectID: project, TimelineID: timeline,
				})
				if err != nil {
					t.Fatal(err)
				}

				var out []string

				for _, a := range list.Assets {
					if a.AssetID == held.AssetID || a.Name == tc.first {
						out = append(out, a.Name)
					}
				}

				return out
			}

			if got := names(project); !slices.Contains(got, tc.first) {
				t.Errorf("the first timeline holds %q, want %q", got, tc.first)
			}

			if got := names(second); !slices.Equal(got, []string{tc.second}) {
				t.Errorf("the second timeline holds %q, want %q", got, tc.second)
			}
		})
	}
}

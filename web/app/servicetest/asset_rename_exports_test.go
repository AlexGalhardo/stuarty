package servicetest

import (
	"bytes"
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

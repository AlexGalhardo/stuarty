package service

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// D-111: an image export records a succeeded job of kind image.export with no project; the job is visible to
// whoever may see the image and to nobody else, and a publication takes its file as a thumbnail.
func TestImageExportRecordsAJobThatOnlyTheOwnerOfTheImageMayUse(t *testing.T) {
	s, _ := newImageService(t)
	alice, bob := networkUser("alice"), networkUser("bob")

	created, err := s.ImageCreate(alice, ImageCreateRequest{Width: 64, Height: 48})
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.ImageExport(alice, ImageExportRequest{ImageID: created.ImageID})
	if err != nil || res.JobID == "" {
		t.Fatalf("ImageExport = %+v, %v", res, err)
	}

	if text := ImageExportText(res); !strings.Contains(text, res.JobID) {
		t.Errorf("the text of the export does not name its job: %q", text)
	}

	row, err := s.store.Jobs().Get(context.Background(), res.JobID)
	if err != nil || row.Kind != JobImageExport || row.State != JobSucceeded || row.ProjectID != nil ||
		row.AssetHash != nil {
		t.Fatalf("the job = %+v, %v", row, err)
	}

	var in imageExportInput
	if err := json.Unmarshal(row.Input, &in); err != nil || in.ImageID != created.ImageID || in.OwnerUserID != "alice" {
		t.Fatalf("the job input = %s, %v: it must name the image and the user who exported it", row.Input, err)
	}

	var stored JobResult
	if err := json.Unmarshal(row.Result, &stored); err != nil || stored.Image == nil ||
		filepath.IsAbs(stored.Image.Path) || stored.Image.SHA256 != res.SHA256 {
		t.Fatalf("the job result = %s, %v: it must hold the digest and no absolute path", row.Result, err)
	}

	if !s.canSee(alice, mustCaller(alice, t), &row) {
		t.Fatal("the owner of the image cannot see its export job")
	}

	file, err := s.publishFile(alice, mustCaller(alice, t), "prj_x", "thumbnail", res.JobID)
	if err != nil || file.SHA256 != res.SHA256 || file.Format != "png" || file.Bytes != res.Bytes {
		t.Fatalf("publishFile = %+v, %v", file, err)
	}

	if _, err := s.publishFile(alice, mustCaller(alice, t), "prj_x", "export", res.JobID); err == nil {
		t.Error("an image export was taken as the video of a publication")
	}

	if _, err := s.publishFile(bob, mustCaller(bob, t), "prj_x", "thumbnail", res.JobID); err == nil {
		t.Error("another user took the image export as a thumbnail")
	}

	limited := mustCaller(alice, t)
	limited.ProjectID = "prj_x"

	if _, err := s.publishFile(alice, limited, "prj_x", "thumbnail", res.JobID); err == nil {
		t.Error("a key limited to a project took an image export as a thumbnail")
	}

	// The file the job recorded changed: the publication refuses it.
	if err := os.WriteFile(filepath.Join(s.workspace, stored.Image.Path), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.publishFile(alice, mustCaller(alice, t), "prj_x", "thumbnail", res.JobID); err == nil {
		t.Error("a changed file was taken as a thumbnail")
	}
}

// T-06: the export one user made of a template every user may read is that user's: another user may neither read
// the job nor take it as a thumbnail, and a local caller may (D-111 review).
func TestSecurity_T06_ImageExportJobIsOnlyItsExporters(t *testing.T) {
	s, _ := newImageService(t)
	alice, bob := networkUser("alice"), networkUser("bob")

	tpl, err := s.ImageCreate(asActor("local:starter"), ImageCreateRequest{Width: 64, Height: 48, Template: true})
	if err != nil {
		t.Fatal(err)
	}

	st := s.store.(*memStore)
	st.images.rows[tpl.ImageID].CreatedBy = StarterActor

	if row, err := st.images.Get(context.Background(), tpl.ImageID); err != nil || !SharedImageTemplate(row.ImageHead) {
		t.Fatalf("the template is not shared: %+v, %v", row.ImageHead, err)
	}

	res, err := s.ImageExport(bob, ImageExportRequest{ImageID: tpl.ImageID})
	if err != nil || res.JobID == "" {
		t.Fatalf("bob exports the shared template = %+v, %v", res, err)
	}

	row, err := s.store.Jobs().Get(context.Background(), res.JobID)
	if err != nil {
		t.Fatal(err)
	}

	if !s.canSee(bob, mustCaller(bob, t), &row) {
		t.Error("bob cannot see his own export of the shared template")
	}

	if s.canSee(alice, mustCaller(alice, t), &row) {
		t.Error("alice sees the export bob made of the shared template")
	}

	if _, err := s.JobGet(alice, JobGetRequest{JobID: res.JobID}); err == nil {
		t.Error("alice reads the export job of bob")
	}

	if _, err := s.publishFile(alice, mustCaller(alice, t), "prj_x", "thumbnail", res.JobID); err == nil {
		t.Error("alice took the export of bob as a thumbnail")
	}

	if _, err := s.publishFile(bob, mustCaller(bob, t), "prj_x", "thumbnail", res.JobID); err != nil {
		t.Errorf("bob cannot take his export of the shared template as a thumbnail: %v", err)
	}

	local := asActor("local:ui")
	if _, err := s.publishFile(local, mustCaller(local, t), "prj_x", "thumbnail", res.JobID); err != nil {
		t.Errorf("a local caller cannot take the export: %v", err)
	}
}

func mustCaller(ctx context.Context, t *testing.T) Caller {
	t.Helper()

	c, ok := CallerFrom(ctx)
	if !ok {
		t.Fatal("no caller")
	}

	return c
}

// VV-301: in network mode the result of image:export holds the name of the file under the workspace and never the
// absolute path of the server; in local mode the absolute path stays.
func TestImageExportHidesTheServerPathInNetworkMode(t *testing.T) {
	s, _ := newImageService(t)
	alice := networkUser("alice")

	created, err := s.ImageCreate(alice, ImageCreateRequest{Width: 64, Height: 48})
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.ImageExport(alice, ImageExportRequest{ImageID: created.ImageID})
	if err != nil {
		t.Fatal(err)
	}

	want := "exports/images/" + created.ImageID + "/"
	if filepath.IsAbs(res.Path) || !strings.HasPrefix(res.Path, want) || strings.Contains(res.Text, s.workspace) {
		t.Errorf("network path = %q, text = %q: want a path under %q and no absolute path", res.Path, res.Text, want)
	}

	if text := ImageExportText(res); strings.Contains(text, s.workspace) {
		t.Errorf("the text of the CLI and of MCP names the workspace: %q", text)
	}

	job, err := s.JobGet(alice, JobGetRequest{JobID: res.JobID})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(raw), filepath.ToSlash(s.workspace)) ||
		strings.Contains(strings.ReplaceAll(string(raw), `\\`, `\`), s.workspace) {
		t.Errorf("the job names the workspace: %s", raw)
	}

	// The last part of the path is the name the download route takes, and the route takes nothing but such a name.
	if _, err := s.ImageFile(alice, ImageFileRequest{ImageID: created.ImageID, Name: path.Base(res.Path)}); err != nil {
		t.Errorf("the file by the name in the path: %v", err)
	}

	if _, err := s.ImageFile(
		networkUser("bob"), ImageFileRequest{ImageID: created.ImageID, Name: path.Base(res.Path)},
	); codeOf(err) != "E_NOT_FOUND" {
		t.Errorf("another user read the file: %v", err)
	}

	base := path.Base(res.Path)

	for _, name := range []string{res.Path, "../" + base, `..\` + base, "../../stuarty.db"} {
		_, err := s.ImageFile(alice, ImageFileRequest{ImageID: created.ImageID, Name: name})
		if codeOf(err) != "E_NOT_FOUND" {
			t.Errorf("the name %q = %v, want E_NOT_FOUND", name, err)
		}
	}

	local := WithCaller(context.Background(), Caller{
		Actor: "local:me", Scopes: []string{ScopeRead, ScopeEdit, ScopeMedia, ScopeRender},
	})

	mine, err := s.ImageCreate(local, ImageCreateRequest{Width: 64, Height: 48})
	if err != nil {
		t.Fatal(err)
	}

	here, err := s.ImageExport(local, ImageExportRequest{ImageID: mine.ImageID})
	if err != nil {
		t.Fatal(err)
	}

	if !filepath.IsAbs(here.Path) {
		t.Errorf("the local path = %q, want the absolute one", here.Path)
	}
}

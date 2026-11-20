package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

	row, err := s.store.Jobs().Get(context.Background(), res.JobID)
	if err != nil || row.Kind != JobImageExport || row.State != JobSucceeded || row.ProjectID != nil ||
		row.AssetHash != nil {
		t.Fatalf("the job = %+v, %v", row, err)
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

func mustCaller(ctx context.Context, t *testing.T) Caller {
	t.Helper()

	c, ok := CallerFrom(ctx)
	if !ok {
		t.Fatal("no caller")
	}

	return c
}

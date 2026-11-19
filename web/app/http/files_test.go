package httpapi_test

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// exportedJob records a succeeded export job and its file in the workspace.
func (h *harness) exportedJob(project, job, state string, content []byte) {
	h.t.Helper()

	ctx := context.Background()
	row := service.JobRow{
		ID:        job,
		Kind:      "export",
		ProjectID: &project,
		Actor:     "local:api",
		State:     "queued",
		CreatedMs: 1,
		UpdatedMs: 1,
	}

	err := h.store.Write(
		ctx,
		func(ctx context.Context, tx service.Tx) error { return tx.Jobs().CreateLimited(ctx, row, nil) },
	)
	if err != nil {
		h.t.Fatal(err)
	}

	if state != "queued" {
		if ok, err := h.store.Jobs().Claim(ctx, job, 2); err != nil || !ok {
			h.t.Fatalf("claim: %v %v", ok, err)
		}

		if ok, err := h.store.Jobs().
			Finish(ctx, job, state, []byte(`{"export":{"revision":1}}`), nil, 3); err != nil ||
			!ok {
			h.t.Fatalf("finish: %v %v", ok, err)
		}
	}

	dir := filepath.Join(string(h.dir), "exports", project)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		h.t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, job+".mp4"), content, 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func TestExportFileServesBytesWithRange(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	project := h.newProject("export")
	content := []byte("0123456789abcdefghij")

	h.exportedJob(project, "job_aaaaaaaaaa", "succeeded", content)

	full := h.get("/api/v1/jobs/job_aaaaaaaaaa/file")
	if full.status != http.StatusOK || !bytes.Equal(full.body, content) {
		t.Fatalf("file = %d %q", full.status, full.body)
	}

	for header, want := range map[string]string{
		"Content-Type":           "video/mp4",
		"Content-Disposition":    `attachment; filename="export-1970-01-01.mp4"`,
		"Cache-Control":          "private, max-age=31536000, immutable",
		"Accept-Ranges":          "bytes",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := full.header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	if etag := full.header.Get("ETag"); etag == "" {
		t.Error("no ETag")
	}

	part := h.do(req{path: "/api/v1/jobs/job_aaaaaaaaaa/file", headers: map[string]string{"Range": "bytes=10-14"}})
	if part.status != http.StatusPartialContent || string(part.body) != "abcde" ||
		part.header.Get("Content-Range") != "bytes 10-14/20" {
		t.Errorf("range = %d %q %q", part.status, part.body, part.header.Get("Content-Range"))
	}

	bad := h.do(req{path: "/api/v1/jobs/job_aaaaaaaaaa/file", headers: map[string]string{"Range": "bytes=100-"}})
	if bad.status != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("unsatisfiable range = %d, want 416", bad.status)
	}

	cached := h.do(
		req{
			path:    "/api/v1/jobs/job_aaaaaaaaaa/file",
			headers: map[string]string{"If-None-Match": full.header.Get("ETag")},
		},
	)
	if cached.status != http.StatusNotModified {
		t.Errorf("If-None-Match = %d, want 304", cached.status)
	}
}

func TestExportFileRefusals(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	project := h.newProject("export")

	h.exportedJob(project, "job_bbbbbbbbbb", "queued", []byte("x"))

	if res := h.get(
		"/api/v1/jobs/job_bbbbbbbbbb/file",
	); res.status != http.StatusConflict ||
		res.json(t)["code"] != "E_INVALID_STATE" {
		t.Errorf("a job that is not succeeded = %d %s", res.status, res.body)
	}

	if res := h.get(
		"/api/v1/jobs/job_zzzzzzzzzz/file",
	); res.status != http.StatusNotFound ||
		res.json(t)["code"] != "E_NOT_FOUND" {
		t.Errorf("unknown job = %d %s", res.status, res.body)
	}

	for _, id := range []string{"..%2F..%2Fdb.sqlite", "job_x%2F..%2F..", "..", "%2e%2e"} {
		if res := h.get("/api/v1/jobs/" + id + "/file"); res.status == http.StatusOK {
			t.Errorf("job id %q served a file", id)
		}
	}
}

func TestSnapPointsAndFontsRoutes(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	project := h.newProject("snap")

	snap := h.get("/api/v1/projects/" + project + "/snap-points")
	if snap.status != http.StatusOK {
		t.Fatalf("snap-points = %d %s", snap.status, snap.body)
	}

	body := snap.json(t)
	if _, ok := body["revision"]; !ok || body["points"] == nil || body["limits"] == nil {
		t.Errorf("snap-points = %v", body)
	}

	if stale := h.get("/api/v1/projects/" + project + "/snap-points?rev=999"); stale.status != http.StatusConflict {
		t.Errorf("a stale rev = %d %s", stale.status, stale.body)
	}

	fonts := h.get("/api/v1/fonts")
	if fonts.status != http.StatusOK || !bytes.Contains(fonts.body, []byte(`"Inter"`)) {
		t.Errorf("fonts = %d %s", fonts.status, fonts.body)
	}

	if audit := h.get("/api/v1/audit"); audit.status != http.StatusOK {
		t.Errorf("audit = %d %s", audit.status, audit.body)
	}
}

func TestExportFileDownloadNameIsTheProjectName(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	project := h.newProject("My Ad: 50% off!")

	h.exportedJob(project, "job_cccccccccc", "succeeded", []byte("x"))

	res := h.get("/api/v1/jobs/job_cccccccccc/file")
	want := `attachment; filename="My-Ad-50-off-1970-01-01.mp4"`
	if got := res.header.Get("Content-Disposition"); got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
}

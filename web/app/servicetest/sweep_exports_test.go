package servicetest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// The files of exported images are not files of a project that no longer exists: the sweep of the start-up
// leaves them (found by the real kill of an image export, epic 90).
func TestSweepKeepsTheExportedImages(t *testing.T) {
	e := newEnvWith(t, func(e *env, o *service.Options) { o.Workspace = string(e.dir) })
	ctx := e.as(agent)

	created, err := e.svc.ImageCreate(ctx, service.ImageCreateRequest{Preset: "video_thumbnail", Name: "Keep"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ImageEdit(ctx, service.ImageEditRequest{
		ImageID: created.ImageID, BaseRevision: 1, Operations: []command.Operation{
			op("layer.add_fill", `{"color": "#101820", "id": "lyr_aaaaaaaaaa"}`),
		},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := e.svc.ImageExport(ctx, service.ImageExportRequest{ImageID: created.ImageID, Format: "png"})
	if err != nil {
		t.Fatal(err)
	}

	sweeper, ok := service.SweeperOf(e.svc)
	if !ok {
		t.Fatal("the service has no sweeper")
	}

	if _, err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(out.Path); err != nil {
		t.Fatalf("the sweep removed the exported image: %v", err)
	}
}

// The folder of an image that does not exist is dropped like the folder of a project that does not exist, and the
// temporary files a killed export left are removed when they are old and no live job is using them.
func TestSweepRemovesWhatAKilledExportLeft(t *testing.T) {
	e := newEnvWith(t, func(e *env, o *service.Options) { o.Workspace = string(e.dir) })
	ctx := e.as(agent)

	created, err := e.svc.ImageCreate(ctx, service.ImageCreateRequest{Preset: "video_thumbnail", Name: "Left"})
	if err != nil {
		t.Fatal(err)
	}

	ws := string(e.dir)
	project := e.newProject("left")
	old := e.clock.now().Add(-time.Hour)

	files := map[string]bool{ // path under the workspace -> it stays
		filepath.Join("exports", "images", created.ImageID, "7-aaaaaaaaaaaa.png"): true,
		filepath.Join("exports", "images", created.ImageID, ".tmp-123"):           false,
		filepath.Join("exports", "images", created.ImageID, ".tmp-fresh"):         true,
		filepath.Join("exports", "images", "img_gone", "1-bbbbbbbbbbbb.png"):      false,
		filepath.Join("exports", project, "job_0000000901.mp4"):                   true,
		filepath.Join("exports", project, ".job_0000000902.part"):                 false,
		filepath.Join("exports", project, ".job_0000000903.part"):                 true,
		filepath.Join("tmp", "job_0000000904", "job_0000000904.partial.mp4"):      false,
		filepath.Join("tmp", "job_0000000903", "job_0000000903.partial.mp4"):      true,
		filepath.Join("tmp", "capabilities-ab12cd34", "probe.txt"):                true,
		filepath.Join("tmp", "job_0000000905", "job_0000000905.partial.mp4"):      true,
	}

	for rel := range files {
		path := filepath.Join(ws, rel)

		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		// A file or a folder is old unless it is the fresh one: something a process wrote a moment ago.
		age := old
		if strings.Contains(rel, "fresh") {
			age = e.clock.now()
		}

		for _, p := range []string{path, filepath.Dir(path)} {
			if strings.Contains(p, "tmp") || p == path {
				if err := os.Chtimes(p, age, age); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	// job_0000000903 is running (its file and its run directory are in use), job_0000000905 is queued (the next
	// worker clears its directory), job_0000000902 and job_0000000904 do not exist.
	for _, id := range []string{"job_0000000903", "job_0000000905"} {
		if err := e.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
			row := service.JobRow{ID: id, Kind: service.JobExport, ProjectID: &project, Actor: ui,
				State: service.JobQueued, Input: json.RawMessage(`{}`)}
			if err := tx.Jobs().CreateLimited(ctx, row, nil); err != nil {
				return err
			}

			if id == "job_0000000903" {
				_, cerr := tx.Jobs().Claim(ctx, id, e.clock.now().UnixMilli())

				return cerr
			}

			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	sweeper, ok := service.SweeperOf(e.svc)
	if !ok {
		t.Fatal("the service has no sweeper")
	}

	res, err := sweeper.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for rel, stays := range files {
		if _, serr := os.Stat(filepath.Join(ws, rel)); (serr == nil) != stays {
			t.Errorf("%s exists = %v, want %v", rel, serr == nil, stays)
		}
	}

	// Two temporary files and one run directory are the incoming files; the folder of the image that is gone is
	// an export directory.
	if res.IncomingFiles != 3 || res.ExportDirs != 1 {
		t.Errorf("sweep = %+v, want 3 incoming files and 1 export directory", res)
	}
}

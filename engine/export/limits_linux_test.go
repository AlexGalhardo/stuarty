//go:build linux

package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
)

// SDD-007 T-16: the limits (RLIMIT_CPU, RLIMIT_FSIZE, RLIMIT_AS for a software encoder) do not break the golden export.
func TestSecurity_T16_LimitsDoNotBreakGoldenExport(t *testing.T) {
	doc := loadScene(t, "clip-caption")
	job := newJob(t, doc, "export-limits", filepath.Join("limits", "out.mp4"))

	if ok, why := ffmpeg.LimitsAvailable(); !ok {
		// skip-env: rlimits
		t.Skip(why)
	}

	l := ffmpeg.LimitsFor(ffmpeg.KindExportSoftware, ffmpeg.ExportDeadline(10*time.Second), 4, 4<<30)

	in, err := job.Runner.In.WithLimits(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}

	r, err := ffmpeg.NewRunner(in, shared.dir)
	if err != nil {
		t.Fatal(err)
	}

	job.Runner = r

	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatalf("the export under limits failed: %v", err)
	}

	_ = os.Remove(res.Path)
}

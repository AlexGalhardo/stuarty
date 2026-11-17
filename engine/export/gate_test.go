package export

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// A project named "../../x" exports to exports/<project id>/<job id>.mp4 and nowhere else (T-13).
func TestSecurity_T13_ExportPathsAreGenerated(t *testing.T) {
	doc := loadScene(t, "contain-text")
	doc.Name = "../../x"

	exports := filepath.Join(shared.dir, "exports-paths")
	job := newJob(t, doc, "export-paths", "unused.mp4")
	job.OutPath, job.ExportsDir, job.ProjectID = "", exports, "prj_abc123"

	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(exports, "prj_abc123", "export-paths.mp4")
	if res.Path != want {
		t.Fatalf("published at %s, want %s", res.Path, want)
	}

	// Only the generated file exists in the exports folder: nothing carries the name of the project.
	var found []string

	if err := filepath.WalkDir(exports, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found = append(found, p)
		}

		return err
	}); err != nil {
		t.Fatal(err)
	}

	if len(found) != 1 || found[0] != want {
		t.Errorf("files under the exports folder: %v, want only %s", found, want)
	}

	if _, err := os.Stat(filepath.Join(shared.dir, "x")); err == nil {
		t.Error("a folder named after the project exists")
	}

	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(filepath.Dir(want)); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("the project folder is %v, %v, want mode 0700", fi, err)
		}
	}

	_ = os.RemoveAll(exports)
}

// An identifier that could leave the exports folder is refused before anything runs.
func TestExportPathRefusesBadIdentifiers(t *testing.T) {
	setup(t)

	for _, id := range []string{"../x", "a/b", `a\b`, "", ".hidden", "prj_" + string(make([]byte, 80))} {
		if _, err := exportPath("exports", id, "job_1"); err == nil {
			t.Errorf("project id %q accepted", id)
		}

		if _, err := exportPath("exports", "prj_a", id); err == nil {
			t.Errorf("job id %q accepted", id)
		}
	}

	want := filepath.Join("exports", "prj_a1", "job_1.mp4")
	if p, err := exportPath("exports", "prj_a1", "job_1"); err != nil || p != want {
		t.Errorf("%q, %v", p, err)
	}

	doc := loadScene(t, "contain-text")
	job := newJob(t, doc, "export-badid", "unused.mp4")
	job.OutPath, job.ExportsDir, job.ProjectID = "", filepath.Join(shared.dir, "exports-bad"), "../escape"

	if _, err := Run(context.Background(), job); err == nil {
		t.Fatal("an export with a path in the project id ran")
	}

	if _, err := os.Stat(filepath.Join(shared.dir, "escape")); err == nil {
		t.Error("the export wrote outside its folder")
	}
}

// A file over the limit ends failed with reason size_limit and nothing is published.
func TestExportSizeLimit(t *testing.T) {
	doc := loadScene(t, "contain-text")
	exports := filepath.Join(shared.dir, "exports-size")
	job := newJob(t, doc, "export-size", "unused.mp4")
	job.OutPath, job.ExportsDir, job.ProjectID, job.MaxBytes = "", exports, "prj_size", 1000

	_, err := Run(context.Background(), job)
	ee := asFFError(t, err)

	if ee.Code != CodeExportFailed || ee.Details["reason"] != ReasonSizeLimit {
		t.Fatalf("error = %v, want E_EXPORT_FAILED with reason size_limit", err)
	}

	if m, _ := filepath.Glob(filepath.Join(exports, "*", "*")); len(m) > 0 {
		t.Errorf("published despite the limit: %v", m)
	}

	if m, _ := filepath.Glob(filepath.Join(shared.dir, "tmp", job.JobID+"*")); len(m) > 0 {
		t.Errorf("the work folders %v were not removed", m)
	}
}

// When the encoder dies early (here the size watchdog), the frame loop stops
// instead of composing the rest of the project.
func TestExportStopsComposingWhenTheEncoderDies(t *testing.T) {
	doc := loadScene(t, "contain-text")

	// The size watchdog polls every 2 s. 300 frames of this scene can be composed
	// before that tick, so the clip is long enough that the encoder is still
	// reading when the file passes the cap.
	for id, it := range doc.Items {
		if it.Type == timeline.ItemClip {
			it.Duration = 900
			doc.Items[id] = it
		}
	}

	exports := filepath.Join(shared.dir, "exports-stop")
	job := newJob(t, doc, "export-stop", "unused.mp4")
	job.OutPath, job.ExportsDir, job.ProjectID, job.MaxBytes = "", exports, "prj_stop", 1000

	var last Progress
	job.OnProgress = func(p Progress) { last = p }

	_, err := Run(context.Background(), job)
	if ee := asFFError(t, err); ee.Details["reason"] != ReasonSizeLimit {
		t.Fatalf("error = %v, want reason size_limit", err)
	}

	if last.Total > 0 && last.Frame >= last.Total {
		t.Errorf("composed all %d frames after the encoder died", last.Total)
	}
}

// Progress is published at most 4 times a second and once at the last frame.
func TestExportProgressIsThrottled(t *testing.T) {
	doc := loadScene(t, "two-clips-cut")
	job := newJob(t, doc, "export-progress", filepath.Join("progress", "out.mp4"))

	var (
		mu    sync.Mutex
		stamp []time.Time
		last  Progress
	)

	job.OnProgress = func(p Progress) {
		mu.Lock()
		defer mu.Unlock()

		stamp = append(stamp, time.Now())
		last = p
	}

	start := time.Now()

	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}

	_ = os.Remove(res.Path)

	mu.Lock()
	defer mu.Unlock()

	if last.Frame != last.Total || last.Total != res.Frames {
		t.Errorf("last progress %+v, want %d of %d", last, res.Frames, res.Frames)
	}

	secs := time.Since(start).Seconds()
	if most := int(secs*4) + 2; len(stamp) > most {
		t.Errorf("%d reports in %.1f s, want at most %d", len(stamp), secs, most)
	}

	for i := 2; i < len(stamp)-1; i++ { // the last report is forced and may follow another closely
		if stamp[i].Sub(stamp[i-2]) < 400*time.Millisecond {
			t.Errorf("reports %d and %d came %s apart", i-2, i, stamp[i].Sub(stamp[i-2]))
		}
	}

	t.Logf("%d reports in %.1f s", len(stamp), secs)
}

// ffmpegRewrite replaces the partial file with the result of an FFmpeg command, to build a file that
// breaks one rule of the verification. The arguments are test input, run straight through the tool.
func ffmpegRewrite(t *testing.T, partial string, args ...string) {
	t.Helper()

	out := partial + ".rewritten.mp4"
	full := append([]string{"-hide_banner", "-v", "error", "-nostdin", "-y", "-i", partial}, args...)
	full = append(full, "-f", "mp4", out)

	if stderr, err := shared.gen.Run(context.Background(), filepath.Dir(partial), full...); err != nil {
		t.Fatalf("rewriting the partial file: %v\n%s", err, stderr)
	}

	if err := os.Rename(out, partial); err != nil {
		t.Fatal(err)
	}
}

// A file with no audio stream, one two frames short and one with mdat before moov each end failed
// with E_VERIFY_FAILED and exactly the failed checks, never succeeded.
func TestVerifyEachCheck(t *testing.T) {
	cases := []struct {
		name string
		args func(frames int) []string
		want []string
	}{
		{"no audio", func(int) []string {
			return []string{"-an", "-c:v", "copy", "-movflags", "+faststart"}
		}, []string{"audio_stream"}},
		{"two frames short", func(frames int) []string {
			secs := strconv.FormatFloat(float64(frames-2)/30, 'f', 4, 64)

			return []string{"-c", "copy", "-t", secs, "-movflags", "+faststart"}
		}, []string{"video_duration", "audio_duration", "video_packets"}},
		{"mdat before moov", func(int) []string { return []string{"-c", "copy"} }, []string{"faststart"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := loadScene(t, "contain-text")
			frames := endFrame(doc)
			job := newJob(t, doc, "verify-"+strconv.Itoa(len(c.name)), filepath.Join("verify", "out.mp4"))
			job.afterEncode = func(partial string) error {
				ffmpegRewrite(t, partial, c.args(frames)...)

				return nil
			}

			_, err := Run(context.Background(), job)
			ee := asFFError(t, err)

			if ee.Code != CodeVerifyFailed {
				t.Fatalf("code %s, want %s: %v", ee.Code, CodeVerifyFailed, err)
			}

			checks, _ := ee.Details["checks"].([]Check)

			var got []string
			for _, ck := range checks {
				got = append(got, ck.Name)
			}

			t.Logf("failed checks: %v", got)

			if len(got) != len(c.want) {
				t.Fatalf("failed %v, want exactly %v", got, c.want)
			}

			for _, w := range c.want {
				found := false

				for _, g := range got {
					found = found || g == w
				}

				if !found {
					t.Errorf("check %s did not fail: %v", w, got)
				}
			}

			noLeftovers(t, job.JobID, job.OutPath)
		})
	}
}

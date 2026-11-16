package export

import (
	"context"
	"testing"
)

func TestExportRangeValidation(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		start, end, total  int
		wantStart, wantEnd int
		ok                 bool
	}{
		"whole":          {0, 0, 90, 0, 90, true},
		"explicit whole": {0, 90, 90, 0, 90, true},
		"inner":          {30, 60, 90, 30, 60, true},
		"open end":       {30, 0, 90, 30, 90, true},
		"empty":          {30, 30, 90, 0, 0, false},
		"past end":       {0, 91, 90, 0, 0, false},
		"negative start": {-1, 10, 90, 0, 0, false},
		"start at end":   {90, 0, 90, 0, 0, false},
	} {
		s, e, err := exportRange(&Job{Start: tc.start, End: tc.end}, tc.total)
		if (err == nil) != tc.ok || s != tc.wantStart || e != tc.wantEnd {
			t.Errorf("%s: got [%d,%d) %v", name, s, e, err)
		}
	}
}

// A range exports exactly its frames and the frames are the ones of the same timeline positions.
func TestExportRangeMatchesTheWholeExport(t *testing.T) {
	setup(t)

	doc := loadScene(t, "clip-caption")
	total := endFrame(doc)

	if total < 12 {
		t.Skipf("scene has %d frames", total)
	}

	whole, err := Run(context.Background(), newJob(t, doc, "range-whole", "range-whole.mp4"))
	if err != nil {
		t.Fatal(err)
	}

	start, end := total/4, total/4+total/2
	job := newJob(t, doc, "range-part", "range-part.mp4")
	job.Start, job.End = start, end

	part, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}

	if part.Frames != end-start {
		t.Fatalf("frames = %d, want %d", part.Frames, end-start)
	}

	w, h := doc.Settings.Width, doc.Settings.Height

	for _, i := range []int{0, (end - start) / 2, end - start - 1} {
		a := decodeFrame(t, part.Path, doc, i)
		b := decodeFrame(t, whole.Path, doc, start+i)

		if s := ssim(luma(a), luma(b), w, h); s < 0.95 {
			t.Errorf("frame %d of the range vs frame %d of the whole: ssim %.3f", i, start+i, s)
		}
	}

	// The scene must move, or the comparison above would hold for a range that started at zero too.
	first := luma(decodeFrame(t, whole.Path, doc, 0))
	inside := luma(decodeFrame(t, whole.Path, doc, start))
	if s := ssim(first, inside, w, h); s >= 0.95 {
		t.Errorf("the scene is too still (ssim %.3f) to tell the range from the start", s)
	}

	job = newJob(t, doc, "range-bad", "range-bad.mp4")
	job.Start, job.End = 0, total+1

	if _, err := Run(context.Background(), job); err == nil {
		t.Error("a range past the end was accepted")
	}
}

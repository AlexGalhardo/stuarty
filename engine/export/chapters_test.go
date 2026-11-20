package export

import (
	"errors"

	"context"
	"encoding/json"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// runExport runs the job and fails the test with the details of a failed verification.
func runExport(t *testing.T, job Job) *Result {
	t.Helper()

	res, err := Run(context.Background(), job)
	if err != nil {
		var fe *ffmpeg.Error
		if errors.As(err, &fe) {
			t.Fatalf("%v %+v", err, fe.Details)
		}

		t.Fatal(err)
	}

	return res
}

type fileChapter struct {
	Start string `json:"start_time"`
	End   string `json:"end_time"`
	Tags  struct {
		Title string `json:"title"`
	} `json:"tags"`
}

// fileChapters is `ffprobe -show_chapters` of a file the export wrote (VV-290).
func fileChapters(t *testing.T, path string) []fileChapter {
	t.Helper()

	dir, err := shared.runner.NewRunDir("chapters-probe-" + strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dir.Remove() })

	argv, err := ffargs.ProbeChapters(shared.runner.Policy(dir, filepath.Dir(path)), path)
	if err != nil {
		t.Fatal(err)
	}

	raw, _, err := shared.runner.RunFFprobe(context.Background(), dir, argv, verifyTimeout, filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}

	var pc struct {
		Chapters []fileChapter `json:"chapters"`
	}
	if err := json.Unmarshal(raw, &pc); err != nil {
		t.Fatal(err)
	}

	return pc.Chapters
}

// The labelled markers of the timeline come out as the chapters of the MP4, in order, with their titles;
// a marker with no label is not a chapter (VV-290).
func TestExportWritesLabelledMarkersAsChapters(t *testing.T) {
	doc := loadScene(t, "two-clips-cut")
	end := endFrame(doc)
	doc.Markers = []timeline.Marker{
		{Frame: 0, Label: "Intro"},
		{Frame: end / 4},
		{Frame: end / 2, Label: `Tricks; a=b #1 \ end`},
	}

	job := newJob(t, doc, "chapters-mp4", "chapters-mp4.mp4")

	res := runExport(t, job)

	got := fileChapters(t, res.Path)
	if len(got) != 2 {
		t.Fatalf("chapters %+v, want 2", got)
	}

	if got[0].Tags.Title != "Intro" || got[1].Tags.Title != `Tricks; a=b #1 \ end` {
		t.Errorf("titles %q, %q", got[0].Tags.Title, got[1].Tags.Title)
	}

	if got[0].Start != "0.000000" {
		t.Errorf("the first chapter starts at %s", got[0].Start)
	}

	if got[0].End != got[1].Start {
		t.Errorf("chapters are not contiguous: %s then %s", got[0].End, got[1].Start)
	}
}

// A project with no labelled marker writes no chapter, and the file is the one it was.
func TestExportWithNoMarkersHasNoChapters(t *testing.T) {
	doc := loadScene(t, "clip-caption")
	doc.Markers = []timeline.Marker{{Frame: 3}}

	job := newJob(t, doc, "chapters-none", "chapters-none.mp4")

	res := runExport(t, job)

	if got := fileChapters(t, res.Path); len(got) != 0 {
		t.Fatalf("chapters %+v, want none", got)
	}
}

// A range exports only the chapters inside it, counted from its first frame.
func TestExportRangeChapters(t *testing.T) {
	doc := loadScene(t, "two-clips-cut")
	end := endFrame(doc)
	doc.Markers = []timeline.Marker{{Frame: 1, Label: "before"}, {Frame: end / 2, Label: "inside"}}

	job := newJob(t, doc, "chapters-range", "chapters-range.mp4")
	job.Start = end / 2

	res := runExport(t, job)

	got := fileChapters(t, res.Path)
	if len(got) != 1 || got[0].Tags.Title != "inside" || got[0].Start != "0.000000" {
		t.Fatalf("chapters %+v", got)
	}
}

// The audio export in an MP4 container carries them too; a WAV and a GIF have no chapters and the export
// still succeeds (SDD-004 section 8.1).
func TestAudioExportChapters(t *testing.T) {
	doc := loadDir(t, formatDir("audio"))
	doc.Markers = []timeline.Marker{{Frame: 0, Label: "Verse"}, {Frame: 15, Label: "Chorus"}}

	job := newJob(t, doc, "chapters-aac", "chapters-aac.m4a")
	job.Format = FormatAAC

	res := runExport(t, job)

	got := fileChapters(t, res.Path)
	if len(got) != 2 || got[0].Tags.Title != "Verse" || got[1].Tags.Title != "Chorus" {
		t.Fatalf("chapters %+v", got)
	}

	wav := newJob(t, doc, "chapters-wav", "chapters-wav.wav")
	wav.Format = FormatWAV

	runExport(t, wav)
}

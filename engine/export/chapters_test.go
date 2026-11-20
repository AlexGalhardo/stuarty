package export

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// runExport runs the job and fails the test with the details of a failed verification.
func runExport(t *testing.T, job Job) *Result {
	t.Helper()

	res, err := Run(context.Background(), job)
	if err != nil {
		if fe, ok := errors.AsType[*ffmpeg.Error](err); ok {
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

// A first marker after the start of the file, the way VV-290 was reported (`marker.add at=10s`), and a
// label that ends with a backslash, in the MP4 and in the M4A: the export succeeds, an untitled chapter
// covers the time before the first marker, and each labelled chapter starts at its marker.
func TestExportChaptersAfterTheFirstFrame(t *testing.T) {
	for _, f := range []Format{FormatMP4, FormatAAC} {
		doc := loadDir(t, formatDir("audio"))
		doc.Markers = []timeline.Marker{
			{Frame: 13, Label: `C:\`},
			{Frame: 33, Label: "Verse"},
			{Frame: 60, Label: "Outro"},
		}

		name := "chapters-late-" + string(f)
		job := newJob(t, doc, name, name+map[Format]string{FormatMP4: ".mp4", FormatAAC: ".m4a"}[f])
		job.Format = f
		job.Start, job.End = 5, 75

		res := runExport(t, job)

		want := []struct {
			start float64
			title string
		}{{0, ""}, {267, "C:"}, {933, "Verse"}, {1833, "Outro"}}

		got := fileChapters(t, res.Path)
		if len(got) != len(want) {
			t.Fatalf("%s: chapters %+v, want %d", f, got, len(want))
		}

		for i, w := range want {
			start, _ := strconv.ParseFloat(got[i].Start, 64)
			if got[i].Tags.Title != w.title || math.Abs(start*1000-w.start) > 1.5 {
				t.Errorf("%s: chapter %d is %+v, want %q at %v ms", f, i+1, got[i], w.title, w.start)
			}
		}
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

// The audio export in an MP4 container carries them too; a WAV has no chapters and the export still
// succeeds (SDD-004 section 8.1).
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

// A marker label is the only text of a user that reaches FFmpeg, inside the ffmetadata file of the
// chapters. A label that tries to end its line (LF, CR, NUL, a Unicode separator, a final backslash)
// to add a chapter, a key or a global tag stays one title of its own chapter, and nothing of it reaches
// the global or the stream metadata of the file (SDD-007 T-102, FF-08).
func TestSecurity_T102_MarkerLabelCannotInjectIntoTheChapterFile(t *testing.T) {
	lineSep, paraSep := string(rune(0x2028)), string(rune(0x2029))

	doc := loadScene(t, "two-clips-cut")
	end := endFrame(doc)
	doc.Markers = []timeline.Marker{
		{Frame: 0, Label: "Intro\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1\ntitle=evil"},
		{Frame: end / 4, Label: "B\r[CHAPTER]\rSTART=1\x00[CHAPTER]\x00START=2\x00END=3\x00title=evil"},
		{Frame: end / 2, Label: "C" + lineSep + "encoder=evil" + paraSep + ";FFMETADATA1 #x \\"},
		{Frame: 3 * end / 4, Label: "D"},
	}

	job := newJob(t, doc, "chapters-t89", "chapters-t89.mp4")

	res := runExport(t, job)

	want := []string{
		"Intro [CHAPTER] TIMEBASE=1/1000 START=0 END=1 title=evil",
		"B [CHAPTER] START=1 [CHAPTER] START=2 END=3 title=evil",
		"C encoder=evil ;FFMETADATA1 #x",
		"D",
	}

	got := fileChapters(t, res.Path)
	if len(got) != len(want) {
		t.Fatalf("chapters %+v, want %d", got, len(want))
	}

	for i, w := range want {
		if got[i].Tags.Title != w {
			t.Errorf("chapter %d: title %q, want %q", i+1, got[i].Tags.Title, w)
		}
	}

	dir, err := shared.runner.NewRunDir("chapters-t89-probe")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dir.Remove() })

	argv, err := ffargs.ProbeJSON(shared.runner.Policy(dir, filepath.Dir(res.Path)), res.Path)
	if err != nil {
		t.Fatal(err)
	}

	raw, _, err := shared.runner.RunFFprobe(context.Background(), dir, argv, verifyTimeout, filepath.Dir(res.Path))
	if err != nil {
		t.Fatal(err)
	}

	var pf struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
		Streams []struct {
			Tags map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &pf); err != nil {
		t.Fatal(err)
	}

	tags := []map[string]string{pf.Format.Tags}
	for _, st := range pf.Streams {
		tags = append(tags, st.Tags)
	}

	// The stream encoder tag ("Lavc libopenh264") is FFmpeg's own, with or without chapters.
	for _, m := range tags {
		for k, v := range m {
			if k == "title" || strings.Contains(v, "evil") || strings.Contains(v, "Intro") {
				t.Errorf("a label reached the metadata of the file: %s=%q", k, v)
			}
		}
	}
}

// The chapter track is the muxer's choice: an MP4 with chapters may have it or not (verifyChapters proves
// the chapters), never two data streams; an MP4 with no chapters has no data stream at all.
func TestStreamsCheckAllowsTheChapterTrack(t *testing.T) {
	media := []probedStream{{CodecType: "video"}, {CodecType: "audio"}}
	data := probedStream{CodecType: "data"}

	labelled := &state{job: &Job{Doc: &timeline.Document{
		Settings: timeline.Settings{FPS: timeline.Rational{Num: 30, Den: 1}},
		Markers:  []timeline.Marker{{Frame: 0, Label: "Intro"}},
	}}, frames: 30}
	plain := &state{job: &Job{Doc: &timeline.Document{
		Settings: timeline.Settings{FPS: timeline.Rational{Num: 30, Den: 1}},
	}}, frames: 30}

	cases := []struct {
		name    string
		s       *state
		streams []probedStream
		bad     bool
	}{
		{"chapters with the track", labelled, append(slices.Clone(media), data), false},
		{"chapters without the track", labelled, media, false},
		{"chapters with two data streams", labelled, append(slices.Clone(media), data, data), true},
		{"no chapters", plain, media, false},
		{"no chapters and a data stream", plain, append(slices.Clone(media), data), true},
		{"a subtitle stream", labelled, append(slices.Clone(media), probedStream{CodecType: "subtitle"}), true},
	}

	for _, c := range cases {
		if _, bad := c.s.streamsCheck(c.streams, 2); bad != c.bad {
			t.Errorf("%s: bad = %v, want %v", c.name, bad, c.bad)
		}
	}
}

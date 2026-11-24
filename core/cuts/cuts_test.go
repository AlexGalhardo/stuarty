package cuts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/facetrack"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// talk is a talk of n sentences of 8 words, 3 s each, with a 1 s pause after each: sentence k starts at 4k s.
func talk(n int) []Word {
	var words []Word

	for k := range n {
		for w := range 8 {
			text := "word"
			if w == 7 {
				text = "end."
			}

			start := k*4000 + w*375
			words = append(words, Word{Text: text, StartMS: start, EndMS: start + 350})
		}
	}

	return words
}

func faces(seconds int, absent func(sec int) bool) *facetrack.Track {
	t := &facetrack.Track{
		Version: facetrack.Version,
		Source:  facetrack.Size{Width: 1920, Height: 1080},
		FPS:     1,
		EndMS:   seconds * 1000,
	}

	for s := range seconds {
		sample := facetrack.Sample{T: s * 1000}
		if !absent(s) {
			sample.Faces = []facetrack.Face{{X: 4000, Y: 2000, W: 1500, H: 3000, Score: 900}}
		}

		t.Samples = append(t.Samples, sample)
	}

	return t
}

func TestProposalsAreWholeSentencesWithAFace(t *testing.T) {
	o := Options{MinMS: 10_000, MaxMS: 20_000, Count: 3}.WithDefaults()
	words := talk(20) // 80 s

	got := Propose(words, faces(80, func(int) bool { return false }), o)
	if len(got) != 3 {
		t.Fatalf("%d proposals, want 3", len(got))
	}

	for i, p := range got {
		if p.N != i+1 || p.EndMS-p.StartMS < 10_000 || p.EndMS-p.StartMS > 20_000 {
			t.Errorf("proposal %d = %+v", i, p)
		}

		if p.StartMS%4000 != 0 || (p.EndMS+1025)%4000 != 0 {
			t.Errorf("proposal %d does not start and end at a sentence: %d to %d", i, p.StartMS, p.EndMS)
		}

		if p.FaceShare != 1000 || p.Title != "word word word word word word word end." {
			t.Errorf("proposal %d = %+v", i, p)
		}

		for j := range i {
			if p.StartMS < got[j].EndMS && got[j].StartMS < p.EndMS {
				t.Errorf("proposals %d and %d overlap", j, i)
			}
		}
	}
}

func TestARangeWithoutAFaceIsNotProposed(t *testing.T) {
	o := Options{MinMS: 10_000, MaxMS: 20_000, Count: 20}.WithDefaults()
	// The speaker is off screen from 20 s to 40 s.
	got := Propose(talk(20), faces(80, func(s int) bool { return s >= 20 && s < 40 }), o)

	if len(got) == 0 {
		t.Fatal("nothing proposed")
	}

	for _, p := range got {
		if p.FaceShare < o.MinFaceShare {
			t.Errorf("a proposal with face share %d: %+v", p.FaceShare, p)
		}

		// 24 s to 36 s lies wholly inside the stretch with no face, so nothing may start there.
		if p.StartMS >= 24_000 && p.StartMS < 36_000 {
			t.Errorf("a proposal inside the stretch with no face: %+v", p)
		}
	}

	if none := Propose(talk(20), nil, o); len(none) != 0 {
		t.Errorf("%d proposals with no face track", len(none))
	}
}

func TestADenserStretchScoresHigher(t *testing.T) {
	o := Options{MinMS: 8_000, MaxMS: 9_000, Count: 5, MinFaceShare: 1}.WithDefaults()
	slow := talk(4)
	fast := make([]Word, 0, 80)

	// 16 s of 12 words per second after the slow part: two sentences of 48 words, 4 s each.
	for k := range 2 {
		for w := range 48 {
			start := 20_000 + k*5_000 + w*83
			text := "w"
			if w == 47 {
				text = "end!"
			}

			fast = append(fast, Word{Text: text, StartMS: start, EndMS: start + 80})
		}
	}

	got := Propose(append(slow, fast...), faces(40, func(int) bool { return false }), o)
	if len(got) == 0 || got[0].StartMS < 20_000 {
		t.Errorf("the dense stretch was not ranked first: %+v", got)
	}
}

func TestProposalsAreDeterministic(t *testing.T) {
	o := DefaultOptions()
	words, track := talk(60), faces(240, func(s int) bool { return s%50 == 49 })

	a, b := Propose(words, track, o), Propose(words, track, o)
	if !reflect.DeepEqual(a, b) {
		t.Error("two runs differ")
	}
}

func TestOptions(t *testing.T) {
	if err := (Options{}).WithDefaults().Validate(); err != nil {
		t.Fatal(err)
	}

	for name, o := range map[string]Options{
		"count":    {Count: 21, MinMS: 5000, MaxMS: 6000, PauseMS: 700, MinFaceShare: 1},
		"min>max":  {Count: 1, MinMS: 9000, MaxMS: 6000, PauseMS: 700, MinFaceShare: 1},
		"too long": {Count: 1, MinMS: 5000, MaxMS: 200_000, PauseMS: 700, MinFaceShare: 1},
		"pause":    {Count: 1, MinMS: 5000, MaxMS: 6000, PauseMS: 10, MinFaceShare: 1},
		"share":    {Count: 1, MinMS: 5000, MaxMS: 6000, PauseMS: 700, MinFaceShare: 2000},
	} {
		if o.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The batch a person accepts applies cleanly on the new vertical timeline a project item makes once the asset is
// placed in it, its framing command is the last of it, and it holds no command only the service may issue.
func TestTheBatchBuildsTheCutOnAnEmptyTimeline(t *testing.T) {
	asset := timeline.Asset{
		ID: "ast_3c5v8n2w6d", Hash: "sha256:" + strings.Repeat("a", 64), Kind: timeline.AssetVideo, Name: "talk.mp4",
		Width: 1920, Height: 1080, FrameRate: timeline.Rational{Num: 30, Den: 1}, FrameCount: 9000, HasAudio: true,
		SampleCount: 14_400_000,
	}

	p := Proposal{N: 2, StartMS: 12_345, EndMS: 47_345}

	batch := Batch(&p, asset.ID)

	if len(batch) != 3 || batch[2].Op != "clip.face_frame" ||
		!strings.Contains(string(batch[1].Args), `"source_in":"12.345s"`) ||
		!strings.Contains(string(batch[1].Args), `"duration":"35.000s"`) {
		t.Fatalf("batch = %+v", batch)
	}

	for _, op := range batch {
		if cmd, ok := command.Default().Get(op.Op); !ok || cmd.Internal {
			t.Errorf("%s is not a command a person may send", op.Op)
		}
	}

	doc := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion,
		ID:            "prj_0000000001",
		Name:          "cuts",
		Settings:      timeline.DefaultSettings(),
		Assets:        map[string]timeline.Asset{asset.ID: asset},
		Tracks:        map[string]timeline.Track{},
		Items:         map[string]timeline.Item{},
	}
	doc.Settings.Width, doc.Settings.Height, doc.Settings.FPS = 1080, 1920, timeline.Rational{Num: 30, Den: 1}

	// The face framing needs its resolved track, which the service fills; apply the first two here.
	res, aerr := command.Default().ApplyBatch(doc, batch[:2], command.Env{})
	if aerr != nil {
		t.Fatal(aerr)
	}

	clip := res.Doc.Items[ClipID]
	if clip.Asset != asset.ID || clip.SourceIn != 370 || clip.Duration != 1050 {
		t.Errorf("clip = %+v", clip)
	}
}

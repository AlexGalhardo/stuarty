package export

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/audio"
)

// Epic 53: a composition at a constant speed in the export (SDD-004 sections 3.3, 3.16 and 7). The picture at
// frame r is the frame floor(r x speed) of the nested timeline, drawn by the Go compositor and by the Rust
// renderer to the same bytes, and the sound keeps its pitch because every nested source goes through atempo.
// The golden frames are in testdata/golden-nested/speed-*; regenerate them with
// go test ./engine/export -run TestCompositionSpeedGolden -update (a reviewed change).

type compSpeedCase struct {
	name     string
	num, den int
	duration int
	frames   []int // frames of the parent to draw
}

var compSpeedCases = []compSpeedCase{
	{"half", 1, 2, 180, []int{0, 1, 30, 31, 100, 150, 179}},
	{"double", 2, 1, 45, []int{0, 1, 10, 22, 30, 44}},
}

func (c compSpeedCase) mod(more ...func(*timeline.Item)) func(*timeline.Item) {
	return func(i *timeline.Item) {
		i.Speed, i.Duration = timeline.Rational{Num: c.num, Den: c.den}, c.duration
		for _, m := range more {
			m(i)
		}
	}
}

// childFrames are the frames of the nested timeline that the frames of the parent show: floor(r x n / d).
func (c compSpeedCase) childFrames() []int {
	out := make([]int, len(c.frames))
	for i, r := range c.frames {
		out[i] = r * c.num / c.den
	}

	return out
}

// A flat composition at a speed is its timeline played at that speed: frame r is, byte for byte, the child's
// own frame floor(r x speed), with both renderers.
func TestCompositionSpeedFlatIsTheChildAtTheMappedFrame(t *testing.T) {
	setup(t)

	for _, c := range compSpeedCases {
		parent, nested := nestedProject(t, [2]int{360, 640}, c.mod())

		for _, kind := range []PixelRenderer{PixelRendererGo, PixelRendererRust} {
			got := renderNested(t, parent, nested, "speed-flat-"+c.name, kind, 0, c.frames)
			want := renderNested(t, nested[nstChild], nil, "speed-flat-child-"+c.name, kind, 0, c.childFrames())

			for i, r := range c.frames {
				if !bytes.Equal(got[i], want[i]) {
					t.Errorf("%s %s: frame %d differs from the child's frame %d", c.name, kind, r, c.childFrames()[i])
				}
			}
		}
	}
}

// The group form at a speed draws the group of the child's frame at the mapped frame: Go and Rust give the same
// bytes, whole and in bands, and the frames are the references of testdata/golden-nested.
func TestCompositionSpeedGolden(t *testing.T) {
	setup(t)

	for _, c := range compSpeedCases {
		t.Run(c.name, func(t *testing.T) {
			parent, nested := nestedProject(t, [2]int{360, 640}, c.mod(func(i *timeline.Item) { i.Opacity = 0.6 }))
			dir := filepath.Join("testdata", "golden-nested", "speed-"+c.name)

			byGo := renderNested(t, parent, nested, "speed-"+c.name, PixelRendererGo, 0, c.frames)
			byRust := renderNested(t, parent, nested, "speed-"+c.name, PixelRendererRust, 8, c.frames)

			// The group of the parent at frame r is the group of the child played at 1x at the frame floor(r x
			// speed): a composition at speed 1 that starts there draws the same bytes.
			for i, r := range c.frames {
				at := c.childFrames()[i]
				plain, _ := nestedProject(t, [2]int{360, 640}, func(it *timeline.Item) {
					it.Opacity, it.SourceIn, it.Duration = 0.6, at, 90-at
				})

				ref := renderNested(t, plain, nested, "speed-ref-"+c.name, PixelRendererGo, 0, []int{0})[0]
				if !bytes.Equal(byGo[i], ref) {
					t.Errorf("frame %d: the group differs from the group at the child's frame %d at speed 1", r, at)
				}

				if !bytes.Equal(byGo[i], byRust[i]) {
					t.Errorf("frame %d: the Rust renderer draws other bytes than the Go compositor", r)
				}

				if *update {
					if err := writePNG(framePNG(dir, r), 360, 640, byGo[i]); err != nil {
						t.Fatal(err)
					}
				}

				want := readPNG(t, framePNG(dir, r))
				if sum(byGo[i]) != sum(want) {
					t.Errorf("frame %d: %s, reference %s", r, sum(byGo[i]), sum(want))
					saveMismatch(t, "speed-"+c.name, r, 360, 640, byGo[i])
				}
			}
		})
	}
}

// speedTone is a project with a composition of a nested timeline that plays one second of a 440 Hz tone (the
// tone of NoisyTone from its first second on), at the speed n/d.
func speedTone(t *testing.T, num, den int) (*timeline.Document, map[string]*timeline.Document) {
	t.Helper()

	const (
		audTrack = "trk_s1s1s1s1s1"
		visTrack = "trk_s2s2s2s2s2"
		toneID   = "ast_s000000001"
	)

	newDoc := func(id string) *timeline.Document {
		d, err := timeline.Decode(
			[]byte(`{"schema_version":1,"id":"` + id + `","name":"T","settings":{"width":360,"height":640},` +
				`"assets":{},"tracks":{"` + audTrack + `":{"id":"` + audTrack + `","kind":"audio","order":"a0"},` +
				`"` + visTrack + `":{"id":"` + visTrack + `","kind":"visual","order":"a1"}},"items":{}}`),
		)
		if err != nil {
			t.Fatal(err)
		}

		return d
	}

	child, parent := newDoc(nstChild), newDoc(nstParent)

	in := ingestClip(t, fixtureSpec{Name: "noisy-tone-speed.wav", NoisyTone: true})
	child.Assets[toneID] = timeline.Asset{ID: toneID, Kind: timeline.AssetAudio, Name: "tone.wav"}
	useAsset(child, toneID, in)

	// One second of the asset from its first second: the tone at its full level.
	a := timeline.NewItem("itm_s3s3s3s3s3", timeline.ItemAudio, audTrack, 0, 30)
	a.Asset, a.SourceIn = toneID, 30
	child.Items[a.ID] = a

	comp := timeline.NewItem(nstComp, timeline.ItemComposition, visTrack, 0, 30*den/num)
	comp.Timeline, comp.Speed = nstChild, timeline.Rational{Num: num, Den: den}
	parent.Items[comp.ID] = comp

	return parent, map[string]*timeline.Document{nstChild: child}
}

// crossings is the number of times the left channel of 16-bit stereo PCM changes sign from the sample from to
// the sample to (exclusive).
func crossings(pcm []byte, from, to int) int {
	n, prev := 0, 0

	for i := from; i < to; i++ {
		v := int(int16(uint16(pcm[i*bytesPerFrame]) | uint16(pcm[i*bytesPerFrame+1])<<8))
		if i > from && (v < 0) != (prev < 0) && v != 0 && prev != 0 {
			n++
		}

		if v != 0 {
			prev = v
		}
	}

	return n
}

// The sound of a composition at a speed keeps its pitch, as the sound of a clip does: the nested tone decoded as
// the plan asks (the speed of the item given to atempo) is 440 Hz at 0.5, 1 and 2, and lasts as long as the
// item does (SDD-004 section 7, "A composition at a speed").
func TestCompositionSpeedKeepsThePitch(t *testing.T) {
	setup(t)

	for _, c := range []struct{ num, den int }{{1, 2}, {1, 1}, {2, 1}} {
		parent, nested := speedTone(t, c.num, c.den)
		job := newJob(t, parent, "speed-pitch", "unused.mp4")
		job.Nested = nested

		srcs := plan.AudioWith(parent, 0, parent.Items[nstComp].Duration, plan.Options{
			Nested: func(id string) (*timeline.Document, bool) { d, ok := nested[id]; return d, ok },
		})
		if len(srcs) != 1 {
			t.Fatalf("%d/%d: %d sources, want the tone", c.num, c.den, len(srcs))
		}

		src := srcs[0]
		if src.Speed != (timeline.Rational{Num: c.num, Den: c.den}) ||
			src.Samples != audio.SampleRate*int64(c.den)/int64(c.num) {
			t.Fatalf("%d/%d: the source plays at %v for %d samples", c.num, c.den, src.Speed, src.Samples)
		}

		s := testState(t, job, "speed-pitch-run-"+string(rune('a'+c.num+3*c.den)))
		rc, err := (&audioOpener{s: s}).Open(context.Background(), src)
		if err != nil {
			t.Fatal(err)
		}

		pcm, err := io.ReadAll(io.LimitReader(rc, src.Samples*bytesPerFrame))
		_ = rc.Close()

		if err != nil || int64(len(pcm)) < src.Samples*bytesPerFrame*9/10 {
			t.Fatalf(
				"%d/%d: %d bytes decoded (%v), want about %d",
				c.num,
				c.den,
				len(pcm),
				err,
				src.Samples*bytesPerFrame,
			)
		}

		// The middle of the output, clear of the start of the tempo filter and of the end of the tone.
		lo, hi := int(src.Samples)/5, int(src.Samples)*4/5
		hz := float64(crossings(pcm, lo, hi)) / 2 * audio.SampleRate / float64(hi-lo)

		t.Logf("%d/%d: %d samples, %.1f Hz", c.num, c.den, src.Samples, hz)

		if hz < 440*0.97 || hz > 440*1.03 {
			t.Errorf("%d/%d: the tone is at %.1f Hz, want 440 (a resample would give %.0f)",
				c.num, c.den, hz, 440*float64(c.num)/float64(c.den))
		}
	}
}

// A speed curve inside a composition at a speed keeps following the picture: the time scale of the curve moves
// the frame boundaries, so the burst that the ramp reaches at frame r comes at r over the speed (SDD-004
// section 7).
func TestCompositionSpeedCurvedSourceFollowsTheScale(t *testing.T) {
	setup(t)

	doc := loadScene(t, "clip-caption")
	fps := doc.Settings.FPS
	d := fps.Num / fps.Den

	ramp := func(scale *timeline.Rational) *plan.SpeedRamp {
		return &plan.SpeedRamp{
			Points: []timeline.SpeedPoint{
				{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}}, {T: 1, Speed: timeline.Rational{Num: 3, Den: 1}},
			},
			ItemFrames: d, TimeScale: scale,
		}
	}

	at := func(scale *timeline.Rational) int {
		pcm := shapedDecode(t, plan.AudioSource{
			SourceStart: 0, Samples: samplesAt(fps, d), Speed: timeline.Rational{Num: 2, Den: 1}, Curve: ramp(scale),
		})
		first, _ := loudSpan(t, pcm)

		return first
	}

	plain := at(nil)
	two := at(&timeline.Rational{Num: 2, Den: 1})
	half := at(&timeline.Rational{Num: 1, Den: 2})

	if abs(two-plain/2) > 20 || abs(half-plain*2) > 40 {
		t.Errorf(
			"the burst is at sample %d, and at %d and %d under the time scales 2 and 1/2: want half and double",
			plain,
			two,
			half,
		)
	}
}

package export

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/audio"
)

// loudSpan is the first and the last output sample whose left channel is above a quarter of the peak.
func loudSpan(t *testing.T, pcm []byte) (first, last int) {
	t.Helper()

	n := len(pcm) / bytesPerFrame
	at := func(i int) int {
		v := int(int16(uint16(pcm[i*bytesPerFrame]) | uint16(pcm[i*bytesPerFrame+1])<<8))
		return max(v, -v)
	}

	peak := 0
	for i := range n {
		peak = max(peak, at(i))
	}

	if peak == 0 {
		t.Fatal("the audio is silent")
	}

	first, last = -1, -1

	for i := range n {
		if at(i) > peak/4 {
			if first < 0 {
				first = i
			}

			last = i
		}
	}

	return first, last
}

// shapedCase decodes a source of the click file, which has one 10 ms burst at 0.5 s (samples 24000 to 24480
// of the source), and finds where the burst falls in the output (SDD-004 sections 3.11 and 3.12).
var shapedRuns atomic.Int32

func shapedDecode(t *testing.T, src plan.AudioSource) []byte {
	t.Helper()

	doc := loadScene(t, "clip-caption")
	job := newJob(t, doc, "shaped-audio", "unused.mp4")

	const id = "ast_oneclick"

	in := ingestClip(t, fixtureSpec{Name: "one-click.wav", OneClick: true})
	doc.Assets[id] = timeline.Asset{Kind: timeline.AssetAudio}
	useAsset(doc, id, in)

	s := testState(t, job, fmt.Sprintf("shaped-audio-run-%d", shapedRuns.Add(1)))
	src.AssetID, src.ItemID, src.Gain = id, "i", gp(0, 1)

	rc, err := (&audioOpener{s: s}).Open(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = rc.Close() }()

	pcm, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}

	return pcm
}

func TestReversedAudioPlaysTheSourceBackwards(t *testing.T) {
	setup(t)

	doc := loadScene(t, "clip-caption")
	frame := samplesAt(doc.Settings.FPS, 1)
	one := timeline.Rational{Num: 1, Den: 1}

	// Forwards, the burst is at its place in the source.
	fwd := shapedDecode(t, plan.AudioSource{SourceStart: 0, Samples: 2 * audio.SampleRate, Speed: one})
	f0, f1 := loudSpan(t, fwd)

	if f0 < 24000 || f0 > 24100 || f1 < 24400 || f1 > 24490 {
		t.Fatalf("the burst of the source is at samples %d to %d, want about 24000 to 24480", f0, f1)
	}

	// Reversed, from the end of the source frame that holds the last sample: output sample j reads the source
	// at E - j, so the burst is at E - f1 to E - f0.
	start := 2*audio.SampleRate - frame
	rev := shapedDecode(t, plan.AudioSource{
		SourceStart: start, Samples: 2 * audio.SampleRate, Speed: one, Reverse: true,
	})
	e := int(start + frame - 1)
	r0, r1 := loudSpan(t, rev)

	if r0 != e-f1 || r1 != e-f0 {
		t.Errorf("the reversed burst is at samples %d to %d, want %d to %d", r0, r1, e-f1, e-f0)
	}

	// At speed 2 the reversed range is half as long and the burst is at half the distance from the end.
	two := timeline.Rational{Num: 2, Den: 1}
	fast := shapedDecode(t, plan.AudioSource{
		SourceStart: start, Samples: audio.SampleRate, Speed: two, Reverse: true,
	})
	g0, g1 := loudSpan(t, fast)
	want0, want1 := (e-f1)/2, (e-f0)/2

	if abs(g0-want0) > 400 || abs(g1-want1) > 400 {
		t.Errorf("the reversed burst at speed 2 is at samples %d to %d, want about %d to %d", g0, g1, want0, want1)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

func TestSpeedCurveMovesTheSoundWithTheFrames(t *testing.T) {
	setup(t)

	doc := loadScene(t, "clip-caption")
	fps := doc.Settings.FPS
	d := fps.Num / fps.Den // one second of frames

	total := samplesAt(fps, d)
	pts := func(a, b int) []timeline.SpeedPoint {
		return []timeline.SpeedPoint{
			{T: 0, Speed: timeline.Rational{Num: a, Den: 1}}, {T: 1, Speed: timeline.Rational{Num: b, Den: 1}},
		}
	}

	// A constant 2x curve reads the source twice as fast: the burst at 0.5 s comes at 0.25 s.
	two := shapedDecode(t, plan.AudioSource{
		SourceStart: 0, Samples: total, Speed: timeline.Rational{Num: 2, Den: 1},
		Curve: &plan.SpeedRamp{Points: pts(2, 2), ItemFrames: d},
	})
	a0, a1 := loudSpan(t, two)

	if abs(a0-12000) > 100 || abs(a1-12240) > 100 {
		t.Errorf("the burst at a constant 2x is at samples %d to %d, want about 12000 to 12240", a0, a1)
	}

	// A ramp from 1x to 3x reads the first frames slowly and the last ones fast: by the frame r the item has
	// read Offset(r) frames, so the burst at half a second is where Offset(r) reaches 15.
	ramp := &plan.SpeedRamp{Points: pts(1, 3), ItemFrames: d}
	slow := shapedDecode(t, plan.AudioSource{
		SourceStart: 0, Samples: total, Speed: timeline.Rational{Num: 2, Den: 1}, Curve: ramp,
	})
	b0, _ := loudSpan(t, slow)

	r := 0
	for timeline.CurveOffset(ramp.Points, d, r) < d/2 {
		r++
	}

	want := int(samplesAt(fps, r))
	if abs(b0-want) > int(2*samplesAt(fps, 1)) {
		t.Errorf("the burst on a 1x to 3x ramp is at sample %d, want about %d (frame %d)", b0, want, r)
	}

	// With reverse the same ramp is read from the end of the last source frame: output sample j reads the source
	// at E - rel(j), so the burst (source samples 24000 to 24480) comes where rel reaches E - 24480.
	end := 2*audio.SampleRate - samplesAt(fps, 1)
	rev := shapedDecode(t, plan.AudioSource{
		SourceStart: end, Samples: total, Speed: timeline.Rational{Num: 2, Den: 1}, Curve: ramp, Reverse: true,
	})
	c0, _ := loudSpan(t, rev)

	r = 0
	for samplesAt(fps, timeline.CurveOffset(ramp.Points, d, r)) < end+samplesAt(fps, 1)-1-24480 {
		r++
	}

	if want := int(samplesAt(fps, r)); abs(c0-want) > int(2*samplesAt(fps, 1)) {
		t.Errorf("the burst of the reversed ramp is at sample %d, want about %d (frame %d)", c0, want, r)
	}
}

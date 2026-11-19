package export

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// ingested makes the assets of a built case ready to decode.
func (c alphaCase) ingested(t *testing.T) *timeline.Document {
	t.Helper()

	for id, spec := range c.fixtures {
		useAsset(c.doc, id, ingestClip(t, spec))
	}

	return c.doc
}

// The video of an item with a speed curve or with reverse shows the source frame the plan names (SDD-004
// sections 3.11 and 3.12): frame r of the shaped item is, byte for byte, the frame the plain clip shows at
// that source frame. The portrait clip carries a timecode and a frame number, so no two frames are equal.
func TestSpeedCurveAndReverseDecodeTheRightSourceFrames(t *testing.T) {
	setup(t)

	plainAt := func(frames []int) [][]byte {
		b := newCase("plain", rate(30, 1))
		b.clip(b.track(), b.video(portrait), 0, 60, nil)

		return renderDoc(t, b.build(frames...).ingested(t), "plain", 0, frames)
	}

	ramp := []timeline.SpeedPoint{
		{T: 0, Speed: rate(1, 1)},
		{T: 1, Speed: rate(3, 1)},
	}

	cases := []struct {
		name string
		set  func(it *timeline.Item)
		dur  int
		src  func(r int) int
	}{
		{"reverse", func(it *timeline.Item) { it.Reverse = true }, 10, func(r int) int { return 20 + 9 - r }},
		{
			"curve",
			func(it *timeline.Item) { it.SpeedCurve, it.Speed = ramp, timeline.CurveMean(ramp) },
			8,
			func(r int) int { return 20 + timeline.CurveOffset(ramp, 8, r) },
		},
		{
			"curve reversed",
			func(it *timeline.Item) { it.SpeedCurve, it.Speed, it.Reverse = ramp, timeline.CurveMean(ramp), true },
			8,
			func(r int) int { return 20 + 16 - 1 - timeline.CurveOffset(ramp, 8, r) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newCase(tc.name, rate(30, 1))
			b.clip(b.track(), b.video(portrait), 0, tc.dur, func(it *timeline.Item) {
				it.SourceIn = 20
				tc.set(it)
			})

			var frames, want []int
			for r := range tc.dur {
				frames, want = append(frames, r), append(want, tc.src(r))
			}

			got := renderDoc(t, b.build(frames...).ingested(t), strings.ReplaceAll(tc.name, " ", "-"), 0, frames)
			ref := plainAt(want)

			for r := range got {
				if !bytes.Equal(got[r], ref[r]) {
					t.Errorf("frame %d is not the source frame %d", r, want[r])
				}
			}

			// Two different source frames are two different pictures, so the comparison means something.
			if bytes.Equal(ref[0], ref[len(ref)-1]) {
				t.Fatal("the first and the last reference frames are equal")
			}
		})
	}
}

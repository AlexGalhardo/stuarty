package lint

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// compositions reports the composition items whose timeline is not in the production (missing or in the
// trash) and, in a Full lint, those that read past the end of their timeline or an empty one (SDD-001 §3.5.1).
// It needs Env.Nested; without it nothing is reported.
func (c *check) compositions(full bool) {
	if c.env.Nested == nil {
		return
	}

	for _, it := range c.sorted() {
		if it.Type != timeline.ItemComposition {
			continue
		}

		nested, ok := c.env.Nested(it.Timeline)
		if !ok || nested == nil {
			c.add(CodeMediaMissing, it.ID, itemRange(it),
				fmt.Sprintf("the timeline %s is not in the project; the composition cannot be rendered.", it.Timeline),
				"restore the timeline from the trash or delete the composition")

			continue
		}

		if !full {
			continue
		}

		end := 0
		for id := range nested.Items {
			n := nested.Items[id]
			end = max(end, n.Start+n.Duration)
		}

		last := int64(it.SourceIn) + int64(it.Duration) - 1
		last = rateFrames(last, c.doc.Settings.FPS, nested.Settings.FPS)

		if end == 0 {
			c.add(CodeSourceRange, it.ID, itemRange(it),
				fmt.Sprintf("the timeline %s of the composition is empty.", it.Timeline),
				"put clips in that timeline or delete the composition")
		} else if last >= int64(end) {
			c.add(CodeSourceRange, it.ID, itemRange(it),
				fmt.Sprintf("the composition shows frame %d of the timeline %s, which has %d frames.",
					last, it.Timeline, end),
				"shorten the composition or lengthen that timeline")
		}
	}
}

// rateFrames converts a frame number of the host rate into a frame of the nested rate, rounding down.
func rateFrames(n int64, host, nested timeline.Rational) int64 {
	if host.Num == 0 || nested.Num == 0 {
		return n
	}

	return n * int64(host.Den) * int64(nested.Num) / (int64(host.Num) * int64(nested.Den))
}

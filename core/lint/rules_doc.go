package lint

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// maxSeconds is the YouTube Shorts limit that L_DURATION_LIMIT warns about.
const maxSeconds = 180

// timelineEnd is the end of the last item on any track, in frames.
func (c *check) timelineEnd() int {
	end := 0

	for _, it := range c.sorted() {
		end = max(end, it.Start+it.Duration)
	}

	return end
}

// visualEnd is the end of the last visible visual item, in frames: where the picture
// ends, even when an audio item or a hidden track runs on.
func (c *check) visualEnd() int {
	end := 0

	for _, it := range c.sorted() {
		if it.Type != timeline.ItemAudio && c.visibleTrack(it.Track) {
			end = max(end, it.Start+it.Duration)
		}
	}

	return end
}

// pictureEnd is the end of the last visible clip or full-frame shape: where the picture ends, even when a
// bar, a title or a caption on another track runs on. A fade out to the background there is deliberate
// (VV-202).
func (c *check) pictureEnd() int {
	end := 0

	for _, it := range c.sorted() {
		if c.visibleTrack(it.Track) && (it.Type == timeline.ItemClip || it.Type == timeline.ItemShape && c.covers(it)) {
			end = max(end, it.Start+it.Duration)
		}
	}

	return end
}

// durationLimit warns when a portrait or square timeline is longer than 180 seconds.
// The limit belongs to the short vertical platforms, so a landscape project (a YouTube
// video) is left alone (VV-021).
func (c *check) durationLimit() {
	fps := c.doc.Settings.FPS
	end := c.timelineEnd()

	if c.doc.Settings.Width > c.doc.Settings.Height || fps.Num < 1 || fps.Den < 1 ||
		int64(end)*int64(fps.Den) <= int64(maxSeconds)*int64(fps.Num) {
		return
	}

	limit := int64(maxSeconds) * int64(fps.Num) / int64(fps.Den)
	seconds := float64(end) * float64(fps.Den) / float64(fps.Num)

	c.add(
		CodeDurationLimit,
		"",
		&Range{Start: limit, End: int64(end)},
		fmt.Sprintf(
			"the timeline lasts %.3f s, longer than the %d s limit of short video platforms.",
			seconds,
			maxSeconds,
		),
		"trim the timeline or accept it",
	)
}

// covers says whether an item fills the whole frame, opaque, unrotated and in its
// default position, with no mask, in the normal blend and nothing that animates or fades it: what it takes to hide the
// items below.
func (c *check) covers(it *timeline.Item) bool {
	if it.Opacity != 1 || it.Mask != nil || it.X != nil || it.Y != nil || it.Scale < 1 || it.Rotation != 0 ||
		it.TransitionIn != nil || it.TransitionOut != nil {
		return false
	}

	// Every blend other than normal mixes the item with what lies beneath it (SDD-004 section 3.8), so the
	// item below still shows (VV-218).
	if it.Blend != "" && it.Blend != "normal" {
		return false
	}

	for _, p := range []string{"opacity", "x", "y", "scale", "rotation"} {
		if len(it.Keyframes[p]) > 0 {
			return false
		}
	}

	st := c.doc.Settings

	switch it.Type {
	case timeline.ItemClip:
		a, ok := c.doc.Assets[it.Asset]
		if !ok || it.Crop != nil {
			return false
		}

		sameAspect := a.Width > 0 && a.Height > 0 && a.Width*st.Height == a.Height*st.Width

		return it.Fit == "cover" || it.Fit == "contain" && sameAspect
	case timeline.ItemShape:
		fill := string(it.Fill)
		opaque := len(fill) == 7 && strings.HasPrefix(fill, "#") ||
			len(fill) == 9 && strings.HasPrefix(fill, "#") && strings.EqualFold(fill[7:], "FF")

		return it.Shape == "rect" && opaque && it.Width*it.Scale >= float64(st.Width) &&
			it.Height*it.Scale >= float64(st.Height)
	case timeline.ItemAudio, timeline.ItemText, timeline.ItemCaption, timeline.ItemComposition:
	}

	return false
}

// occluded reports a clip or shape that an item on a higher visible track hides for
// its whole range.
func (c *check) occluded() {
	for _, low := range c.sorted() {
		if (low.Type != timeline.ItemClip && low.Type != timeline.ItemShape) || !c.visibleTrack(low.Track) {
			continue
		}

		for _, up := range c.sorted() {
			if up.ID == low.ID || (up.Type != timeline.ItemClip && up.Type != timeline.ItemShape) ||
				!c.visibleTrack(up.Track) || c.doc.Tracks[up.Track].Order >= c.doc.Tracks[low.Track].Order ||
				up.Start > low.Start || up.Start+up.Duration < low.Start+low.Duration || !c.covers(up) {
				continue
			}

			c.add(CodeOccluded, low.ID, itemRange(low),
				fmt.Sprintf("%s is hidden for its whole range by %s on a higher track.", low.ID, up.ID),
				"move or delete one of the two")

			break
		}
	}
}

// audioOverlap reports audible clips on different tracks that play at the same time.
func (c *check) audioOverlap() {
	var clips []*timeline.Item

	for _, it := range c.sorted() {
		if it.Type == timeline.ItemClip && c.audible(it) {
			clips = append(clips, it)
		}
	}

	slices.SortStableFunc(clips, func(a, b *timeline.Item) int { return a.Start - b.Start })

	for i, a := range clips {
		for _, b := range clips[i+1:] {
			start, end := max(a.Start, b.Start), min(a.Start+a.Duration, b.Start+b.Duration)
			if a.Track == b.Track || start >= end {
				continue
			}

			c.add(CodeAudioOverlap, a.ID, &Range{Start: int64(start), End: int64(end)},
				fmt.Sprintf("%s and %s both play sound during frames %d-%d.", a.ID, b.ID, start, end-1),
				"mute one with item.set_audio audio_muted or move one")
		}
	}
}

// processorChains reports a limiter that another processor follows, and a second noise reduction, in the chain of
// an item or a track (SDD-001 section 3.5).
func (c *check) processorChains() {
	report := func(id string, chain []timeline.Processor, rng *Range) {
		notLast, repeated := timeline.ChainWarnings(chain)
		for _, i := range notLast {
			c.add(CodeAudioLimiterNotLast, id, rng,
				fmt.Sprintf("The limiter at place %d of the chain of %s is not the last processor.", i, id),
				"move it last with audio.processor_move")
		}

		for _, i := range repeated {
			c.add(CodeAudioNoiseReductionRepeats, id, rng,
				fmt.Sprintf("The chain of %s has a second noise reduction at place %d.", id, i), "remove one")
		}
	}

	for _, id := range slices.Sorted(maps.Keys(c.doc.Tracks)) {
		report(id, c.doc.Tracks[id].Processors, nil)
	}

	for _, it := range c.sorted() {
		report(it.ID, it.Processors, itemRange(it))
	}
}

// transitionEmpty reports a transition that has nothing visible beneath it, so that
// it fades or slides over the background. A transition_in at frame 0 and a
// transition_out at the end of the picture (the last visible item) are expected and
// left alone (VV-021).
func (c *check) transitionEmpty() {
	end := c.visualEnd()
	pictureEnd := c.pictureEnd()

	for _, it := range c.sorted() {
		if it.Type == timeline.ItemAudio || !c.visibleTrack(it.Track) {
			continue
		}

		itemEnd := it.Start + it.Duration

		for _, t := range []struct {
			name       string
			tr         *timeline.Transition
			from, to   int
			expectedAt bool
		}{
			{"transition_in", it.TransitionIn, it.Start, it.Start + durationOf(it.TransitionIn), it.Start == 0},
			{"transition_out", it.TransitionOut, itemEnd - durationOf(it.TransitionOut), itemEnd, itemEnd == end || itemEnd >= pictureEnd},
		} {
			if t.tr == nil || t.tr.Kind == "dip" || t.expectedAt || c.coveredBeneath(it, t.from, t.to) {
				continue
			}

			c.add(CodeTransitionEmpty, it.ID, &Range{Start: int64(t.from), End: int64(t.to)},
				fmt.Sprintf("the %s of %s covers frames %d-%d, where nothing visible lies beneath it.",
					t.name, it.ID, t.from, t.to-1),
				"place a clip under it or remove the transition")
		}
	}
}

func durationOf(t *timeline.Transition) int {
	if t == nil {
		return 0
	}

	return t.Duration
}

// coveredBeneath says whether every frame of [from, to) has another visible visual item
// on a lower track. It sorts the spans of those items once and sweeps them, so the cost
// does not depend on how long the transition or the items are.
func (c *check) coveredBeneath(it *timeline.Item, from, to int) bool {
	var spans [][2]int

	for _, o := range c.sorted() {
		if o.ID != it.ID && o.Type != timeline.ItemAudio && c.visibleTrack(o.Track) &&
			c.doc.Tracks[o.Track].Order > c.doc.Tracks[it.Track].Order {
			spans = append(spans, [2]int{o.Start, o.Start + o.Duration})
		}
	}

	slices.SortFunc(spans, func(a, b [2]int) int { return a[0] - b[0] })

	reach := from

	for _, sp := range spans {
		if sp[0] > reach {
			break
		}

		reach = max(reach, sp[1])
		if reach >= to {
			return true
		}
	}

	return reach >= to
}

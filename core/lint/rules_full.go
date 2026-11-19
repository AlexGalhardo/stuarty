package lint

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	// frameTolerance is how far the first word of a caption may sit from the transcript.
	frameTolerance = 3
	// loudnessTolerance is the largest difference to the target, in LU, left alone.
	loudnessTolerance = 6
	// peakCeilingDB and maxLimitingDB are the rule of the export gain (engine/audio): the true peak
	// of the mix may not pass peakCeilingDB after the gain, and the limiter takes off at most
	// maxLimitingDB to get there, so the gain never exceeds the headroom plus maxLimitingDB.
	peakCeilingDB = -1.5
	maxLimitingDB = 12
	// clipLevel is the sum of source peaks above which the mix clips.
	clipLevel = 1.0
)

// captionDesync is L_CAPTION_DESYNC: the item part (a caption that extends beyond its
// linked clip) and, in a full run with a transcript, the part that compares the first
// word with where the transcript puts it.
func (c *check) captionDesync(full bool) {
	for _, it := range c.sorted() {
		clip, ok := c.doc.Items[it.LinkedItem]
		if it.Type != timeline.ItemCaption || it.LinkedItem == "" || !ok || clip.Type != timeline.ItemClip {
			continue
		}

		if it.Start+it.Duration > clip.Start+clip.Duration {
			c.add(CodeCaptionDesync, it.ID, itemRange(it),
				fmt.Sprintf("the caption ends at frame %d, after its clip %s ends at frame %d.",
					it.Start+it.Duration, clip.ID, clip.Start+clip.Duration),
				"run caption.retime_from_clip")

			continue
		}

		if full {
			c.firstWordDrift(it, &clip)
		}
	}
}

// firstWordDrift compares the first word of a caption with the transcript word of the
// same text nearest to it, mapped through the start, in-point and speed of the clip.
func (c *check) firstWordDrift(it, clip *timeline.Item) {
	if c.env.Artifacts == nil || len(it.Words) == 0 {
		return
	}

	words, ok := c.env.Artifacts.Transcript(clip.Asset)
	if !ok {
		return
	}

	// A curve or a reverse has no constant mapping from the transcript to the timeline.
	if clip.Reverse || len(clip.SpeedCurve) > 0 {
		return
	}

	first := it.Words[0]
	at := it.Start + first.Start
	num, den, si := clip.Speed.Num, clip.Speed.Den, clip.SourceIn
	srcEnd := si + (clip.Duration*num+den-1)/den
	best, found := 0, false

	for _, w := range words {
		if w.Start < si || w.Start >= srcEnd || !strings.EqualFold(w.Text, first.Text) {
			continue
		}

		mapped := clip.Start + ((w.Start-si)*den+num-1)/num
		if !found || abs(mapped-at) < abs(best-at) {
			best, found = mapped, true
		}
	}

	if found && abs(best-at) > frameTolerance {
		c.add(CodeCaptionDesync, it.ID, itemRange(it),
			fmt.Sprintf("the first word starts at frame %d; the transcript puts it at frame %d.", at, best),
			"run caption.retime_from_clip")
	}
}

func abs(v int) int { return max(v, -v) }

// gainAt is the gain of an audible source at frame r of the item: volume, fades and
// volume keyframes.
func gainAt(it *timeline.Item, r int) float64 {
	g := timeline.ValueAt(it.Keyframes["volume"], r, it.Volume)

	if it.FadeIn > 0 && r < it.FadeIn {
		g *= float64(r) / float64(it.FadeIn)
	}

	if left := it.Duration - r; it.FadeOut > 0 && left < it.FadeOut {
		g *= float64(left) / float64(it.FadeOut)
	}

	return g
}

// sources are the audible items, in id order.
func (c *check) sources() []*timeline.Item {
	var out []*timeline.Item

	for _, it := range c.sorted() {
		if c.audible(it) {
			out = append(out, it)
		}
	}

	return out
}

// mixPeaks is the estimated peak of the mix in each 20 ms window: the sum, over the
// audible sources, of the peak of the asset times the gain at that moment.
func (c *check) mixPeaks(rate float64) map[int]float64 {
	sum := map[int]float64{}

	for _, it := range c.sources() {
		peaks, ok := c.env.Artifacts.Peaks(it.Asset)
		if !ok {
			continue
		}

		first := int(float64(it.Start) / rate * 1000 / Window)
		last := int(float64(it.Start+it.Duration) / rate * 1000 / Window)

		for w := first; w <= last; w++ {
			// The frame of the item at the middle of the window.
			r := int((float64(w)+0.5)*Window/1000*rate) - it.Start
			if r < 0 || r >= it.Duration {
				continue
			}

			secs := float64(it.SourceIn+it.SourceOffset(r)) / rate
			if i := int(secs * 1000 / Window); i >= 0 && i < len(peaks) {
				sum[w] += peaks[i] * gainAt(it, r)
			}
		}
	}

	return sum
}

// audioClip is L_AUDIO_CLIP: in some 20 ms window the sum of the source peaks times
// their gains exceeds 1.0; the message gives the first range and how many there are.
func (c *check) audioClip() {
	fps := c.doc.Settings.FPS
	if c.env.Artifacts == nil || fps.Num < 1 || fps.Den < 1 {
		return
	}

	rate := float64(fps.Num) / float64(fps.Den)
	sum := c.mixPeaks(rate)

	var hot []int

	for w, v := range sum {
		if v > clipLevel {
			hot = append(hot, w)
		}
	}

	if len(hot) == 0 {
		return
	}

	slices.Sort(hot)

	runs := [][2]int{{hot[0], hot[0]}}

	for _, w := range hot[1:] {
		if last := &runs[len(runs)-1]; last[1] == w-1 {
			last[1] = w
		} else {
			runs = append(runs, [2]int{w, w})
		}
	}

	start := int(float64(runs[0][0]) * Window / 1000 * rate)
	end := int(math.Ceil(float64(runs[0][1]+1) * Window / 1000 * rate))

	c.add(CodeAudioClip, "", &Range{Start: int64(start), End: int64(end)},
		fmt.Sprintf("the estimated mix peaks above 0 dBFS in %d range(s); the first is frames %d-%d.",
			len(runs), start, end-1),
		"lower volume with item.set_audio")
}

// loudness is L_LOUDNESS: the energy of the sources, each with the loudness of its
// asset, its gain and the share of the timeline it plays, gives the mix loudness; more
// than 6 LU from the target is a finding that states the gain the export would apply.
// reachableGain caps the gain that reaches the target by what the export can do: the peak of
// the mix plus the gain may not pass peakCeilingDB, and the limiter takes off at most
// maxLimitingDB (VV-074). Without the peaks of the assets the estimate is not capped.
func (c *check) reachableGain(gain float64) float64 {
	fps := c.doc.Settings.FPS
	if fps.Num < 1 || fps.Den < 1 {
		return gain
	}

	peak := 0.0
	for _, v := range c.mixPeaks(float64(fps.Num) / float64(fps.Den)) {
		peak = max(peak, v)
	}

	if peak <= 0 {
		return gain
	}

	return math.Round(min(gain, peakCeilingDB-20*math.Log10(peak)+maxLimitingDB)*10) / 10
}

func (c *check) loudness() {
	end := c.timelineEnd()
	if c.env.Artifacts == nil || end == 0 {
		return
	}

	energy := 0.0

	for _, it := range c.sources() {
		l, ok := c.env.Artifacts.Loudness(it.Asset)
		if !ok {
			continue
		}

		power := 0.0
		for r := range it.Duration {
			power += gainAt(it, r) * gainAt(it, r)
		}

		energy += math.Pow(10, l.LUFS/10) * power / float64(end)
	}

	if energy <= 0 {
		return
	}

	estimate := math.Round(10*math.Log10(energy)*10) / 10
	target := c.doc.Settings.LoudnessTarget

	if math.Abs(target-estimate) <= loudnessTolerance+1e-9 {
		return
	}

	gain := c.reachableGain(target - estimate)

	c.add(
		CodeLoudness,
		"",
		nil,
		fmt.Sprintf(
			"the estimated mix loudness is %.1f LUFS; the target is %.1f LUFS, so the export applies a gain of %+.1f dB.",
			estimate,
			target,
			gain,
		),
		"adjust volumes or accept the normalization",
	)
}

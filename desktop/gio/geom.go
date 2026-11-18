package gioapp

import (
	"math"
	"strconv"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/core/timeval"
)

const (
	minPxPerSec = 24
	maxPxPerSec = 360
)

// frameToX maps a frame to a pixel in the timeline content, before scrolling.
func frameToX(frame int, fps timeval.FPS, pxPerSec float64) int {
	if fps.Num <= 0 || fps.Den <= 0 {
		fps = timeval.FPS{Num: 30, Den: 1}
	}

	sec := float64(frame) * float64(fps.Den) / float64(fps.Num)

	return int(math.Round(sec * pxPerSec))
}

// xToFrame maps a pixel in the timeline content, before scrolling, to a frame.
func xToFrame(x int, fps timeval.FPS, pxPerSec float64) int {
	if fps.Num <= 0 || fps.Den <= 0 || pxPerSec <= 0 {
		return 0
	}

	sec := float64(x) / pxPerSec
	frame := int(math.Round(sec * float64(fps.Num) / float64(fps.Den)))

	if frame < 0 {
		return 0
	}

	return frame
}

func framesArg(frame int) string {
	return strconv.Itoa(frame) + "f"
}

func itemLabel(it timeline.Item, assets map[string]timeline.Asset) string {
	if it.Name != "" {
		return it.Name
	}

	if it.Text != "" {
		return it.Text
	}

	if a, ok := assets[it.Asset]; ok && a.Name != "" {
		return a.Name
	}

	if it.Type == "" {
		return "clip"
	}

	return string(it.Type)
}

// hitKind is what a pointer press on the timeline landed on.
type hitKind int

const (
	hitEmpty hitKind = iota
	hitRuler
	hitBody
	hitStart
	hitEnd
)

type hit struct {
	kind   hitKind
	itemID string
	frame  int
}

type placedClip struct {
	id         string
	start, dur int
	row        int
}

// hitTimeline reports the clip or ruler under a point in timeline-content pixels.
// y is relative to the top of the content, which starts with the ruler.
func hitTimeline(clips []placedClip, x, y, rulerH, rowH, edge int, frameAt func(x int) int) hit {
	frame := 0
	if frameAt != nil {
		frame = frameAt(x)
	}

	if y < rulerH {
		return hit{kind: hitRuler, frame: frame}
	}

	row := (y - rulerH) / rowH
	best := hit{kind: hitEmpty, frame: frame}

	for _, c := range clips {
		if c.row != row || c.dur <= 0 {
			continue
		}

		// The caller passes x already in the same space as the clip edges.
		if x < c.start || x >= c.start+c.dur {
			continue
		}

		kind := hitBody
		if c.dur >= edge*3 && x < c.start+edge {
			kind = hitStart
		} else if c.dur >= edge*3 && x >= c.start+c.dur-edge {
			kind = hitEnd
		}

		best = hit{kind: kind, itemID: c.id, frame: frame}
	}

	return best
}

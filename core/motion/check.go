package motion

import (
	"fmt"
	"sort"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The fields a mismatch names (SDD-011 section 15.3).
const (
	FieldKind      = "kind"
	FieldSize      = "size"
	FieldFrameRate = "frame_rate"
	FieldDuration  = "duration"
	FieldAlpha     = "alpha"
)

// Mismatch is one way a rendered file does not fit the brief, with the exact value expected and the value found.
type Mismatch struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Found    string `json:"found"`
	Hint     string `json:"hint"`
}

// Check compares an asset with the brief and lists every mismatch; none means it fits. Nothing is stretched: a
// file that does not fit is reported, never adjusted. A video is checked for size, rate, duration and alpha, an
// image (a still holds for any time) for size and alpha, and an audio file or a lookup table for its kind.
func Check(b Brief, a timeline.Asset) []Mismatch {
	var out []Mismatch

	if a.Kind != timeline.AssetVideo && a.Kind != timeline.AssetImage {
		return append(out, Mismatch{
			Field: FieldKind, Expected: "video or image", Found: string(a.Kind),
			Hint: "render a video or an image; an audio file or a lookup table cannot be placed as a clip",
		})
	}

	if a.Width != b.Width || a.Height != b.Height {
		out = append(out, Mismatch{
			Field:    FieldSize,
			Expected: fmt.Sprintf("%dx%d", b.Width, b.Height),
			Found:    fmt.Sprintf("%dx%d", a.Width, a.Height),
			Hint:     fmt.Sprintf("render at %dx%d, the size of the project", b.Width, b.Height),
		})
	}

	if a.Kind == timeline.AssetVideo {
		if a.FrameRate.Num*b.FrameRate.Den != b.FrameRate.Num*a.FrameRate.Den {
			out = append(out, Mismatch{
				Field: FieldFrameRate, Expected: rate(b.FrameRate), Found: rate(a.FrameRate),
				Hint: "render at " + rate(b.FrameRate) + " frames per second, the rate of the project",
			})
		}

		if n, ok := timeline.AssetDuration(a, b.FrameRate); ok && n != b.DurationFrames {
			out = append(out, Mismatch{
				Field: FieldDuration, Expected: fmt.Sprintf("%df", b.DurationFrames), Found: fmt.Sprintf("%df", n),
				Hint: fmt.Sprintf("render exactly %d frames", b.DurationFrames),
			})
		}
	}

	if b.NeedsAlpha && !a.HasAlpha {
		out = append(out, Mismatch{
			Field:    FieldAlpha,
			Expected: "alpha",
			Found:    "no alpha",
			Hint:     "the clip goes over another track: render with transparency (ProRes 4444 in a MOV, or WebM VP9 with alpha)",
		})
	}

	return out
}

func rate(r timeline.Rational) string { return fmt.Sprintf("%d/%d", r.Num, r.Den) }

// Targets lists, in order, the clips of the document that read one of the assets of earlier (a set of asset ids,
// the earlier renders of one animation), except the new asset. It is the set a replacement by render key reaches.
func Targets(doc *timeline.Document, earlier map[string]bool, newAsset string) []string {
	var ids []string

	for id := range doc.Items {
		it := doc.Items[id]
		if it.Type == timeline.ItemClip && it.Asset != newAsset && earlier[it.Asset] {
			ids = append(ids, id)
		}
	}

	sort.Strings(ids)

	return ids
}

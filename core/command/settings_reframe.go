package command

import (
	"math"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// reframeSizes are the frame sizes of the aspect presets of settings.reframe.
var reframeSizes = map[string][2]int{
	"16:9": {1920, 1080},
	"9:16": {1080, 1920},
	"1:1":  {1080, 1080},
	"4:5":  {1080, 1350},
}

func init() {
	Register(Define(Spec{
		Name:    "settings.reframe",
		Summary: "Re-targets the project to another aspect ratio and refits the items to the new frame.",
		Description: "Sets the frame size from an aspect preset (16:9 is 1920x1080, 9:16 is 1080x1920, 1:1 is 1080x1080, " +
			"4:5 is 1080x1350) or from width and height. Like settings.set, the explicit x and y of the items and their " +
			"keyframes follow the size. Beyond that, text and shape items, and clips with fit none, are scaled by the " +
			"smaller of the two axis ratios so they keep their share of the frame instead of overflowing it. " +
			"Clips with fit cover or contain fill the frame by themselves; adjust their crop with clip.set_framing.",
		Example: `{"aspect": "9:16"}`,
		Errors:  []string{CodeValidation},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applySettingsReframe))
}

type reframeArgs struct {
	Aspect string `json:"aspect,omitempty" enum:"16:9,9:16,1:1,4:5" desc:"A preset frame size, or give width and height."`
	Width  *int   `json:"width,omitempty"                           desc:"Frame width in pixels; even."                   min:"16" max:"4096"`
	Height *int   `json:"height,omitempty"                          desc:"Frame height in pixels; even."                  min:"16" max:"4096"`
}

func applySettingsReframe(c *Context, a *reframeArgs) error {
	w, h, err := reframeSize(a)
	if err != nil {
		return err
	}
	s := &c.Doc.Settings
	oldW, oldH := s.Width, s.Height
	rx, ry := float64(w)/float64(oldW), float64(h)/float64(oldH)

	if err := c.checkReframeScales(min(rx, ry)); err != nil {
		return err
	}
	s.Width, s.Height = w, h
	c.SetEffective("width", w)
	c.SetEffective("height", h)
	c.SetEffective("rescaled_items", c.rescale(rx, ry))
	c.SetEffective("resized_items", c.resizeOverlays(min(rx, ry)))
	return nil
}

func reframeSize(a *reframeArgs) (w, h int, err *Error) {
	if a.Aspect != "" {
		if a.Width != nil || a.Height != nil {
			return 0, 0, invalidArg("args.aspect", "args: give aspect, or width and height, not both.").
				WithHint("Send {\"aspect\": \"9:16\"} or {\"width\": 1080, \"height\": 1920}.")
		}
		sz := reframeSizes[a.Aspect]
		return sz[0], sz[1], nil
	}
	if a.Width == nil || a.Height == nil {
		return 0, 0, invalidArg("args", "args: give aspect, or both width and height.").
			WithHint("Send {\"aspect\": \"9:16\"} or {\"width\": 1080, \"height\": 1920}.")
	}
	for _, dim := range []struct {
		name string
		v    int
	}{{"width", *a.Width}, {"height", *a.Height}} {
		if dim.v%2 != 0 {
			return 0, 0, invalidArg(
				"args."+dim.name,
				"args.%s must be even and between 16 and 4096, got %d.",
				dim.name,
				dim.v,
			).
				WithHint("Use %s %d or %d.", dim.name, dim.v-1, dim.v+1)
		}
	}
	return *a.Width, *a.Height, nil
}

// resizedByReframe says whether settings.reframe scales the item: the items that do not fill the
// frame by themselves.
func resizedByReframe(it *timeline.Item) bool {
	switch it.Type {
	case timeline.ItemText, timeline.ItemShape:
		return true
	case timeline.ItemClip:
		return it.Fit == "none"
	case timeline.ItemAudio, timeline.ItemCaption:
		return false
	}
	return false
}

// checkReframeScales refuses the reframe when a scale, or a scale keyframe, would leave 0.001 to 20.
func (c *Context) checkReframeScales(f float64) error {
	if f == 1 {
		return nil
	}
	for _, id := range c.sortedItemIDs() {
		it := c.Doc.Items[id]
		if !resizedByReframe(&it) {
			continue
		}
		vals := []float64{it.Scale}
		for _, kf := range it.Keyframes["scale"] {
			vals = append(vals, kf.Value)
		}
		for _, v := range vals {
			if n := v * f; n > 20 || round3(n) < 0.001 {
				return Errorf(
					CodeValidation,
					"Item %s would get the scale %s, outside 0.001 to 20.",
					id,
					trimFloat(round3(n)),
				).
					WithHint("Set the size with settings.set to keep the items as they are, or give the item a scale nearer to 1 first.").
					WithPath("args")
			}
		}
	}
	return nil
}

// resizeOverlays multiplies the scale of the items that resizedByReframe selects, and their scale
// keyframes, by f and returns how many items changed.
func (c *Context) resizeOverlays(f float64) int {
	if f == 1 {
		return 0
	}
	changed := 0
	for _, id := range c.sortedItemIDs() {
		it := c.Doc.Items[id]
		if !resizedByReframe(&it) {
			continue
		}
		it.Scale = math.Max(round3(it.Scale*f), 0.001)
		for i := range it.Keyframes["scale"] {
			it.Keyframes["scale"][i].Value = math.Max(round3(it.Keyframes["scale"][i].Value*f), 0.001)
		}
		c.Doc.Items[id] = it
		changed++
	}
	return changed
}

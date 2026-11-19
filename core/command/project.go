package command

import (
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:        "project.rename",
		Summary:     "Sets the project name.",
		Description: "Sets the name of the project. The name is stored exactly as given, without trimming.",
		Example:     `{"name": "Launch teaser v2"}`,
		Errors:      []string{CodeValidation},
		Scope:       ScopeEdit,
	}, applyProjectRename))

	Register(Define(Spec{
		Name:    "settings.set",
		Summary: "Sets the frame size, the background colour or the loudness target.",
		Description: "Sets the given settings. When the size changes, the explicit x and y of every visual item, and " +
			"the values of its x and y keyframes, are scaled by the ratio of new to old size on each axis; scale and " +
			"text size are untouched and items at the default position stay centred.",
		Example: `{"width": 720, "height": 1280}`,
		Errors:  []string{CodeValidation},
		Scope:   ScopeEdit,
	}, applySettingsSet))
}

type renameArgs struct {
	Name string `json:"name" min:"1" max:"120" desc:"The new name, 1 to 120 characters."`
}

func applyProjectRename(c *Context, a *renameArgs) error {
	c.Doc.Name = a.Name
	c.SetEffective("name", a.Name)
	return nil
}

type settingsArgs struct {
	Width          *int     `json:"width,omitempty"           min:"16"  max:"4096" desc:"Frame width in pixels; even."`
	Height         *int     `json:"height,omitempty"          min:"16"  max:"4096" desc:"Frame height in pixels; even."`
	Background     *string  `json:"background,omitempty"                           desc:"A hex colour, $name from template.colors, or transparent (no background colour; formats with alpha keep it)."`
	LoudnessTarget *float64 `json:"loudness_target,omitempty" min:"-30" max:"-5"   desc:"Integrated loudness target in LUFS."`
}

func applySettingsSet(c *Context, a *settingsArgs) error {
	if err := atLeastOne([]string{"width", "height", "background", "loudness_target"},
		a.Width != nil, a.Height != nil, a.Background != nil, a.LoudnessTarget != nil); err != nil {
		return err
	}
	for _, dim := range []struct {
		name string
		v    *int
	}{{"width", a.Width}, {"height", a.Height}} {
		if dim.v != nil && *dim.v%2 != 0 {
			return Errorf(CodeValidation, "args.%s must be even and between 16 and 4096, got %d.", dim.name, *dim.v).
				WithHint("Use %s %d or %d.", dim.name, *dim.v-1, *dim.v+1).WithPath("args." + dim.name)
		}
	}
	if a.Background != nil {
		if *a.Background != timeline.BackgroundTransparent {
			if err := c.checkColour("args.background", *a.Background); err != nil {
				return err
			}
		}
	}

	s := &c.Doc.Settings
	oldW, oldH := s.Width, s.Height
	if a.Width != nil {
		s.Width = *a.Width
		c.SetEffective("width", s.Width)
	}
	if a.Height != nil {
		s.Height = *a.Height
		c.SetEffective("height", s.Height)
	}
	if a.Background != nil {
		s.Background = timeline.Color(*a.Background)
		if isHex(*a.Background) {
			s.Background = timeline.Color(strings.ToUpper(*a.Background))
		}
		c.SetEffective("background", string(s.Background))
	}
	if a.LoudnessTarget != nil {
		s.LoudnessTarget = *a.LoudnessTarget
		c.SetEffective("loudness_target", s.LoudnessTarget)
	}
	c.SetEffective("rescaled_items", c.rescale(float64(s.Width)/float64(oldW), float64(s.Height)/float64(oldH)))
	return nil
}

// rescale scales the explicit x and y of every visual item and its x and y keyframes
// by the given ratios and returns the number of items changed.
func (c *Context) rescale(rx, ry float64) int {
	if rx == 1 && ry == 1 {
		return 0
	}
	changed := 0
	for _, id := range c.sortedItemIDs() {
		it := c.Doc.Items[id]
		if it.Type == timeline.ItemAudio {
			continue
		}
		hit := false
		if it.X != nil && rx != 1 {
			v := round3(*it.X * rx)
			it.X, hit = &v, true
		}
		if it.Y != nil && ry != 1 {
			v := round3(*it.Y * ry)
			it.Y, hit = &v, true
		}
		for prop, ratio := range map[string]float64{"x": rx, "y": ry} {
			if ratio == 1 {
				continue
			}
			for i := range it.Keyframes[prop] {
				it.Keyframes[prop][i].Value = round3(it.Keyframes[prop][i].Value * ratio)
				hit = true
			}
		}
		if hit {
			c.Doc.Items[id] = it
			changed++
		}
	}
	return changed
}

package image

import (
	"slices"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// Preset is a common size of an image and the smallest text the lint accepts on it (SDD-021 section 6).
type Preset struct {
	Name    string `json:"name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	MinText int    `json:"min_text"`
}

// DefaultMinText is the smallest text size of a document that has no preset.
const DefaultMinText = 24

// Presets are the sizes of SDD-021 section 6; they are data.
var Presets = []Preset{
	{Name: "video_thumbnail", Width: 1280, Height: 720, MinText: 36},
	{Name: "square_post", Width: 1080, Height: 1080, MinText: 32},
	{Name: "vertical_cover", Width: 1080, Height: 1920, MinText: 40},
}

// PresetByName returns the preset with the name.
func PresetByName(name string) (Preset, bool) {
	i := slices.IndexFunc(Presets, func(p Preset) bool { return p.Name == name })
	if i < 0 {
		return Preset{}, false
	}

	return Presets[i], true
}

// PresetNames lists the names of the presets.
func PresetNames() []string {
	out := make([]string, len(Presets))
	for i, p := range Presets {
		out[i] = p.Name
	}

	return out
}

// DefaultBackground is the background of a new document.
const DefaultBackground = "#FFFFFF"

func init() {
	register(
		command.DefineIn(command.Spec{
			Name:    "image.create",
			Summary: "Sets the canvas, name and background of a new image document.",
			Description: "Gives a new document its size, from a preset (video_thumbnail 1280x720, square_post 1080x1080, " +
				"vertical_cover 1080x1920) or from width and height (1 to 4096 each), its name and its background " +
				"(a colour, default #FFFFFF, or transparent). template true makes a template whose text and image layers " +
				"may be slots. Refused on a document that already has a canvas.",
			Example: `{"preset": "video_thumbnail", "name": "Episode 12", "background": "#101820"}`,
			Errors:  []string{command.CodeValidation, command.CodeInvalidState},
			Scope:   command.ScopeEdit,
		}, applyCreate),
		command.DefineIn(command.Spec{
			Name:    "canvas.resize",
			Summary: "Changes the size of the canvas.",
			Description: "mode keep (default) leaves every layer where it is. mode scale multiplies the x, y, w and h of " +
				"every layer by the ratio of the new size to the old, rounded half up; text sizes and stroke widths are " +
				"not scaled. The size is 1 to 4096 on each side.",
			Example: `{"width": 1080, "height": 1080, "mode": "keep"}`,
			Errors:  []string{command.CodeValidation},
			Scope:   command.ScopeEdit,
		}, applyResize),
	)
}

type createArgs struct {
	Preset     string `json:"preset,omitempty"     desc:"A named size; give it or width and height."                       enum:"video_thumbnail,square_post,vertical_cover"`
	Width      *int   `json:"width,omitempty"      desc:"Pixels, 1 to 4096."                                                                                                 min:"1" max:"4096"`
	Height     *int   `json:"height,omitempty"     desc:"Pixels, 1 to 4096."                                                                                                 min:"1" max:"4096"`
	Name       string `json:"name,omitempty"       desc:"At most 120 characters; default Untitled."                                                                                  max:"120"`
	Background string `json:"background,omitempty" desc:"A colour (#RRGGBB or #RRGGBBAA) or transparent; default #FFFFFF."`
	Template   bool   `json:"template,omitempty"   desc:"Make a template: its text and image layers may be slots."`
}

func applyCreate(c *command.Context, d *Document, a *createArgs) error {
	if d.Width != 0 || d.Height != 0 {
		return command.Errorf(command.CodeInvalidState, "The document already has a canvas of %dx%d.", d.Width, d.Height).
			WithHint("Change the size with canvas.resize.")
	}

	switch {
	case a.Preset != "" && (a.Width != nil || a.Height != nil):
		return invalid("args.preset", "Give a preset or a width and a height, not both.").
			WithHint("Remove width and height, or remove preset.")
	case a.Preset != "":
		p, _ := PresetByName(a.Preset)
		d.Width, d.Height, d.Preset = p.Width, p.Height, p.Name
	case a.Width != nil && a.Height != nil:
		d.Width, d.Height = *a.Width, *a.Height
	default:
		return invalid("args", "Give a preset, or both width and height.").
			WithHint("Use preset video_thumbnail, or width and height in pixels.")
	}

	d.Name = a.Name
	if d.Name == "" {
		d.Name = "Untitled"
	}

	d.Background = DefaultBackground

	if a.Background != "" {
		d.Background = Transparent

		if a.Background != Transparent {
			bg, err := colour("args.background", a.Background)
			if err != nil {
				return err
			}

			d.Background = bg
		}
	}

	d.Template = a.Template
	if d.Layers == nil {
		d.Layers = []Layer{}
	}

	c.SetEffective("width", d.Width)
	c.SetEffective("height", d.Height)
	c.SetEffective("background", d.Background)

	return nil
}

type resizeArgs struct {
	Width  int    `json:"width"          desc:"Pixels, 1 to 4096."       min:"1" max:"4096"`
	Height int    `json:"height"         desc:"Pixels, 1 to 4096."       min:"1" max:"4096"`
	Mode   string `json:"mode,omitempty" desc:"keep (default) or scale."                    enum:"keep,scale"`
}

func applyResize(c *command.Context, d *Document, a *resizeArgs) error {
	ow, oh := d.Width, d.Height

	if a.Mode == "scale" {
		var rescale func(ls []Layer)

		rescale = func(ls []Layer) {
			for i := range ls {
				l := &ls[i]
				l.X, l.Y = roundDiv(l.X*a.Width, ow), roundDiv(l.Y*a.Height, oh)

				if l.Kind != KindGroup {
					l.W, l.H = max(roundDiv(l.W*a.Width, ow), 1), max(roundDiv(l.H*a.Height, oh), 1)
				}

				rescale(l.Children)
			}
		}

		rescale(d.Layers)
	}

	d.Width, d.Height = a.Width, a.Height
	d.Preset = ""

	c.SetEffective("width", d.Width)
	c.SetEffective("height", d.Height)

	return nil
}

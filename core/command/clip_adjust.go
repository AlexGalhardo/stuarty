package command

import (
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.set_framing",
		Summary: "Sets how the asset of a clip is fitted into the frame and how much of each side is cropped.",
		Description: "Sets fit (cover, contain or none) and merges the given crop sides with the current ones. Each side " +
			"is a fraction of the asset from 0 to 1; left plus right and top plus bottom must stay below 1. " +
			"A side that has keyframes (crop_left, crop_top, crop_right, crop_bottom; see keyframe.set) cannot be set here.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "fit": "contain", "crop": {"left": 0.1, "right": 0.1}}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState},
		Scope:   ScopeEdit,
		Phase:   Alpha,
	}, applyClipSetFraming))

	Register(Define(Spec{
		Name:    "clip.set_color",
		Summary: "Sets the brightness, contrast, saturation, temperature, tint, hue, lookup table, HSL bands, wheels and curves of a clip.",
		Description: "Sets the given adjustments, rounded to 3 decimals; a value equal to the default (brightness 0, " +
			"contrast 1, saturation 1, temperature 0, tint 0, hue 0) clears the stored one. Positive temperature is " +
			"warmer, positive tint is more magenta, hue rotates the colours in degrees. lut_asset is the id of a lut asset (a " +
			".cube file) and lut_intensity how much of it is mixed in, 0 to 1, default 1; lut_intensity 0 removes the lookup table. " +
			"hsl bands (red, orange, yellow, green, cyan, blue, purple, magenta: hue, saturation, lightness, -1 to 1), " +
			"wheels (shadows, midtones, highlights: hue 0 to 360, amount 0 to 1, luma -1 to 1) and curves (master, red, " +
			"green, blue: 2 to 8 points of x and y from 0 to 1) each replace what they name and a neutral one removes it.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "brightness": 0.1, "temperature": 0.3}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState},
		Scope:   ScopeEdit,
		Phase:   Alpha,
	}, applyClipSetColor))
}

type cropArgs struct {
	Left   *float64 `json:"left,omitempty"   min:"0" max:"1"`
	Top    *float64 `json:"top,omitempty"    min:"0" max:"1"`
	Right  *float64 `json:"right,omitempty"  min:"0" max:"1"`
	Bottom *float64 `json:"bottom,omitempty" min:"0" max:"1"`
}

type clipFramingArgs struct {
	ItemID string    `json:"item_id"        desc:"A clip."                                                            pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Fit    string    `json:"fit,omitempty"  desc:"cover (default), contain or none."                                                                                         enum:"cover,contain,none"`
	Crop   *cropArgs `json:"crop,omitempty" desc:"Sides to crop, fractions 0 to 1; sides not given keep their value."`
}

// clipItem returns the item when it is a clip, else the E_TYPE_MISMATCH of the commands
// that apply to clips only.
func (c *Context) clipItem(id, what string) (timeline.Item, *Error) {
	if err := idForm("args.item_id", timeline.PrefixItem, id); err != nil {
		return timeline.Item{}, err
	}
	it, err := c.item(id)
	if err != nil {
		return it, err
	}
	if it.Type != timeline.ItemClip {
		return it, Errorf(CodeTypeMismatch, "Item %s is %s item; %s applies to clips.", id, article(it.Type), what).
			WithHint("Use item.set_transform to size other items.").WithPath("args.item_id")
	}
	return it, nil
}

// visualItem returns the item when it is a clip, text, caption or shape item, else the
// E_TYPE_MISMATCH of the commands that apply to every visual item (blur, filter).
func (c *Context) visualItem(id, what string) (timeline.Item, *Error) {
	if err := idForm("args.item_id", timeline.PrefixItem, id); err != nil {
		return timeline.Item{}, err
	}
	it, err := c.item(id)
	if err != nil {
		return it, err
	}
	if !isVisual(it.Type) {
		return it, Errorf(
			CodeTypeMismatch,
			"Item %s is %s item; %s applies to visual items.",
			id,
			article(it.Type),
			what,
		).
			WithHint("Use item.set_audio for the sound of an audio item.").
			WithPath("args.item_id")
	}
	return it, nil
}

func article(t timeline.ItemType) string {
	if t == timeline.ItemAudio {
		return "an audio"
	}
	return "a " + string(t)
}

type cropSide struct {
	given *float64
	dst   *float64
}

func applyClipSetFraming(c *Context, a *clipFramingArgs) error {
	if err := atLeastOne([]string{"fit", "crop"}, a.Fit != "", a.Crop != nil); err != nil {
		return err
	}
	it, err := c.clipItem(a.ItemID, "framing")
	if err != nil {
		return err
	}
	if a.Fit != "" {
		it.Fit = a.Fit
	}
	if a.Crop != nil {
		if err := cropAnimated(&it, a.Crop); err != nil {
			return err
		}
		merged := timeline.Crop{}
		if it.Crop != nil {
			merged = *it.Crop
		}
		for _, s := range []cropSide{
			{a.Crop.Left, &merged.Left},
			{a.Crop.Top, &merged.Top},
			{a.Crop.Right, &merged.Right},
			{a.Crop.Bottom, &merged.Bottom},
		} {
			if s.given != nil {
				*s.dst = round3(*s.given)
			}
		}
		if err := checkCropSums(&merged); err != nil {
			return err
		}
		if merged.Left == 0 && merged.Top == 0 && merged.Right == 0 && merged.Bottom == 0 {
			it.Crop = nil
		} else {
			it.Crop = &merged
		}
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("fit", it.Fit)
	if it.Crop == nil {
		c.SetEffective("crop", map[string]any{})
	} else {
		c.SetEffective("crop", cropMap(it.Crop))
	}
	return nil
}

// cropAnimated refuses a crop side that has keyframes, which define its value.
func cropAnimated(it *timeline.Item, cr *cropArgs) *Error {
	var animated []string
	for _, p := range []struct {
		name  string
		given *float64
	}{
		{"crop_left", cr.Left},
		{"crop_top", cr.Top},
		{"crop_right", cr.Right},
		{"crop_bottom", cr.Bottom},
	} {
		if p.given != nil && len(it.Keyframes[p.name]) > 0 {
			animated = append(animated, p.name)
		}
	}
	if len(animated) == 0 {
		return nil
	}
	first := animated[0]
	return Errorf(CodeInvalidState, "Property %s of %s has %d keyframes, which define its value.",
		first, it.ID, len(it.Keyframes[first])).
		WithHint("Use keyframe.set to change a keyframe, or keyframe.clear to remove them first.").
		WithDetail("properties", animated).WithPath("args.crop")
}

func checkCropSums(m *timeline.Crop) *Error {
	for _, p := range []struct {
		a, b   float64
		na, nb string
	}{{m.Left, m.Right, "left", "right"}, {m.Top, m.Bottom, "top", "bottom"}} {
		if sum := p.a + p.b; sum >= 1 {
			return invalidArg("args.crop", "args.crop: %s %s plus %s %s is %s; it must be below 1.",
				p.na, trimFloat(p.a), p.nb, trimFloat(p.b), trimFloat(round3(sum))).
				WithHint("Reduce %s or %s so that together they stay below 1.", p.na, p.nb)
		}
	}
	return nil
}

func cropMap(cr *timeline.Crop) map[string]any {
	m := map[string]any{}
	for name, v := range map[string]float64{"left": cr.Left, "top": cr.Top, "right": cr.Right, "bottom": cr.Bottom} {
		if v > 0 {
			m[name] = v
		}
	}
	return m
}

type clipColorArgs struct {
	ItemID       string        `json:"item_id"                 desc:"A clip."                                                                                      pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Brightness   *float64      `json:"brightness,omitempty"    desc:"-1 to 1; default 0."                                                                                                                                 min:"-1"   max:"1"`
	Contrast     *float64      `json:"contrast,omitempty"      desc:"0 to 2; default 1."                                                                                                                                  min:"0"    max:"2"`
	Saturation   *float64      `json:"saturation,omitempty"    desc:"0 to 2; default 1."                                                                                                                                  min:"0"    max:"2"`
	Temperature  *float64      `json:"temperature,omitempty"   desc:"-1 (cooler) to 1 (warmer); default 0."                                                                                                               min:"-1"   max:"1"`
	Tint         *float64      `json:"tint,omitempty"          desc:"-1 (greener) to 1 (more magenta); default 0."                                                                                                        min:"-1"   max:"1"`
	Hue          *float64      `json:"hue,omitempty"           desc:"-180 to 180 degrees; default 0."                                                                                                                     min:"-180" max:"180"`
	LutAsset     string        `json:"lut_asset,omitempty"     desc:"Id of a lut asset to grade the clip with."                                                    pattern:"^ast_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	LutIntensity *float64      `json:"lut_intensity,omitempty" desc:"0 to 1; default 1; 0 removes the lookup table."                                                                                                      min:"0"    max:"1"`
	Hsl          []hslBandArgs `json:"hsl,omitempty"           desc:"Bands of colour ranges, up to 8; a given range replaces its band, a neutral band removes it."                                                                             maxitems:"8"`
	Wheels       *wheelsArgs   `json:"wheels,omitempty"        desc:"Colour wheels of shadows, midtones and highlights."`
	Curves       *curvesArgs   `json:"curves,omitempty"        desc:"Tone curves: master, red, green, blue."`
}

func applyClipSetColor(c *Context, a *clipColorArgs) error {
	if err := atLeastOne(
		[]string{
			"brightness",
			"contrast",
			"saturation",
			"temperature",
			"tint",
			"hue",
			"lut_asset",
			"lut_intensity",
			"hsl",
			"wheels",
			"curves",
		},
		a.Brightness != nil,
		a.Contrast != nil,
		a.Saturation != nil,
		a.Temperature != nil,
		a.Tint != nil,
		a.Hue != nil,
		a.LutAsset != "",
		a.LutIntensity != nil,
		len(a.Hsl) > 0,
		a.Wheels != nil,
		a.Curves != nil,
	); err != nil {
		return err
	}
	if err := idForm("args.lut_asset", timeline.PrefixAsset, a.LutAsset); err != nil {
		return err
	}
	it, err := c.clipItem(a.ItemID, "colour")
	if err != nil {
		return err
	}
	if err := colourNotAnimated(&it, a); err != nil {
		return err
	}
	if a.Brightness != nil {
		it.Brightness = round3(*a.Brightness)
	}
	if a.Contrast != nil {
		it.Contrast = round3(*a.Contrast)
	}
	if a.Saturation != nil {
		it.Saturation = round3(*a.Saturation)
	}
	if a.Temperature != nil {
		it.Temperature = round3(*a.Temperature)
	}
	if a.Tint != nil {
		it.Tint = round3(*a.Tint)
	}
	if a.Hue != nil {
		it.Hue = round3(*a.Hue)
	}
	if err := setLut(c, &it, a); err != nil {
		return err
	}
	if err := setGrade(&it, a); err != nil {
		return err
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("brightness", it.Brightness)
	c.SetEffective("contrast", it.Contrast)
	c.SetEffective("saturation", it.Saturation)
	c.SetEffective("temperature", it.Temperature)
	c.SetEffective("tint", it.Tint)
	c.SetEffective("hue", it.Hue)
	gradeEffective(c, &it, a)
	if it.Lut != nil {
		c.SetEffective("lut", map[string]any{"asset_id": it.Lut.Asset, "intensity": it.Lut.Intensity})
	} else {
		c.SetEffective("lut", nil)
	}
	return nil
}

// colourNotAnimated refuses to set the static value of a colour property that has keyframes: they
// define its value, as for item.set_transform.
func colourNotAnimated(it *timeline.Item, a *clipColorArgs) *Error {
	var animated []string
	for _, p := range []struct {
		name  string
		given *float64
	}{
		{"brightness", a.Brightness},
		{"contrast", a.Contrast},
		{"saturation", a.Saturation},
		{"temperature", a.Temperature},
		{"tint", a.Tint},
		{"hue", a.Hue},
	} {
		if p.given != nil && len(it.Keyframes[p.name]) > 0 {
			animated = append(animated, p.name)
		}
	}
	if len(animated) == 0 {
		return nil
	}
	first := animated[0]
	return Errorf(CodeInvalidState, "Property %s of %s has %d keyframes, which define its value.",
		first, it.ID, len(it.Keyframes[first])).
		WithHint("Use keyframe.set to change a keyframe, or keyframe.clear to remove them first.").
		WithDetail("properties", animated).WithPath("args." + first)
}

// setLut applies lut_asset and lut_intensity: the asset must be a lut asset of the project, the
// intensity defaults to 1 for a new table and keeps the current value for the same one, and an
// intensity of 0 removes the table (SDD-001 section 3.5).
func setLut(c *Context, it *timeline.Item, a *clipColorArgs) *Error {
	if a.LutAsset == "" && a.LutIntensity == nil {
		return nil
	}
	if a.LutAsset == "" && it.Lut == nil {
		return invalidArg("args.lut_intensity", "args.lut_intensity is given but the clip has no lookup table.").
			WithHint("Send lut_asset with the id of a lut asset.")
	}
	next := timeline.Lut{Intensity: 1}
	if it.Lut != nil {
		next = *it.Lut
	}
	if a.LutAsset != "" {
		asset, err := c.asset(a.LutAsset, "args.lut_asset")
		if err != nil {
			return err
		}
		if asset.Kind != timeline.AssetLUT {
			return Errorf(
				CodeTypeMismatch,
				"Asset %s is %s %s asset; lut_asset needs a lut asset.",
				a.LutAsset,
				indefinite(string(asset.Kind)),
				asset.Kind,
			).
				WithHint("Import a .cube file and use the id of that asset.").
				WithPath("args.lut_asset")
		}
		if next.Asset != a.LutAsset {
			next.Intensity = 1
		}
		next.Asset = a.LutAsset
	}
	if a.LutIntensity != nil {
		next.Intensity = round3(*a.LutIntensity)
	}
	if next.Intensity == 0 {
		it.Lut = nil
		return nil
	}
	it.Lut = &next
	return nil
}

func indefinite(word string) string {
	if word != "" && strings.ContainsRune("aeiou", rune(word[0])) {
		return "an"
	}
	return "a"
}

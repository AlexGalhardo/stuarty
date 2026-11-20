package image

import (
	"fmt"
	"slices"
	"strings"
)

// The look of a layer (SDD-021 section 3.1, epic 29): adjustments, an ordered list of filters, a mask and a
// blend mode. Every value is a whole number and the stored form never holds a neutral value.

// The kinds of filter. A layer has at most one filter of each kind.
const (
	FilterGrayscale = "grayscale"
	FilterSepia     = "sepia"
	FilterInvert    = "invert"
	FilterWarm      = "warm"
	FilterCool      = "cool"
	FilterBlur      = "blur"
	FilterSharpen   = "sharpen"
	FilterVignette  = "vignette"
	FilterGrain     = "grain"
	FilterShadow    = "shadow"
	FilterGlow      = "glow"
	FilterStroke    = "stroke"
)

// The kinds of mask.
const (
	MaskRect        = "rect"
	MaskRoundedRect = "rounded_rect"
	MaskEllipse     = "ellipse"
	MaskLayer       = "layer"
)

// Limits of the look (SDD-021 section 12).
const (
	MaxFilters     = 8
	MaxCurvePoints = 8
	MaxMaskRadius  = 16384
	MaxMaskFeather = 200
)

// The neutral levels.
const (
	LevelsBlack = 0
	LevelsWhite = 255
	LevelsGamma = 100
)

// BlendModes are the blend modes of a layer, in the order of SDD-021 section 3.1.
var BlendModes = []string{
	"normal", "multiply", "screen", "overlay", "soft_light", "hard_light", "darken", "lighten", "color_dodge",
	"color_burn", "difference",
}

// FilterKinds are the kinds of filter, in the order of SDD-021 section 3.1.
var FilterKinds = []string{
	FilterGrayscale, FilterSepia, FilterInvert, FilterWarm, FilterCool, FilterBlur, FilterSharpen, FilterVignette,
	FilterGrain, FilterShadow, FilterGlow, FilterStroke,
}

// CurvePoint is a point of a tone curve, 0 to 255 on both axes.
type CurvePoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Levels maps the black point and the white point to 0 and 255; Gamma is in hundredths.
type Levels struct {
	Black int `json:"black"`
	White int `json:"white"`
	Gamma int `json:"gamma"`
}

// LUTRef is the colour lookup table of a layer (epic 49): the content hash of a lut asset and how much of it is
// mixed in, a whole percent.
type LUTRef struct {
	Asset     string `json:"asset"`
	Intensity int    `json:"intensity"`
}

// Limits of the lookup table of a layer: its intensity in per cent.
const (
	MinLUTIntensity = 1
	MaxLUTIntensity = 100
)

// Adjust holds the adjustments of a layer that differ from neutral.
type Adjust struct {
	Brightness  int          `json:"brightness,omitempty"`
	Contrast    int          `json:"contrast,omitempty"`
	Saturation  int          `json:"saturation,omitempty"`
	Temperature int          `json:"temperature,omitempty"`
	Tint        int          `json:"tint,omitempty"`
	Hue         int          `json:"hue,omitempty"`
	Exposure    int          `json:"exposure,omitempty"`
	Levels      *Levels      `json:"levels,omitempty"`
	Curve       []CurvePoint `json:"curve,omitempty"`
	LUT         *LUTRef      `json:"lut,omitempty"`
}

// Filter is one filter of a layer: its kind and every parameter of the kind, all filled in.
type Filter struct {
	Kind     string `json:"kind"`
	Amount   *int   `json:"amount,omitempty"`
	Radius   *int   `json:"radius,omitempty"`
	Softness *int   `json:"softness,omitempty"`
	Size     *int   `json:"size,omitempty"`
	Seed     *int   `json:"seed,omitempty"`
	Opacity  *int   `json:"opacity,omitempty"`
	DX       *int   `json:"dx,omitempty"`
	DY       *int   `json:"dy,omitempty"`
	Blur     *int   `json:"blur,omitempty"`
	Width    *int   `json:"width,omitempty"`
	Color    string `json:"color,omitempty"`
}

// Mask is the mask of a layer: a shape in thousandths of the box of the layer, or the alpha of another layer.
type Mask struct {
	Kind    string `json:"kind"`
	X       int    `json:"x,omitempty"`
	Y       int    `json:"y,omitempty"`
	W       int    `json:"w,omitempty"`
	H       int    `json:"h,omitempty"`
	Radius  int    `json:"radius,omitempty"`
	Feather int    `json:"feather,omitempty"`
	Invert  bool   `json:"invert,omitempty"`
	Source  string `json:"source,omitempty"`
}

// intParam is a numeric parameter of a filter kind.
type intParam struct {
	name         string
	lo, hi, init int
}

var amountPercent = intParam{"amount", 1, 100, 100}

// filterParams are the numeric parameters of each kind with their range and default (SDD-021 section 3.1).
var filterParams = map[string][]intParam{
	FilterGrayscale: {amountPercent},
	FilterSepia:     {amountPercent},
	FilterInvert:    {amountPercent},
	FilterWarm:      {amountPercent},
	FilterCool:      {amountPercent},
	FilterBlur:      {{"radius", 1, 100, 8}},
	FilterSharpen:   {{"amount", 1, 300, 100}, {"radius", 1, 20, 2}},
	FilterVignette:  {{"amount", 1, 100, 50}, {"softness", 0, 100, 50}},
	FilterGrain:     {{"amount", 1, 100, 30}, {"size", 1, 8, 1}, {"seed", 0, 65535, 0}},
	FilterShadow:    {{"opacity", 0, 100, 50}, {"dx", -200, 200, 4}, {"dy", -200, 200, 4}, {"blur", 0, 100, 8}},
	FilterGlow:      {{"radius", 1, 100, 12}, {"amount", 1, 400, 100}},
	FilterStroke:    {{"width", 1, 100, 4}},
}

// filterColours is the default colour of the kinds that have one.
var filterColours = map[string]string{FilterShadow: "#000000", FilterGlow: "#FFFFFF", FilterStroke: "#000000"}

// KnownFilter reports whether kind is a kind of filter.
func KnownFilter(kind string) bool {
	_, ok := filterParams[kind]

	return ok
}

// param is the address of the numeric member of a filter by name.
func (f *Filter) param(name string) **int {
	switch name {
	case "amount":
		return &f.Amount
	case "radius":
		return &f.Radius
	case "softness":
		return &f.Softness
	case "size":
		return &f.Size
	case "seed":
		return &f.Seed
	case "opacity":
		return &f.Opacity
	case "dx":
		return &f.DX
	case "dy":
		return &f.DY
	case "blur":
		return &f.Blur
	case "width":
		return &f.Width
	}

	return nil
}

// Value is the numeric parameter of a filter by name, 0 when it has none.
func (f *Filter) Value(name string) int {
	if p := f.param(name); p != nil && *p != nil {
		return **p
	}

	return 0
}

// NewFilter is a filter of a kind with the defaults of the kind.
func NewFilter(kind string) Filter {
	f := Filter{Kind: kind, Color: filterColours[kind]}
	for _, p := range filterParams[kind] {
		*f.param(p.name) = new(p.init)
	}

	return f
}

func cloneInt(p *int) *int {
	if p == nil {
		return nil
	}

	v := *p

	return &v
}

func cloneFilter(f Filter) Filter {
	out := f
	for _, n := range []string{"amount", "radius", "softness", "size", "seed", "opacity", "dx", "dy", "blur", "width"} {
		*out.param(n) = cloneInt(*f.param(n))
	}

	return out
}

func cloneLook(out, l *Layer) {
	if l.Adjust != nil {
		a := *l.Adjust
		if a.Levels != nil {
			lv := *a.Levels
			a.Levels = &lv
		}

		a.Curve = slices.Clone(a.Curve)

		if a.LUT != nil {
			lut := *a.LUT
			a.LUT = &lut
		}

		out.Adjust = &a
	}

	if l.Filters != nil {
		out.Filters = make([]Filter, len(l.Filters))
		for i := range l.Filters {
			out.Filters[i] = cloneFilter(l.Filters[i])
		}
	}

	if l.Mask != nil {
		m := *l.Mask
		out.Mask = &m
	}
}

// neutral reports whether an adjustment changes nothing.
func (a *Adjust) neutral() bool {
	return a.Brightness == 0 && a.Contrast == 0 && a.Saturation == 0 && a.Temperature == 0 && a.Tint == 0 &&
		a.Hue == 0 && a.Exposure == 0 && a.Levels == nil && len(a.Curve) == 0 && a.LUT == nil
}

// normalize drops what is neutral, so that the stored form is canonical.
func normalizeAdjust(l *Layer) {
	a := l.Adjust
	if a == nil {
		return
	}

	if a.Levels != nil && *a.Levels == (Levels{LevelsBlack, LevelsWhite, LevelsGamma}) {
		a.Levels = nil
	}

	if len(a.Curve) == 0 {
		a.Curve = nil
	}

	if a.neutral() {
		l.Adjust = nil
	}
}

// ValidateAdjust lists what is wrong with an adjustment, as messages with a path under prefix.
func validateAdjust(a *Adjust, prefix string, add func(path, format string, a ...any)) {
	rng := func(name string, v, lo, hi int) {
		if v < lo || v > hi {
			add(prefix+"."+name, "%s is %d; it must be %d to %d", name, v, lo, hi)
		}
	}

	rng("brightness", a.Brightness, -100, 100)
	rng("contrast", a.Contrast, -100, 100)
	rng("saturation", a.Saturation, -100, 100)
	rng("temperature", a.Temperature, -100, 100)
	rng("tint", a.Tint, -100, 100)
	rng("hue", a.Hue, -180, 180)
	rng("exposure", a.Exposure, -300, 300)

	if lv := a.Levels; lv != nil {
		rng("levels.black", lv.Black, 0, 254)
		rng("levels.white", lv.White, 1, 255)
		rng("levels.gamma", lv.Gamma, 10, 1000)

		if lv.White <= lv.Black {
			add(prefix+".levels", "the white point %d is not above the black point %d", lv.White, lv.Black)
		}
	}

	if len(a.Curve) > MaxCurvePoints {
		add(prefix+".curve", "the curve has %d points; the limit is %d", len(a.Curve), MaxCurvePoints)
	}

	for i, p := range a.Curve {
		rng(fmt.Sprintf("curve[%d].x", i), p.X, 0, 255)
		rng(fmt.Sprintf("curve[%d].y", i), p.Y, 0, 255)

		if i > 0 && p.X <= a.Curve[i-1].X {
			add(prefix+fmt.Sprintf(".curve[%d].x", i), "the points of the curve must have x strictly increasing")
		}
	}

	if a.LUT != nil {
		if !hashPattern.MatchString(a.LUT.Asset) {
			add(prefix+".lut.asset", "the lookup table %q is not a content hash", a.LUT.Asset)
		}

		rng("lut.intensity", a.LUT.Intensity, MinLUTIntensity, MaxLUTIntensity)
	}
}

// validateFilter lists what is wrong with a filter.
func validateFilter(f *Filter, prefix string, add func(path, format string, a ...any)) {
	params, ok := filterParams[f.Kind]
	if !ok {
		add(prefix+".kind", "the filter kind %q is not known", f.Kind)

		return
	}

	known := map[string]bool{}

	for _, p := range params {
		known[p.name] = true

		v := f.param(p.name)
		switch {
		case *v == nil:
			add(prefix+"."+p.name, "the %s filter has no %s", f.Kind, p.name)
		case **v < p.lo || **v > p.hi:
			add(prefix+"."+p.name, "%s is %d; it must be %d to %d", p.name, **v, p.lo, p.hi)
		}
	}

	for _, n := range []string{"amount", "radius", "softness", "size", "seed", "opacity", "dx", "dy", "blur", "width"} {
		if !known[n] && *f.param(n) != nil {
			add(prefix+"."+n, "the %s filter has no %s", f.Kind, n)
		}
	}

	if _, has := filterColours[f.Kind]; has {
		if !ValidColour(f.Color) {
			add(prefix+".color", "the colour %q is not a colour", f.Color)
		}
	} else if f.Color != "" {
		add(prefix+".color", "the %s filter has no colour", f.Kind)
	}
}

// validateMask lists what is wrong with a mask on its own (the source is checked by the document).
func validateMask(m *Mask, prefix string, add func(path, format string, a ...any)) {
	switch m.Kind {
	case MaskRect, MaskRoundedRect, MaskEllipse:
		inside := func(name string, v, lo int) {
			if v < lo || v > 1000 {
				add(prefix+"."+name, "%s is %d; it must be %d to 1000", name, v, lo)
			}
		}

		inside("x", m.X, 0)
		inside("y", m.Y, 0)
		inside("w", m.W, 1)
		inside("h", m.H, 1)

		if m.X+m.W > 1000 || m.Y+m.H > 1000 {
			add(prefix, "the rectangle of the mask leaves the box of the layer")
		}

		if m.Feather < 0 || m.Feather > MaxMaskFeather {
			add(prefix+".feather", "feather is %d; it must be 0 to %d", m.Feather, MaxMaskFeather)
		}

		if m.Radius < 0 || m.Radius > MaxMaskRadius || (m.Radius != 0 && m.Kind != MaskRoundedRect) {
			add(prefix+".radius", "radius is %d; it is 0 to %d and only for rounded_rect", m.Radius, MaxMaskRadius)
		}

		if m.Source != "" {
			add(prefix+".source", "only a mask of kind layer has a source")
		}
	case MaskLayer:
		if m.X != 0 || m.Y != 0 || m.W != 0 || m.H != 0 || m.Radius != 0 || m.Feather != 0 {
			add(prefix, "a mask of kind layer has only source and invert")
		}

		if m.Source == "" {
			add(prefix+".source", "a mask of kind layer needs a source")
		}
	default:
		add(prefix+".kind", "the mask kind %q is not rect, rounded_rect, ellipse or layer", m.Kind)
	}
}

func (v *validator) look(l *Layer, p string) {
	add := func(path, format string, a ...any) { v.add("look", path, l.ID, format, a...) }

	if l.Kind == KindGroup {
		if l.Adjust != nil || len(l.Filters) > 0 || l.Mask != nil || l.Blend != "" {
			v.add("look", p, l.ID, "a group has no adjustments, filters, mask or blend mode")
		}

		return
	}

	if l.Adjust != nil {
		validateAdjust(l.Adjust, p+".adjust", add)

		if l.Adjust.neutral() {
			add(p+".adjust", "a stored adjustment holds no neutral value")
		}
	}

	if len(l.Filters) > MaxFilters {
		add(p+".filters", "the layer has %d filters; the limit is %d", len(l.Filters), MaxFilters)
	}

	seen := map[string]bool{}

	for i := range l.Filters {
		fp := fmt.Sprintf("%s.filters[%d]", p, i)
		validateFilter(&l.Filters[i], fp, add)

		if seen[l.Filters[i].Kind] {
			add(fp+".kind", "the layer has two %s filters", l.Filters[i].Kind)
		}

		seen[l.Filters[i].Kind] = true
	}

	if l.Mask != nil {
		validateMask(l.Mask, p+".mask", add)
	}

	if l.Blend != "" && (l.Blend == "normal" || !slices.Contains(BlendModes, l.Blend)) {
		add(
			p+".blend",
			"the blend mode %q is not stored: it is one of the modes of SDD-021 section 3.1 but normal",
			l.Blend,
		)
	}
}

// maskSources checks every mask of kind layer: the source exists, is not the layer itself, is not a group and
// has no mask of kind layer of its own.
func (v *validator) maskSources() {
	Walk(v.d, func(l, _ *Layer, _ int) {
		if l.Mask == nil || l.Mask.Kind != MaskLayer || l.Mask.Source == "" {
			return
		}

		src, ok := Find(v.d, l.Mask.Source)

		switch {
		case !ok:
			v.add("look", "mask.source", l.ID, "the mask source %s is not a layer of the document", l.Mask.Source)
		case src.ID == l.ID:
			v.add("look", "mask.source", l.ID, "a layer cannot be the source of its own mask")
		case src.Kind == KindGroup:
			v.add("look", "mask.source", l.ID, "the mask source %s is a group", src.ID)
		case src.Mask != nil && src.Mask.Kind == MaskLayer:
			v.add("look", "mask.source", l.ID, "the mask source %s has a mask from a layer itself", src.ID)
		}
	})
}

func collectIDs(l *Layer, into map[string]bool) {
	into[l.ID] = true

	for i := range l.Children {
		collectIDs(&l.Children[i], into)
	}
}

// dropMasksFrom removes the mask of every layer whose source is one of the removed layers, so that no mask
// names a layer the document no longer has.
func dropMasksFrom(d *Document, gone map[string]bool) {
	Walk(d, func(l, _ *Layer, _ int) {
		if l.Mask != nil && l.Mask.Kind == MaskLayer && gone[l.Mask.Source] {
			l.Mask = nil
		}
	})
}

// shortHash is a content hash as the text view names it: the algorithm and the first 12 digits.
func shortHash(h string) string {
	const keep = len("sha256:") + 12

	if len(h) > keep {
		return h[:keep]
	}

	return h
}

// describeLook is the part of the text line of a layer that names its look: " blend multiply adjust
// brightness=10 filters blur,shadow mask ellipse". It is empty for a layer with none.
func describeLook(l *Layer) string {
	var b strings.Builder

	if l.Blend != "" {
		b.WriteString(" blend " + l.Blend)
	}

	if a := l.Adjust; a != nil {
		var keys []string

		add := func(name string, v int) {
			if v != 0 {
				keys = append(keys, fmt.Sprintf("%s=%d", name, v))
			}
		}

		add("brightness", a.Brightness)
		add("contrast", a.Contrast)
		add("saturation", a.Saturation)
		add("temperature", a.Temperature)
		add("tint", a.Tint)
		add("hue", a.Hue)
		add("exposure", a.Exposure)

		if a.Levels != nil {
			keys = append(keys, fmt.Sprintf("levels=%d/%d/%d", a.Levels.Black, a.Levels.White, a.Levels.Gamma))
		}

		if len(a.Curve) > 0 {
			keys = append(keys, fmt.Sprintf("curve=%dpoints", len(a.Curve)))
		}

		if a.LUT != nil {
			keys = append(keys, fmt.Sprintf("lut=%s@%d", shortHash(a.LUT.Asset), a.LUT.Intensity))
		}

		b.WriteString(" adjust " + strings.Join(keys, ","))
	}

	if len(l.Filters) > 0 {
		kinds := make([]string, len(l.Filters))
		for i := range l.Filters {
			kinds[i] = l.Filters[i].Kind
		}

		b.WriteString(" filters " + strings.Join(kinds, ","))
	}

	if m := l.Mask; m != nil {
		b.WriteString(" mask " + m.Kind)

		if m.Kind == MaskLayer {
			b.WriteString(" " + m.Source)
		}

		if m.Invert {
			b.WriteString(" inverted")
		}
	}

	return b.String()
}

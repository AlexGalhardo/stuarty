package image

import (
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
)

func init() {
	register(
		command.DefineIn(command.Spec{
			Name:    "layer.adjust",
			Summary: "Sets the brightness, contrast, saturation, temperature, tint, hue, exposure, levels or tone curve of a layer.",
			Description: "Each key given replaces that adjustment and the others stay; a value equal to its neutral one removes it. " +
				"brightness, contrast, saturation, temperature and tint are -100 to 100, hue -180 to 180 degrees, exposure " +
				"-300 to 300 hundredths of a stop. levels takes black (0 to 254), white (1 to 255, above black) and gamma " +
				"(10 to 1000 hundredths, 100 is neutral); the keys left out keep their value. curve is up to 8 points " +
				"{x, y} of 0 to 255 with x increasing, and an empty list removes it. reset true removes every adjustment. " +
				"Not valid for a group.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "brightness": 10, "contrast": 20, "levels": {"gamma": 120}}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyAdjust),
		command.DefineIn(command.Spec{
			Name:    "layer.filter_add",
			Summary: "Adds a filter to a layer: a colour filter, blur, sharpen, vignette, grain, shadow, glow or stroke.",
			Description: "kind is grayscale, sepia, invert, warm, cool, blur, sharpen, vignette, grain, shadow, glow or stroke; " +
				"a layer has at most one filter of each kind and at most 8. The parameters left out take the defaults of the " +
				"kind. Filters act in list order; index is the position of the new one (0 is the first to act, absent means " +
				"last). Not valid for a group.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "kind": "blur", "radius": 12}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
				command.CodeLimit,
			},
			Scope: command.ScopeEdit,
		}, applyFilterAdd),
		command.DefineIn(command.Spec{
			Name:        "layer.filter_set",
			Summary:     "Changes the parameters of a filter of a layer.",
			Description: "kind names the filter; the parameters given replace the old ones and the others stay.",
			Example:     `{"layer_id": "lyr_a1b2c3d4e5", "kind": "blur", "radius": 20}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyFilterSet),
		command.DefineIn(command.Spec{
			Name:        "layer.filter_remove",
			Summary:     "Removes a filter of a layer.",
			Description: "kind names the filter.",
			Example:     `{"layer_id": "lyr_a1b2c3d4e5", "kind": "blur"}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyFilterRemove),
		command.DefineIn(command.Spec{
			Name:        "layer.filter_move",
			Summary:     "Changes the place of a filter in the list of a layer.",
			Description: "index is the final position of the filter among the filters of the layer; the first acts first.",
			Example:     `{"layer_id": "lyr_a1b2c3d4e5", "kind": "shadow", "index": 0}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyFilterMove),
		command.DefineIn(command.Spec{
			Name:    "layer.mask_set",
			Summary: "Sets the mask of a layer: a rectangle, a rounded rectangle or an ellipse, or the alpha of another layer.",
			Description: "A shape mask is in thousandths of the box of the layer: x, y, w and h (default the whole box), radius " +
				"(rounded_rect only, pixels), feather (the soft edge in pixels, 0 to 200) and invert. kind layer takes source, " +
				"the id of another layer that is not a group and has no mask from a layer of its own, and invert: the layer " +
				"shows only where the source has pixels. The old mask is replaced. Not valid for a group.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "kind": "ellipse", "x": 100, "y": 100, "w": 800, "h": 800, "feather": 12}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyMaskSet),
		command.DefineIn(command.Spec{
			Name:        "layer.mask_clear",
			Summary:     "Removes the mask of a layer.",
			Description: "A layer with no mask is left as it is.",
			Example:     `{"layer_id": "lyr_a1b2c3d4e5"}`,
			Errors: []string{
				command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyMaskClear),
		command.DefineIn(command.Spec{
			Name:    "layer.blend",
			Summary: "Sets the blend mode of a layer.",
			Description: "mode is normal, multiply, screen, overlay, soft_light, hard_light, darken, lighten, color_dodge, " +
				"color_burn or difference; normal removes the mode. Not valid for a group.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "mode": "multiply"}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyBlend),
	)
}

// lookTarget finds the layer a look command changes: it must exist, be unlocked and not be a group.
func lookTarget(d *Document, id, op string) (*Layer, *command.Error) {
	l, err := target(d, id, "args.layer_id")
	if err != nil {
		return nil, err
	}

	if l.Kind == KindGroup {
		return nil, command.Errorf(command.CodeTypeMismatch, "Layer %s is a group; %s changes the layers inside it.", l.ID, op).
			WithHint("Apply it to each child, or to a layer that is not a group.").
			WithPath("args.layer_id")
	}

	return l, nil
}

// violations turns what the validators found into one E_VALIDATION error, or nil.
func violations(collect func(add func(path, format string, a ...any))) *command.Error {
	var first *command.Error

	collect(func(path, format string, a ...any) {
		if first == nil {
			first = invalid(path, format, a...).WithHint("Keep each value inside the range the message gives.")
		}
	})

	return first
}

type levelsArgs struct {
	Black *int `json:"black,omitempty" desc:"0 to 254."                               min:"0"  max:"254"`
	White *int `json:"white,omitempty" desc:"1 to 255, above black."                  min:"1"  max:"255"`
	Gamma *int `json:"gamma,omitempty" desc:"Hundredths, 10 to 1000; 100 is neutral." min:"10" max:"1000"`
}

type adjustArgs struct {
	LayerID     string        `json:"layer_id"              pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Brightness  *int          `json:"brightness,omitempty"                                                         min:"-100" max:"100"`
	Contrast    *int          `json:"contrast,omitempty"                                                           min:"-100" max:"100"`
	Saturation  *int          `json:"saturation,omitempty"                                                         min:"-100" max:"100"`
	Temperature *int          `json:"temperature,omitempty"                                                        min:"-100" max:"100"`
	Tint        *int          `json:"tint,omitempty"                                                               min:"-100" max:"100"`
	Hue         *int          `json:"hue,omitempty"                                                                min:"-180" max:"180" desc:"Degrees."`
	Exposure    *int          `json:"exposure,omitempty"                                                           min:"-300" max:"300" desc:"Hundredths of a stop."`
	Levels      *levelsArgs   `json:"levels,omitempty"`
	Curve       *[]CurvePoint `json:"curve,omitempty"                                                                         max:"8"   desc:"Up to 8 points of 0 to 255, x increasing; an empty list removes the curve."`
	Reset       bool          `json:"reset,omitempty"                                                                                   desc:"Remove every adjustment; no other argument."`
}

func applyAdjust(c *command.Context, d *Document, a *adjustArgs) error {
	given := a.Brightness != nil || a.Contrast != nil || a.Saturation != nil || a.Temperature != nil ||
		a.Tint != nil || a.Hue != nil || a.Exposure != nil || a.Levels != nil || a.Curve != nil

	if a.Reset && given {
		return invalid("args.reset", "Give reset, or the adjustments, not both.").
			WithHint("Remove the adjustments, or remove reset.")
	}

	if !a.Reset {
		if err := atLeastOne([]string{
			"brightness",
			"contrast",
			"saturation",
			"temperature",
			"tint",
			"hue",
			"exposure",
			"levels",
			"curve",
			"reset",
		}, given); err != nil {
			return err
		}
	}

	l, err := lookTarget(d, a.LayerID, "layer.adjust")
	if err != nil {
		return err
	}

	next := Adjust{}
	if l.Adjust != nil && !a.Reset {
		next = *l.Adjust
		if next.Levels != nil {
			lv := *next.Levels
			next.Levels = &lv
		}
	}

	set := func(dst *int, v *int) {
		if v != nil {
			*dst = *v
		}
	}

	set(&next.Brightness, a.Brightness)
	set(&next.Contrast, a.Contrast)
	set(&next.Saturation, a.Saturation)
	set(&next.Temperature, a.Temperature)
	set(&next.Tint, a.Tint)
	set(&next.Hue, a.Hue)
	set(&next.Exposure, a.Exposure)

	if a.Levels != nil {
		lv := Levels{LevelsBlack, LevelsWhite, LevelsGamma}
		if next.Levels != nil {
			lv = *next.Levels
		}

		set(&lv.Black, a.Levels.Black)
		set(&lv.White, a.Levels.White)
		set(&lv.Gamma, a.Levels.Gamma)
		next.Levels = &lv
	}

	if a.Curve != nil {
		next.Curve = slices.Clone(*a.Curve)
	}

	if verr := violations(func(add func(path, format string, a ...any)) {
		validateAdjust(&next, "args", add)
	}); verr != nil {
		return verr.WithHint(
			"Keep each adjustment inside its range, the white point above the black point and the curve x increasing.",
		)
	}

	l.Adjust = &next
	normalizeAdjust(l)

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("adjust", adjustOrEmpty(l))

	return nil
}

func adjustOrEmpty(l *Layer) map[string]any {
	if l.Adjust == nil {
		return map[string]any{}
	}

	return object(l.Adjust)
}

// filterArgs are the arguments of layer.filter_add and layer.filter_set: the parameters of every kind, of which
// a kind accepts its own.
type filterArgs struct {
	LayerID  string `json:"layer_id"           pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Kind     string `json:"kind"                                                                      enum:"grayscale,sepia,invert,warm,cool,blur,sharpen,vignette,grain,shadow,glow,stroke"`
	Amount   *int   `json:"amount,omitempty"                                                                                                                                                 desc:"Per cent: a colour filter 1 to 100, sharpen 1 to 300, vignette and grain 1 to 100, glow 1 to 400." min:"1"    max:"400"`
	Radius   *int   `json:"radius,omitempty"                                                                                                                                                 desc:"Pixels: blur and glow 1 to 100, sharpen 1 to 20."                                                  min:"1"    max:"100"`
	Softness *int   `json:"softness,omitempty"                                                                                                                                               desc:"Vignette, 0 to 100."                                                                               min:"0"    max:"100"`
	Size     *int   `json:"size,omitempty"                                                                                                                                                   desc:"Grain, 1 to 8."                                                                                    min:"1"    max:"8"`
	Seed     *int   `json:"seed,omitempty"                                                                                                                                                   desc:"Grain, 0 to 65535."                                                                                min:"0"    max:"65535"`
	Opacity  *int   `json:"opacity,omitempty"                                                                                                                                                desc:"Shadow, per cent."                                                                                 min:"0"    max:"100"`
	DX       *int   `json:"dx,omitempty"                                                                                                                                                     desc:"Shadow offset in pixels."                                                                          min:"-200" max:"200"`
	DY       *int   `json:"dy,omitempty"                                                                                                                                                     desc:"Shadow offset in pixels."                                                                          min:"-200" max:"200"`
	Blur     *int   `json:"blur,omitempty"                                                                                                                                                   desc:"Shadow blur in pixels."                                                                            min:"0"    max:"100"`
	Width    *int   `json:"width,omitempty"                                                                                                                                                  desc:"Stroke width in pixels."                                                                           min:"1"    max:"100"`
	Color    string `json:"color,omitempty"                                                                                                                                                  desc:"Shadow, glow and stroke: #RRGGBB or #RRGGBBAA."`
}

// filterAddArgs are the arguments of layer.filter_add: those of layer.filter_set and the place (the argument
// decoder does not read embedded structs, so the fields are repeated).
type filterAddArgs struct {
	LayerID  string `json:"layer_id"           pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Kind     string `json:"kind"                                                                      enum:"grayscale,sepia,invert,warm,cool,blur,sharpen,vignette,grain,shadow,glow,stroke"`
	Amount   *int   `json:"amount,omitempty"                                                                                                                                                 desc:"Per cent: a colour filter 1 to 100, sharpen 1 to 300, vignette and grain 1 to 100, glow 1 to 400." min:"1"    max:"400"`
	Radius   *int   `json:"radius,omitempty"                                                                                                                                                 desc:"Pixels: blur and glow 1 to 100, sharpen 1 to 20."                                                  min:"1"    max:"100"`
	Softness *int   `json:"softness,omitempty"                                                                                                                                               desc:"Vignette, 0 to 100."                                                                               min:"0"    max:"100"`
	Size     *int   `json:"size,omitempty"                                                                                                                                                   desc:"Grain, 1 to 8."                                                                                    min:"1"    max:"8"`
	Seed     *int   `json:"seed,omitempty"                                                                                                                                                   desc:"Grain, 0 to 65535."                                                                                min:"0"    max:"65535"`
	Opacity  *int   `json:"opacity,omitempty"                                                                                                                                                desc:"Shadow, per cent."                                                                                 min:"0"    max:"100"`
	DX       *int   `json:"dx,omitempty"                                                                                                                                                     desc:"Shadow offset in pixels."                                                                          min:"-200" max:"200"`
	DY       *int   `json:"dy,omitempty"                                                                                                                                                     desc:"Shadow offset in pixels."                                                                          min:"-200" max:"200"`
	Blur     *int   `json:"blur,omitempty"                                                                                                                                                   desc:"Shadow blur in pixels."                                                                            min:"0"    max:"100"`
	Width    *int   `json:"width,omitempty"                                                                                                                                                  desc:"Stroke width in pixels."                                                                           min:"1"    max:"100"`
	Color    string `json:"color,omitempty"                                                                                                                                                  desc:"Shadow, glow and stroke: #RRGGBB or #RRGGBBAA."`
	Index    *int   `json:"index,omitempty"                                                                                                                                                  desc:"Position among the filters; absent means last."                                                    min:"0"    max:"8"`
}

func (a *filterAddArgs) params() filterArgs {
	return filterArgs{
		LayerID: a.LayerID, Kind: a.Kind, Amount: a.Amount, Radius: a.Radius, Softness: a.Softness, Size: a.Size,
		Seed: a.Seed, Opacity: a.Opacity, DX: a.DX, DY: a.DY, Blur: a.Blur, Width: a.Width, Color: a.Color,
	}
}

// merge puts the parameters given in a on f, refusing one the kind does not have.
func (a *filterArgs) merge(f *Filter) *command.Error {
	given := map[string]*int{
		"amount": a.Amount, "radius": a.Radius, "softness": a.Softness, "size": a.Size, "seed": a.Seed,
		"opacity": a.Opacity, "dx": a.DX, "dy": a.DY, "blur": a.Blur, "width": a.Width,
	}

	own := map[string]bool{}
	for _, p := range filterParams[f.Kind] {
		own[p.name] = true
	}

	for _, n := range []string{"amount", "radius", "softness", "size", "seed", "opacity", "dx", "dy", "blur", "width"} {
		v := given[n]
		if v == nil {
			continue
		}

		if !own[n] {
			return invalid("args."+n, "The %s filter has no %s.", f.Kind, n).
				WithHint("The %s filter takes: %s.", f.Kind, filterParamNames(f.Kind))
		}

		*f.param(n) = new(*v)
	}

	if a.Color != "" {
		if _, has := filterColours[f.Kind]; !has {
			return invalid("args.color", "The %s filter has no colour.", f.Kind).
				WithHint("The %s filter takes: %s.", f.Kind, filterParamNames(f.Kind))
		}

		col, err := colour("args.color", a.Color)
		if err != nil {
			return err
		}

		f.Color = col
	}

	return violations(func(add func(path, format string, a ...any)) {
		validateFilter(f, "args", add)
	})
}

func filterParamNames(kind string) string {
	names := make([]string, 0, 4)
	for _, p := range filterParams[kind] {
		names = append(names, p.name)
	}

	if _, has := filterColours[kind]; has {
		names = append(names, "color")
	}

	return strings.Join(names, ", ")
}

func filterIndex(l *Layer, kind string) int {
	return slices.IndexFunc(l.Filters, func(f Filter) bool { return f.Kind == kind })
}

func filterNotFound(l *Layer, kind string) *command.Error {
	return command.Errorf(command.CodeNotFound, "Layer %s has no %s filter.", l.ID, kind).
		WithHint("Add it with layer.filter_add.").
		WithDetail("kind", "filter").WithDetail("id", kind).WithPath("args.kind")
}

func applyFilterAdd(c *command.Context, d *Document, a *filterAddArgs) error {
	l, err := lookTarget(d, a.LayerID, "layer.filter_add")
	if err != nil {
		return err
	}

	if filterIndex(l, a.Kind) >= 0 {
		return command.Errorf(command.CodeInvalidState, "Layer %s already has a %s filter.", l.ID, a.Kind).
			WithHint("Change it with layer.filter_set.").WithPath("args.kind")
	}

	if len(l.Filters) >= MaxFilters {
		return command.Errorf(command.CodeLimit, "Layer %s already has %d filters; the limit is %d.", l.ID, len(l.Filters), MaxFilters).
			WithHint("Remove a filter you no longer need.").
			WithDetail("limit", "filters_per_layer").
			WithDetail("value", len(l.Filters)).
			WithDetail("ceiling", MaxFilters)
	}

	if !KnownFilter(a.Kind) {
		return invalid("args.kind", "The filter kind %q is not known.", a.Kind)
	}

	f, p := NewFilter(a.Kind), a.params()
	if err := p.merge(&f); err != nil {
		return err
	}

	at := len(l.Filters)
	if a.Index != nil {
		if *a.Index > len(l.Filters) {
			return invalid("args.index", "index is %d; the layer has %d filters.", *a.Index, len(l.Filters)).
				WithHint("Use 0 to %d.", len(l.Filters))
		}

		at = *a.Index
	}

	l.Filters = slices.Insert(l.Filters, at, f)

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("kind", a.Kind)
	c.SetEffective("filter", object(f))
	c.SetEffective("index", at)

	return nil
}

func applyFilterSet(c *command.Context, d *Document, a *filterArgs) error {
	l, err := lookTarget(d, a.LayerID, "layer.filter_set")
	if err != nil {
		return err
	}

	i := filterIndex(l, a.Kind)
	if i < 0 {
		return filterNotFound(l, a.Kind)
	}

	if a.Amount == nil && a.Radius == nil && a.Softness == nil && a.Size == nil && a.Seed == nil && a.Opacity == nil &&
		a.DX == nil && a.DY == nil && a.Blur == nil && a.Width == nil && a.Color == "" {
		return atLeastOne([]string{"the parameters of the filter"}, false)
	}

	f := cloneFilter(l.Filters[i])
	if err := a.merge(&f); err != nil {
		return err
	}

	l.Filters[i] = f

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("filter", object(f))

	return nil
}

type filterKindArgs struct {
	LayerID string `json:"layer_id" pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Kind    string `json:"kind"                                                            enum:"grayscale,sepia,invert,warm,cool,blur,sharpen,vignette,grain,shadow,glow,stroke"`
}

func applyFilterRemove(c *command.Context, d *Document, a *filterKindArgs) error {
	l, err := lookTarget(d, a.LayerID, "layer.filter_remove")
	if err != nil {
		return err
	}

	i := filterIndex(l, a.Kind)
	if i < 0 {
		return filterNotFound(l, a.Kind)
	}

	l.Filters = slices.Delete(l.Filters, i, i+1)
	if len(l.Filters) == 0 {
		l.Filters = nil
	}

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("kind", a.Kind)

	return nil
}

type filterMoveArgs struct {
	LayerID string `json:"layer_id" pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Kind    string `json:"kind"                                                            enum:"grayscale,sepia,invert,warm,cool,blur,sharpen,vignette,grain,shadow,glow,stroke"`
	Index   int    `json:"index"                                                                                                                                                  min:"0" max:"7"`
}

func applyFilterMove(c *command.Context, d *Document, a *filterMoveArgs) error {
	l, err := lookTarget(d, a.LayerID, "layer.filter_move")
	if err != nil {
		return err
	}

	i := filterIndex(l, a.Kind)
	if i < 0 {
		return filterNotFound(l, a.Kind)
	}

	if a.Index >= len(l.Filters) {
		return invalid("args.index", "index is %d; the layer has %d filters.", a.Index, len(l.Filters)).
			WithHint("Use 0 to %d.", len(l.Filters)-1)
	}

	f := l.Filters[i]
	l.Filters = slices.Delete(l.Filters, i, i+1)
	l.Filters = slices.Insert(l.Filters, a.Index, f)

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("kind", a.Kind)
	c.SetEffective("index", a.Index)

	return nil
}

type maskSetArgs struct {
	LayerID string  `json:"layer_id"          pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Kind    string  `json:"kind"                                                                     enum:"rect,rounded_rect,ellipse,layer"`
	X       *int    `json:"x,omitempty"                                                                                                     desc:"Thousandths of the box, 0 to 999."         min:"0" max:"1000"`
	Y       *int    `json:"y,omitempty"                                                                                                     desc:"Thousandths of the box, 0 to 999."         min:"0" max:"1000"`
	W       *int    `json:"w,omitempty"                                                                                                     desc:"Thousandths of the box, 1 to 1000."        min:"1" max:"1000"`
	H       *int    `json:"h,omitempty"                                                                                                     desc:"Thousandths of the box, 1 to 1000."        min:"1" max:"1000"`
	Radius  *int    `json:"radius,omitempty"                                                                                                desc:"rounded_rect only, pixels."                min:"0" max:"16384"`
	Feather *int    `json:"feather,omitempty"                                                                                               desc:"Pixels of soft edge."                      min:"0" max:"200"`
	Invert  bool    `json:"invert,omitempty"`
	Source  *string `json:"source,omitempty"  pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"                                        desc:"kind layer only: the id of another layer."`
}

func applyMaskSet(c *command.Context, d *Document, a *maskSetArgs) error {
	l, err := lookTarget(d, a.LayerID, "layer.mask_set")
	if err != nil {
		return err
	}

	m := Mask{Kind: a.Kind, Invert: a.Invert}

	if a.Kind == MaskLayer {
		for name, v := range map[string]*int{"x": a.X, "y": a.Y, "w": a.W, "h": a.H, "radius": a.Radius, "feather": a.Feather} {
			if v != nil {
				return invalid("args."+name, "A mask of kind layer has no %s.", name).
					WithHint("Give source and, if you want, invert.")
			}
		}

		if a.Source == nil {
			return invalid(
				"args.source",
				"A mask of kind layer needs source.",
			).WithHint("Give the id of another layer.")
		}

		src, rerr := reference(d, *a.Source, "args.source")
		if rerr != nil {
			return rerr
		}

		switch {
		case src.ID == l.ID:
			return invalid("args.source", "A layer cannot be the source of its own mask.").
				WithHint("Name another layer as the source.")
		case src.Kind == KindGroup:
			return command.Errorf(command.CodeTypeMismatch, "Layer %s is a group; a mask takes the alpha of a layer that is not a group.", src.ID).
				WithHint("Use a layer inside the group as the source.").
				WithPath("args.source")
		case src.Mask != nil && src.Mask.Kind == MaskLayer:
			return command.Errorf(command.CodeInvalidState, "Layer %s has a mask from a layer itself.", src.ID).
				WithHint("A mask source cannot have a mask from a layer: remove it first.").WithPath("args.source")
		}

		m.Source = src.ID
	} else {
		if a.Source != nil {
			return invalid(
				"args.source",
				"Only a mask of kind layer has a source.",
			).WithHint("Use kind layer, or remove source.")
		}

		m.X, m.Y, m.W, m.H = orElse(a.X, 0), orElse(a.Y, 0), orElse(a.W, 1000), orElse(a.H, 1000)
		m.Feather = orElse(a.Feather, 0)

		if a.Kind == MaskRoundedRect {
			m.Radius = orElse(a.Radius, 16)
		} else if a.Radius != nil && *a.Radius != 0 {
			return invalid("args.radius", "args.radius is only valid for kind %q, got %q.", MaskRoundedRect, a.Kind).
				WithHint("Use kind rounded_rect with radius.")
		}
	}

	if verr := violations(func(add func(path, format string, a ...any)) {
		validateMask(&m, "args", add)
	}); verr != nil {
		return verr
	}

	// Another layer may use this one as its source: a source cannot have a mask from a layer.
	if m.Kind == MaskLayer {
		for _, other := range usersOfSource(d, l.ID) {
			return command.Errorf(command.CodeInvalidState, "Layer %s uses this layer as the source of its mask.", other).
				WithHint("A mask source cannot have a mask from a layer: clear the mask of %s first.", other).
				WithPath("args.layer_id")
		}
	}

	l.Mask = &m

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("mask", object(m))

	return nil
}

// usersOfSource lists the layers whose mask takes the alpha of the layer with the id.
func usersOfSource(d *Document, id string) []string {
	var out []string

	Walk(d, func(l, _ *Layer, _ int) {
		if l.Mask != nil && l.Mask.Kind == MaskLayer && l.Mask.Source == id {
			out = append(out, l.ID)
		}
	})

	return out
}

type layerOnlyArgs struct {
	LayerID string `json:"layer_id" pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyMaskClear(c *command.Context, d *Document, a *layerOnlyArgs) error {
	l, err := lookTarget(d, a.LayerID, "layer.mask_clear")
	if err != nil {
		return err
	}

	l.Mask = nil

	c.SetEffective("layer_id", l.ID)

	return nil
}

type blendArgs struct {
	LayerID string `json:"layer_id" pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Mode    string `json:"mode"                                                            enum:"normal,multiply,screen,overlay,soft_light,hard_light,darken,lighten,color_dodge,color_burn,difference"`
}

func applyBlend(c *command.Context, d *Document, a *blendArgs) error {
	l, err := lookTarget(d, a.LayerID, "layer.blend")
	if err != nil {
		return err
	}

	l.Blend = a.Mode
	if a.Mode == "normal" {
		l.Blend = ""
	}

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("mode", a.Mode)

	return nil
}

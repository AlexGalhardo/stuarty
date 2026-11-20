package command

import (
	"reflect"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The values a shadow and an outline start with when the command does not give them (VV-122).
const (
	defaultShadowOpacity = 0.5
	defaultShadowDX      = 8
	defaultShadowDY      = 8
	defaultShadowBlur    = 12
	defaultOutlineWidth  = 4
	defaultEffectColour  = "#000000"
)

func init() {
	namedSchemas["shadow_or_null"] = func() map[string]any {
		s := schemaOf(reflect.TypeFor[shadowArgs]())
		s["type"] = []string{"object", "null"}
		return s
	}
	namedSchemas["outline_or_null"] = func() map[string]any {
		s := schemaOf(reflect.TypeFor[outlineArgs]())
		s["type"] = []string{"object", "null"}
		return s
	}

	Register(Define(Spec{
		Name:    "clip.set_shadow",
		Summary: "Gives a clip a drop shadow that follows its visible shape.",
		Description: "Sets a shadow drawn below the clip: the silhouette of its picture (a keyed subject keeps its " +
			"cut-out shape) in color (default #000000), at opacity (0 to 1, default 0.5), moved by dx and dy pixels " +
			"(-500 to 500, default 8) and softened by blur pixels (0 to 100, default 12). Lengths are project pixels. " +
			"Replaces the current shadow; shadow null removes it. Works on clips and still images.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "shadow": {"color": "#000000", "opacity": 0.6, "dx": 10, "dy": 12, "blur": 16}}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
	}, applyClipSetShadow))

	Register(Define(Spec{
		Name:    "clip.set_outline",
		Summary: "Gives a clip an outline that follows its visible shape.",
		Description: "Sets a contour of width pixels (0.5 to 100, default 4) in color (default #000000) around " +
			"the visible shape of the clip, so a keyed subject gets a sticker-like border. Width is in project " +
			"pixels. Replaces the current outline; outline null removes it. Works on clips and still images.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "outline": {"color": "#FFFFFF", "width": 6}}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
	}, applyClipSetOutline))
}

type shadowArgs struct {
	Color   *string  `json:"color,omitempty"   desc:"#RRGGBB, #RRGGBBAA or $name; default #000000."`
	Opacity *float64 `json:"opacity,omitempty" desc:"0 to 1; default 0.5."                          min:"0"    max:"1"`
	DX      *float64 `json:"dx,omitempty"      desc:"Horizontal offset in pixels; default 8."       min:"-500" max:"500"`
	DY      *float64 `json:"dy,omitempty"      desc:"Vertical offset in pixels; default 8."         min:"-500" max:"500"`
	Blur    *float64 `json:"blur,omitempty"    desc:"Soft edge in pixels; default 12."              min:"0"    max:"100"`
}

type outlineArgs struct {
	Color *string  `json:"color,omitempty" desc:"#RRGGBB, #RRGGBBAA or $name; default #000000."`
	Width *float64 `json:"width,omitempty" desc:"Width in pixels; default 4."                   min:"0.5" max:"100"`
}

type clipShadowArgs struct {
	ItemID string      `json:"item_id" desc:"A clip."                           pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Shadow *shadowArgs `json:"shadow"  desc:"The shadow, or null to remove it."                                                        schema:"shadow_or_null"`
}

type clipOutlineArgs struct {
	ItemID  string       `json:"item_id" desc:"A clip."                            pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Outline *outlineArgs `json:"outline" desc:"The outline, or null to remove it."                                                        schema:"outline_or_null"`
}

// effectColour validates an optional colour member and returns it, or the default.
func (c *Context) effectColour(path string, given *string) (timeline.Color, *Error) {
	if given == nil {
		return defaultEffectColour, nil
	}
	if err := c.checkColour(path, *given); err != nil {
		return "", err
	}
	if isHex(*given) {
		return timeline.Color(strings.ToUpper(*given)), nil
	}
	return timeline.Color(*given), nil
}

// orDefault3 returns the value a pointer holds, rounded to 3 decimals, or def.
func orDefault3(p *float64, def float64) float64 {
	if p == nil {
		return def
	}
	return round3(*p)
}

func applyClipSetShadow(c *Context, a *clipShadowArgs) error {
	it, err := c.clipOrCompositionItem(a.ItemID, "a shadow")
	if err != nil {
		return err
	}
	if a.Shadow == nil {
		it.Shadow = nil
		c.Doc.Items[a.ItemID] = it
		c.SetEffective("shadow", nil)
		return nil
	}
	s := a.Shadow
	colour, cerr := c.effectColour("args.shadow.color", s.Color)
	if cerr != nil {
		return cerr
	}
	sh := timeline.Shadow{
		Color:   colour,
		Opacity: orDefault3(s.Opacity, defaultShadowOpacity),
		DX:      orDefault3(s.DX, defaultShadowDX),
		DY:      orDefault3(s.DY, defaultShadowDY),
		Blur:    orDefault3(s.Blur, defaultShadowBlur),
	}
	for _, f := range []struct {
		name   string
		v      float64
		lo, hi float64
	}{{"opacity", sh.Opacity, 0, 1}, {"dx", sh.DX, -500, 500}, {"dy", sh.DY, -500, 500}, {"blur", sh.Blur, 0, 100}} {
		if f.v < f.lo || f.v > f.hi {
			return invalidArg("args.shadow."+f.name, "The value of args.shadow.%s is %s, outside the accepted range.",
				f.name, trimFloat(f.v)).WithHint("Use a value between %s and %s.", trimFloat(f.lo), trimFloat(f.hi))
		}
	}
	if cur := it.Shadow; cur != nil {
		sh.Extra = cur.Extra
	}
	it.Shadow = &sh
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("shadow", map[string]any{
		"color": string(sh.Color), "opacity": sh.Opacity, "dx": sh.DX, "dy": sh.DY, "blur": sh.Blur,
	})
	return nil
}

func applyClipSetOutline(c *Context, a *clipOutlineArgs) error {
	it, err := c.clipOrCompositionItem(a.ItemID, "an outline")
	if err != nil {
		return err
	}
	if a.Outline == nil {
		it.Outline = nil
		c.Doc.Items[a.ItemID] = it
		c.SetEffective("outline", nil)
		return nil
	}
	o := a.Outline
	colour, cerr := c.effectColour("args.outline.color", o.Color)
	if cerr != nil {
		return cerr
	}
	out := timeline.Outline{Color: colour, Width: orDefault3(o.Width, defaultOutlineWidth)}
	if out.Width < 0.5 || out.Width > 100 {
		return invalidArg("args.outline.width", "The value of args.outline.width is %s, outside the accepted range.",
			trimFloat(out.Width)).WithHint("Use a width between 0.5 and 100.")
	}
	if cur := it.Outline; cur != nil {
		out.Extra = cur.Extra
	}
	it.Outline = &out
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("outline", map[string]any{"color": string(out.Color), "width": out.Width})
	return nil
}

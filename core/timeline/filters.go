package timeline

import "slices"

// Filters is the fixed list of colour filters of a visual item (SDD-001 §3.4). They
// are implemented in Go by the compositor, never by an FFmpeg filter (ADR-0011).
var Filters = []string{"none", "grayscale", "sepia", "invert", "warm", "cool"}

// Voices is the fixed list of voice presets of a clip or audio item (SDD-004 §7); absent is none.
var Voices = []string{"clear", "warm", "deep", "bright", "radio"}

func (v *validator) voice(path, key string, it Item) {
	if it.Voice != "" && !slices.Contains(Voices, it.Voice) {
		v.add(VShape, path+".voice", key, "voice must be one of clear, warm, deep, bright or radio")
	}
}

// Blends is the fixed list of blend modes of a visual item (SDD-004 §3.8); absent is "normal".
var Blends = []string{"normal", "multiply", "screen", "overlay", "add"}

// MaxBlur is the largest blur radius, in pixels at project resolution.
const MaxBlur = 100

// MaxMaskFeather is the widest soft edge of a mask, in pixels at project resolution.
const MaxMaskFeather = 200

// filter checks blur and the filter of a visual item.
func (v *validator) filter(path, key string, it Item) {
	if !inRange(it.Blur, 0, MaxBlur) {
		v.add(VShape, path+".blur", key, "blur must be between 0 and %d", MaxBlur)
	}
	known := false
	for _, f := range Filters {
		known = known || f == it.Filter
	}
	if !known {
		v.add(VShape, path+".filter", key, "filter must be one of none, grayscale, sepia, invert, warm or cool")
	}
	if !inRange(it.FilterAmount, 0, 1) {
		v.add(VShape, path+".filter_amount", key, "filter_amount must be between 0 and 1")
	}
	if it.Blend != "" && !slices.Contains(Blends, it.Blend) {
		v.add(VShape, path+".blend", key, "blend must be one of normal, multiply, screen, overlay or add")
	}
	if m := it.Mask; m != nil {
		v.mask(path+".mask", key, m)
	}
	v.shadowOutline(path, key, it)
	if k := it.ChromaKey; k != nil {
		v.color(path+".chroma_key.color", key, k.Color, true)
		for _, s := range []struct {
			name string
			val  float64
		}{{"similarity", k.Similarity}, {"smoothness", k.Smoothness}, {"spill", k.Spill}} {
			if !inRange(s.val, 0, 1) {
				v.add(VShape, path+".chroma_key."+s.name, key, "chroma_key %s must be between 0 and 1", s.name)
			}
		}
	}
}

// shadowOutline checks the drop shadow and the outline of a clip.
func (v *validator) shadowOutline(path, key string, it Item) {
	if s := it.Shadow; s != nil {
		v.color(path+".shadow.color", key, s.Color, true)
		for _, f := range []struct {
			name   string
			val    float64
			lo, hi float64
		}{{"opacity", s.Opacity, 0, 1}, {"dx", s.DX, -500, 500}, {"dy", s.DY, -500, 500}, {"blur", s.Blur, 0, 100}} {
			if !inRange(f.val, f.lo, f.hi) {
				v.add(VShape, path+".shadow."+f.name, key, "shadow %s must be between %v and %v", f.name, f.lo, f.hi)
			}
		}
	}
	if o := it.Outline; o != nil {
		v.color(path+".outline.color", key, o.Color, true)
		if !inRange(o.Width, 0.5, 100) {
			v.add(VShape, path+".outline.width", key, "outline width must be between 0.5 and 100")
		}
	}
}

// mask checks the shape, the rectangle (inside the item) and the feather of a mask.
func (v *validator) mask(path, key string, m *Mask) {
	if m.Shape != "rect" && m.Shape != "ellipse" {
		v.add(VShape, path+".shape", key, "mask shape must be rect or ellipse")
	}
	for _, f := range []struct {
		name   string
		val    float64
		lo, hi float64
	}{{"x", m.X, 0, 1}, {"y", m.Y, 0, 1}, {"width", m.Width, 0.001, 1}, {"height", m.Height, 0.001, 1}} {
		if !inRange(f.val, f.lo, f.hi) {
			v.add(VShape, path+"."+f.name, key, "mask %s must be between %v and %v", f.name, f.lo, f.hi)
		}
	}
	if m.X+m.Width > 1.0005 || m.Y+m.Height > 1.0005 {
		v.add(VShape, path, key, "the mask must stay inside the item: x + width and y + height are at most 1")
	}
	if m.Feather < 0 || m.Feather > MaxMaskFeather {
		v.add(VShape, path+".feather", key, "mask feather must be between 0 and %d", MaxMaskFeather)
	}
}

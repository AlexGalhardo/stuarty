package timeline

// Filters is the fixed list of colour filters of a visual item (SDD-001 §3.4). They
// are implemented in Go by the compositor, never by an FFmpeg filter (ADR-0011).
var Filters = []string{"none", "grayscale", "sepia", "invert", "warm", "cool"}

// MaxBlur is the largest blur radius, in pixels at project resolution.
const MaxBlur = 100

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

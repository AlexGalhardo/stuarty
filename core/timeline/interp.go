package timeline

import "math"

// Ease is e(u) of SDD-004 §3.1 for an easing name; an empty or unknown name is
// linear. It is the only implementation: core/command and core/plan both call it.
func Ease(kind string, u float64) float64 {
	switch kind {
	case "ease_in":
		return u * u * u
	case "ease_out":
		return 1 - (1-u)*(1-u)*(1-u)
	case "ease_in_out":
		if u < 0.5 {
			return 4 * u * u * u
		}
		return 1 - (-2*u+2)*(-2*u+2)*(-2*u+2)/2
	case "hold":
		return 0
	}
	return u
}

// Interpolate evaluates keys, sorted by T and not empty, at the frame r inside the
// item. r may be fractional (the audio envelope needs it). Before the first
// keyframe the first value holds and after the last the last value holds. The
// result is not rounded.
func Interpolate(keys []Keyframe, r float64) float64 {
	last := len(keys) - 1
	if r <= float64(keys[0].T) {
		return keys[0].Value
	}
	if r >= float64(keys[last].T) {
		return keys[last].Value
	}
	lo, hi := 0, last // keys[lo].T <= r < keys[hi].T
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if float64(keys[mid].T) <= r {
			lo = mid
		} else {
			hi = mid
		}
	}
	u := (r - float64(keys[lo].T)) / float64(keys[hi].T-keys[lo].T)
	return keys[lo].Value + (keys[hi].Value-keys[lo].Value)*keys[lo].EaseOf(u)
}

// ValueAtProp is ValueAt for the named property, kept inside the range of that property: a curve that
// overshoots never gives a keyframe written by a trim or a split a value the property cannot hold.
func ValueAtProp(prop string, keys []Keyframe, r int, static float64) float64 {
	v := ValueAt(keys, r, static)
	if lo, hi, ok := KeyframeRange(prop); ok && len(keys) > 0 {
		v = min(max(v, lo), hi)
	}

	return v
}

// ValueAt is the value of an animated property at frame r inside the item, rounded
// to 3 decimals; with no keyframes it is the static value (SDD-004 §3.1).
func ValueAt(keys []Keyframe, r int, static float64) float64 {
	if len(keys) == 0 {
		return static
	}
	return math.Round(Interpolate(keys, float64(r))*1000) / 1000
}

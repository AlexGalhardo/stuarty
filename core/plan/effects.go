package plan

import "github.com/AlexGalhardo/stuarty/core/timeline"

// TransitionSpec is the state of a transition that the executors draw (SDD-004 §3.2): the kinds
// whip, dip and wipe, which change pixels and not only the animated values. P is the progress as
// 16.16 fixed point, strictly between 0 and 65536.
type TransitionSpec struct {
	Kind      string `json:"kind"`
	Edge      string `json:"edge"` // "in" or "out"
	P         int    `json:"p"`
	Color     string `json:"color,omitempty"`     // dip: #RRGGBB, black when absent
	Direction string `json:"direction,omitempty"` // whip and wipe: left, right, up or down, left when absent
}

// transitionProgress is the progress of an edge as the fraction n/m of SDD-004 §3.2 in 16.16 fixed
// point, rounded half up.
func transitionProgress(n, m int) int { return (n*65536 + m/2) / m }

// transitionAt is the transition that changes pixels at item frame r, or nil.
func (p *planner) transitionAt(it *timeline.Item, r int) *TransitionSpec {
	var tr *timeline.Transition

	edge, n := "in", r+1

	switch {
	case it.TransitionIn != nil && r < it.TransitionIn.Duration:
		tr = it.TransitionIn
	case it.TransitionOut != nil && r >= it.Duration-it.TransitionOut.Duration:
		tr, edge, n = it.TransitionOut, "out", it.Duration-r
	}

	if tr == nil || tr.Kind != "whip" && tr.Kind != "dip" && tr.Kind != "wipe" {
		return nil
	}

	spec := &TransitionSpec{Kind: tr.Kind, Edge: edge, P: transitionProgress(n, tr.Duration+1), Direction: tr.Direction}

	if tr.Kind == "dip" {
		spec.Color = "#000000"
		if tr.Color != "" {
			spec.Color = resolveColor(p.doc, string(tr.Color))
		}
	}

	if spec.Kind != "dip" && spec.Direction == "" {
		spec.Direction = "left"
	}

	return spec
}

// pixelEffects is the effects of an item that change pixels, in order. Shake is not among them: it
// moves the layer and the planner applies it (applyShake below).
func pixelEffects(it *timeline.Item) []timeline.Effect {
	var out []timeline.Effect

	for _, e := range it.Effects {
		if e.Kind != timeline.EffectShake {
			out = append(out, e)
		}
	}

	return out
}

// mix32 is the integer hash of SDD-004 §3.13 (the "lowbias32" mixer): u32 arithmetic with wrapping.
func mix32(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16

	return x
}

// Hash3 is hash3(a, b, c) = mix(a + mix(b + mix(c + 0x9e3779b9))) of SDD-004 §3.13, in u32.
func Hash3(a, b, c uint32) uint32 { return mix32(a + mix32(b+mix32(c+0x9e3779b9))) }

// ShakeOffset is the displacement of a layer with the shake effect e at item frame r, in the frame
// pixels of the project and thousandths of a degree (SDD-004 §3.13).
func ShakeOffset(e *timeline.Effect, r int, fps timeline.Rational, width, height int) (dx, dy, rotMilli int) {
	if e.Amount == nil || e.Speed == nil || e.Seed == nil || fps.Num < 1 {
		return 0, 0, 0
	}

	u := int64(r) * int64(*e.Speed) * int64(fps.Den)
	j := uint32((u / int64(fps.Num)) & 0xffffffff)
	f := (u % int64(fps.Num)) * 256 / int64(fps.Num)
	seed := uint32(*e.Seed & 0xffff)
	am := int64(milli(*e.Amount))

	value := func(k uint32) int64 {
		h0 := int64(Hash3(seed, j, k)>>16) - 32768
		h1 := int64(Hash3(seed, j+1, k)>>16) - 32768

		return (h0*(256-f) + h1*f) >> 8
	}

	mx := (am*int64(width)*30 + 500000) / 1000000
	my := (am*int64(height)*30 + 500000) / 1000000

	return int((value(0)*mx + 16384) >> 15), int((value(1)*my + 16384) >> 15), int((value(2)*am*2 + 16384) >> 15)
}

// milli is x in thousandths, rounded half away from zero.
func milli(x float64) int {
	if x < 0 {
		return -int(-x*1000 + 0.5)
	}

	return int(x*1000 + 0.5)
}

// applyShake moves the layer by the shake effect of its item, if any.
func (p *planner) applyShake(it *timeline.Item, r int, l *Layer) {
	for i := range it.Effects {
		if it.Effects[i].Kind != timeline.EffectShake {
			continue
		}

		st := p.doc.Settings
		dx, dy, rot := ShakeOffset(&it.Effects[i], r, st.FPS, st.Width, st.Height)
		l.CX, l.CY = round3(l.CX+float64(dx)), round3(l.CY+float64(dy))
		l.Rotation = round3(l.Rotation + float64(rot)/1000)

		return
	}
}

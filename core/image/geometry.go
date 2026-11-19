package image

// box is a rectangle of whole pixels.
type box struct{ x, y, w, h int }

func boxOf(l *Layer) map[string]int {
	return map[string]int{"x": l.X, "y": l.Y, "w": l.W, "h": l.H}
}

// fitBox is the box of a source of sw by sh that is contained in (contain) or covers (cover) the target box,
// with its aspect kept and centred on it. Every step is integer arithmetic, rounded half up.
func fitBox(sw, sh, tx, ty, tw, th int, contain bool) box {
	widthLimited := tw*sh <= th*sw
	if !contain {
		widthLimited = tw*sh >= th*sw
	}

	var w, h int

	if widthLimited {
		w, h = tw, roundDiv(sh*tw, sw)
	} else {
		w, h = roundDiv(sw*th, sh), th
	}

	w, h = max(w, 1), max(h, 1)

	return box{x: tx + floorDiv(tw-w, 2), y: ty + floorDiv(th-h, 2), w: w, h: h}
}

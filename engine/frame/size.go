// Package frame makes the pictures people and agents look at: one composed frame as a JPEG and a
// contact sheet of several (SDD-004 section 9, SDD-003 section 3.11). Both go through the compositor
// and the decoders of the export, so they show what the export will contain.
package frame

import "math"

// Image token arithmetic of SDD-003 section 3.11 (NFR-022).
const (
	tokenSide        = 28
	maxLongEdge      = 1568
	MaxTokens        = 1568 // the most one image costs on the standard tier
	DefaultFrameBudg = 800
	DefaultSheetBudg = 1500
	// MaxSheetCells is the most cells a sheet has; DefaultSheetCells is the usual count.
	MaxSheetCells     = 16
	DefaultSheetCells = 9
	// DefaultUIWidth and DefaultAgentWidth are the widths of the paused frame of the editor and of
	// a frame for an agent (NFR-023).
	DefaultUIWidth    = 540
	DefaultAgentWidth = 384
	// MaxWidth is the widest frame served.
	MaxWidth = 1080
	// JPEGQuality is the quality of every frame and sheet.
	JPEGQuality = 80
)

// Tokens is the cost of an image of w by h pixels: ceil(w/28) x ceil(h/28).
func Tokens(w, h int) int {
	return ((w + tokenSide - 1) / tokenSide) * ((h + tokenSide - 1) / tokenSide)
}

// ImageSize returns the largest width and height with the aspect ratio aw:ah whose cost is at most
// budget tokens. The long edge starts at 1568 pixels and shrinks by 28 until the cost fits; the short
// edge is rounded to the nearest even number.
func ImageSize(aw, ah, budget int) (w, h, tokens int) {
	if aw <= 0 || ah <= 0 {
		return 0, 0, 0
	}
	long, short := max(aw, ah), min(aw, ah)
	for edge := maxLongEdge; ; edge -= tokenSide {
		s := evenNearest(float64(edge) * float64(short) / float64(long))
		w, h = edge, s
		if ah > aw {
			w, h = s, edge
		}
		tokens = Tokens(w, h)
		if tokens <= budget || edge <= tokenSide {
			return w, h, tokens
		}
	}
}

// evenNearest rounds to the nearest even integer, at least 2 (halves go up).
func evenNearest(x float64) int {
	return max(2, 2*int(math.Floor(x/2+0.5)))
}

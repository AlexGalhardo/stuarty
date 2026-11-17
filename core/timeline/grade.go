package timeline

import "fmt"

// HslRanges are the colour ranges an HslBand adjusts, in hue order (SDD-004 §3.14).
var HslRanges = []string{"red", "orange", "yellow", "green", "cyan", "blue", "purple", "magenta"}

// Curve limits (SDD-001 §3.5).
const (
	MinGradePoints = 2
	MaxGradePoints = 8
)

// HslBand moves one colour range of a clip: Hue shifts it by up to 30 degrees either way (-1 to 1),
// Saturation and Lightness change it by up to 100% (-1 to 1). A band whose three values are 0 is
// not stored.
type HslBand struct {
	Range      string  `json:"range"                enum:"red,orange,yellow,green,cyan,blue,purple,magenta"`
	Hue        float64 `json:"hue,omitempty"                                                                min:"-1" max:"1"`
	Saturation float64 `json:"saturation,omitempty"                                                         min:"-1" max:"1"`
	Lightness  float64 `json:"lightness,omitempty"                                                          min:"-1" max:"1"`
	Extra      Extra   `json:"-"`
}

// Wheel tints one tonal range: Hue is the direction of the tint in degrees (0 red, 120 green, 240
// blue), Amount how strong it is (0 to 1) and Luma lifts or lowers the range (-1 to 1). A wheel with
// Amount 0 and Luma 0 is not stored.
type Wheel struct {
	Hue    float64 `json:"hue"    min:"0"  max:"360"`
	Amount float64 `json:"amount" min:"0"  max:"1"`
	Luma   float64 `json:"luma"   min:"-1" max:"1"`
	Extra  Extra   `json:"-"`
}

// Wheels are the colour wheels of the shadows, the midtones and the highlights (SDD-004 §3.14).
type Wheels struct {
	Shadows    *Wheel `json:"shadows,omitempty"`
	Midtones   *Wheel `json:"midtones,omitempty"`
	Highlights *Wheel `json:"highlights,omitempty"`
	Extra      Extra  `json:"-"`
}

// CurvePoint is one point of a tone curve: X is the input and Y the output, both 0 to 1.
type CurvePoint struct {
	X     float64 `json:"x" min:"0" max:"1"`
	Y     float64 `json:"y" min:"0" max:"1"`
	Extra Extra   `json:"-"`
}

// Curves are the tone curves of a clip (SDD-004 §3.14): Master applies to the three channels, Red,
// Green and Blue to one. Each is 2 to 8 points with X strictly increasing, joined by a smooth curve;
// the identity curve (0,0) to (1,1) is not stored.
type Curves struct {
	Master []CurvePoint `json:"master,omitempty" minitems:"2" maxitems:"8"`
	Red    []CurvePoint `json:"red,omitempty"    minitems:"2" maxitems:"8"`
	Green  []CurvePoint `json:"green,omitempty"  minitems:"2" maxitems:"8"`
	Blue   []CurvePoint `json:"blue,omitempty"   minitems:"2" maxitems:"8"`
	Extra  Extra        `json:"-"`
}

// Channels returns the curves by name in the order master, red, green, blue.
func (c *Curves) Channels() []struct {
	Name   string
	Points []CurvePoint
} {
	return []struct {
		Name   string
		Points []CurvePoint
	}{{"master", c.Master}, {"red", c.Red}, {"green", c.Green}, {"blue", c.Blue}}
}

// IdentityCurve reports whether the points are the straight line from (0,0) to (1,1).
func IdentityCurve(pts []CurvePoint) bool {
	return len(pts) == 2 && pts[0].X == 0 && pts[0].Y == 0 && pts[1].X == 1 && pts[1].Y == 1
}

// CurveProblem says why the points are not a valid curve, or "".
func CurveProblem(pts []CurvePoint) string {
	if len(pts) < MinGradePoints || len(pts) > MaxGradePoints {
		return fmt.Sprintf("a curve has %d to %d points, got %d", MinGradePoints, MaxGradePoints, len(pts))
	}
	for i, p := range pts {
		if !inRange(p.X, 0, 1) || !inRange(p.Y, 0, 1) {
			return fmt.Sprintf("point %d is outside 0 to 1", i)
		}
		if i > 0 && p.X <= pts[i-1].X {
			return fmt.Sprintf("point %d has x %v, not above the x of the point before it", i, p.X)
		}
	}
	return ""
}

// GradeProblem returns the first reason why the advanced colour of a clip is not valid, or "", with
// the path of the part below the item (for example "hsl[1].hue").
func GradeProblem(hsl []HslBand, w *Wheels, c *Curves) (path, msg string) {
	if len(hsl) > len(HslRanges) {
		return "hsl", fmt.Sprintf("hsl has at most %d bands", len(HslRanges))
	}
	seen := map[string]bool{}
	for i, b := range hsl {
		p := fmt.Sprintf("hsl[%d]", i)
		known := false
		for _, r := range HslRanges {
			known = known || r == b.Range
		}
		switch {
		case !known:
			return p + ".range", fmt.Sprintf("%q is not a colour range", b.Range)
		case seen[b.Range]:
			return p + ".range", fmt.Sprintf("range %s appears twice", b.Range)
		case !inRange(b.Hue, -1, 1):
			return p + ".hue", "hue must be between -1 and 1"
		case !inRange(b.Saturation, -1, 1):
			return p + ".saturation", "saturation must be between -1 and 1"
		case !inRange(b.Lightness, -1, 1):
			return p + ".lightness", "lightness must be between -1 and 1"
		}
		seen[b.Range] = true
	}
	if w != nil {
		for _, x := range []struct {
			name string
			w    *Wheel
		}{{"shadows", w.Shadows}, {"midtones", w.Midtones}, {"highlights", w.Highlights}} {
			switch {
			case x.w == nil:
			case !inRange(x.w.Hue, 0, 360):
				return "wheels." + x.name + ".hue", "hue must be between 0 and 360"
			case !inRange(x.w.Amount, 0, 1):
				return "wheels." + x.name + ".amount", "amount must be between 0 and 1"
			case !inRange(x.w.Luma, -1, 1):
				return "wheels." + x.name + ".luma", "luma must be between -1 and 1"
			}
		}
	}
	if c != nil {
		for _, ch := range c.Channels() {
			if ch.Points == nil {
				continue
			}
			if m := CurveProblem(ch.Points); m != "" {
				return "curves." + ch.Name, m
			}
		}
	}
	return "", ""
}

// ColourProps are the colour properties of a clip that keyframe.set can animate (SDD-001 §3.8).
var ColourProps = []string{"brightness", "contrast", "saturation", "temperature", "tint", "hue"}

// ColourRange is the range of a colour property, and whether it is one.
func ColourRange(prop string) (lo, hi float64, ok bool) {
	switch prop {
	case "brightness", "temperature", "tint":
		return -1, 1, true
	case "contrast", "saturation":
		return 0, 2, true
	case "hue":
		return -180, 180, true
	}
	return 0, 0, false
}

// ColourValue is the static value of a colour property of an item.
func (it *Item) ColourValue(prop string) float64 {
	switch prop {
	case "brightness":
		return it.Brightness
	case "contrast":
		return it.Contrast
	case "saturation":
		return it.Saturation
	case "temperature":
		return it.Temperature
	case "tint":
		return it.Tint
	}
	return it.Hue
}

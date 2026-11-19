package command

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

type hslBandArgs struct {
	Range      string   `json:"range"                desc:"The colour range."                                           enum:"red,orange,yellow,green,cyan,blue,purple,magenta"`
	Hue        *float64 `json:"hue,omitempty"        desc:"Shift of the hue, -1 to 1 (30 degrees each way); default 0."                                                         min:"-1" max:"1"`
	Saturation *float64 `json:"saturation,omitempty" desc:"-1 to 1; default 0."                                                                                                 min:"-1" max:"1"`
	Lightness  *float64 `json:"lightness,omitempty"  desc:"-1 to 1; default 0."                                                                                                 min:"-1" max:"1"`
}

type wheelArgs struct {
	Hue    *float64 `json:"hue,omitempty"    desc:"Direction of the tint in degrees, 0 red, 120 green, 240 blue; default 0." min:"0"  max:"360"`
	Amount *float64 `json:"amount,omitempty" desc:"Strength of the tint, 0 to 1; default 0."                                 min:"0"  max:"1"`
	Luma   *float64 `json:"luma,omitempty"   desc:"Lifts or lowers the range, -1 to 1; default 0."                           min:"-1" max:"1"`
}

type wheelsArgs struct {
	Shadows    *wheelArgs `json:"shadows,omitempty"    desc:"Replaces the wheel; a neutral wheel (amount 0, luma 0) removes it."`
	Midtones   *wheelArgs `json:"midtones,omitempty"   desc:"As shadows."`
	Highlights *wheelArgs `json:"highlights,omitempty" desc:"As shadows."`
}

type curvePointArgs struct {
	X float64 `json:"x" desc:"Input, 0 to 1."  min:"0" max:"1"`
	Y float64 `json:"y" desc:"Output, 0 to 1." min:"0" max:"1"`
}

type curvesArgs struct {
	Master []curvePointArgs `json:"master,omitempty" desc:"2 to 8 points, x strictly increasing; [] or the line (0,0)-(1,1) removes the curve." maxitems:"8"`
	Red    []curvePointArgs `json:"red,omitempty"    desc:"As master, for the red channel."                                                     maxitems:"8"`
	Green  []curvePointArgs `json:"green,omitempty"  desc:"As master, for the green channel."                                                   maxitems:"8"`
	Blue   []curvePointArgs `json:"blue,omitempty"   desc:"As master, for the blue channel."                                                    maxitems:"8"`
}

func val(p *float64) float64 {
	if p == nil {
		return 0
	}
	return round3(*p)
}

// setGrade merges hsl, wheels and curves into the item: a band, a wheel or a curve that is given
// replaces the stored one, and a neutral one removes it (SDD-001 section 3.5).
func setGrade(it *timeline.Item, a *clipColorArgs) *Error {
	if len(a.Hsl) > 0 {
		if err := setHsl(it, a.Hsl); err != nil {
			return err
		}
	}
	if a.Wheels != nil {
		setWheels(it, a.Wheels)
	}
	if a.Curves != nil {
		return setCurves(it, a.Curves)
	}
	return nil
}

func setHsl(it *timeline.Item, given []hslBandArgs) *Error {
	next := map[string]timeline.HslBand{}
	for _, b := range it.Hsl {
		next[b.Range] = b
	}
	seen := map[string]bool{}
	for i := range given {
		g := &given[i]
		if seen[g.Range] {
			return invalidArg(fmt.Sprintf("args.hsl[%d].range", i), "args.hsl: range %s appears twice.", g.Range).
				WithHint("Send one band per range.")
		}
		seen[g.Range] = true
		next[g.Range] = timeline.HslBand{
			Range: g.Range, Hue: val(g.Hue), Saturation: val(g.Saturation), Lightness: val(g.Lightness),
		}
	}
	it.Hsl = nil
	for _, r := range timeline.HslRanges {
		if b, ok := next[r]; ok && (b.Hue != 0 || b.Saturation != 0 || b.Lightness != 0) {
			it.Hsl = append(it.Hsl, b)
		}
	}
	return nil
}

func setWheels(it *timeline.Item, given *wheelsArgs) {
	w := timeline.Wheels{}
	if it.Wheels != nil {
		w = *it.Wheels
	}
	for _, x := range []struct {
		given *wheelArgs
		dst   **timeline.Wheel
	}{{given.Shadows, &w.Shadows}, {given.Midtones, &w.Midtones}, {given.Highlights, &w.Highlights}} {
		if x.given == nil {
			continue
		}
		next := timeline.Wheel{Hue: val(x.given.Hue), Amount: val(x.given.Amount), Luma: val(x.given.Luma)}
		if next.Amount == 0 && next.Luma == 0 {
			*x.dst = nil
		} else {
			*x.dst = &next
		}
	}
	it.Wheels = &w
	if w.Shadows == nil && w.Midtones == nil && w.Highlights == nil {
		it.Wheels = nil
	}
}

func setCurves(it *timeline.Item, given *curvesArgs) *Error {
	c := timeline.Curves{}
	if it.Curves != nil {
		c = *it.Curves
	}
	for _, x := range []struct {
		name  string
		given []curvePointArgs
		dst   *[]timeline.CurvePoint
	}{
		{"master", given.Master, &c.Master},
		{"red", given.Red, &c.Red},
		{"green", given.Green, &c.Green},
		{"blue", given.Blue, &c.Blue},
	} {
		if x.given == nil {
			continue
		}
		pts := make([]timeline.CurvePoint, len(x.given))
		for i, p := range x.given {
			pts[i] = timeline.CurvePoint{X: round3(p.X), Y: round3(p.Y)}
		}
		if len(pts) == 0 || timeline.IdentityCurve(pts) {
			*x.dst = nil
			continue
		}
		if m := timeline.CurveProblem(pts); m != "" {
			return invalidArg("args.curves."+x.name, "args.curves.%s: %s.", x.name, m).
				WithHint("Send 2 to 8 points with x strictly increasing and every value from 0 to 1.")
		}
		*x.dst = pts
	}
	it.Curves = &c
	if c.Master == nil && c.Red == nil && c.Green == nil && c.Blue == nil {
		it.Curves = nil
	}
	return nil
}

// gradeEffective reports the stored advanced colour for the parts the call gave.
func gradeEffective(c *Context, it *timeline.Item, a *clipColorArgs) {
	if len(a.Hsl) > 0 {
		hsl := it.Hsl
		if hsl == nil {
			hsl = []timeline.HslBand{}
		}
		c.SetEffective("hsl", hsl)
	}
	if a.Wheels != nil {
		c.SetEffective("wheels", it.Wheels)
	}
	if a.Curves != nil {
		c.SetEffective("curves", it.Curves)
	}
}

package command

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Style arguments (SDD-002 R15): a partial TextStyle or CaptionStyle given as an
// object. The stored style holds exactly the keys the caller set.

type styleRule struct {
	min, max *float64 // numbers
	whole    bool     // an integer, as a multiple of 100 for the weight
	enum     []string
	colour   bool
	boolean  bool
	text     bool
}

var styleRules = map[string]styleRule{
	"font_family":          {text: true},
	"font_weight":          {min: new(float64(100)), max: new(float64(900)), whole: true},
	"size":                 {min: new(float64(8)), max: new(float64(1000))},
	"color":                {colour: true},
	"stroke_color":         {colour: true},
	"stroke_width":         {min: new(float64(0)), max: new(float64(40))},
	"shadow_color":         {colour: true},
	"shadow_dx":            {},
	"shadow_dy":            {},
	"shadow_blur":          {min: new(float64(0)), max: new(float64(100))},
	"background_color":     {colour: true},
	"background_padding":   {min: new(float64(0))},
	"background_radius":    {min: new(float64(0))},
	"align":                {enum: []string{"left", "center", "right"}},
	"line_height":          {min: new(float64(0.8)), max: new(float64(3))},
	"max_width":            {min: new(float64(0))},
	"uppercase":            {boolean: true},
	"position":             {enum: []string{"bottom", "middle", "top", "custom"}},
	"highlight":            {enum: []string{"none", "word"}},
	"highlight_color":      {colour: true},
	"highlight_background": {colour: true},
}

func jsonKind(v any) string {
	switch v.(type) {
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case json.Number:
		return "a number"
	case nil:
		return "null"
	case []any:
		return "an array"
	default:
		return "an object"
	}
}

func isHex(s string) bool {
	if len(s) != 7 && len(s) != 9 || s[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(s[1:], 16, 64)
	return err == nil
}

// checkStyleValue checks one value against its rule; caption says whether the
// caption-only keys are allowed.
func (c *Context) checkStyleValue(path, key string, v any, rule styleRule) *Error {
	bad := func(format string, a ...any) *Error {
		return Errorf(CodeValidation, format, a...).WithPath(path)
	}
	switch {
	case rule.boolean:
		if _, ok := v.(bool); !ok {
			return bad("%s must be a boolean, got %s.", path, jsonKind(v)).WithHint("Send true or false.")
		}
	case rule.text:
		if s, ok := v.(string); !ok || s == "" {
			return bad(
				"%s must be a non-empty string, got %s.",
				path,
				jsonKind(v),
			).WithHint(`Send a family name such as "Inter".`)
		}
	case rule.colour:
		return c.checkColour(path, v)
	case rule.enum != nil:
		s, ok := v.(string)
		if !ok || !slices.Contains(rule.enum, s) {
			return bad("%s must be one of %s, got %v.", path, strings.Join(rule.enum, ", "), v).
				WithHint("Use one of: %s.", strings.Join(rule.enum, ", "))
		}
	default:
		return checkStyleNumber(path, key, v, rule)
	}
	return nil
}

func checkStyleNumber(path, key string, v any, rule styleRule) *Error {
	n, ok := v.(json.Number)
	if !ok {
		return Errorf(
			CodeValidation,
			"%s must be a number, got %s.",
			path,
			jsonKind(v),
		).WithHint("Send a number.").
			WithPath(path)
	}
	f, err := strconv.ParseFloat(string(n), 64)
	inRange := err == nil && (rule.min == nil || f >= *rule.min) && (rule.max == nil || f <= *rule.max)
	if rule.whole {
		inRange = inRange && f == float64(int(f)) && int(f)%100 == 0
	}
	if key == "max_width" {
		inRange = err == nil && f > 0
	}
	if inRange {
		return nil
	}
	want := "a valid number"
	switch {
	case key == "font_weight":
		want = "a multiple of 100 between 100 and 900"
	case key == "max_width":
		want = "greater than 0"
	case rule.min != nil && rule.max != nil:
		want = fmt.Sprintf("between %s and %s", trimFloat(*rule.min), trimFloat(*rule.max))
	case rule.min != nil:
		want = "at least " + trimFloat(*rule.min)
	}
	return Errorf(CodeValidation, "%s must be %s, got %s.", path, want, string(n)).
		WithHint("Use a value that is %s.", want).WithPath(path)
}

func (c *Context) checkColour(path string, v any) *Error {
	s, ok := v.(string)
	if !ok {
		return Errorf(CodeValidation, "%s must be a colour string, got %s.", path, jsonKind(v)).
			WithHint(`Send "#RRGGBB" or "#RRGGBBAA".`).WithPath(path)
	}
	if isHex(s) {
		return nil
	}
	if name, ok := strings.CutPrefix(s, "$"); ok {
		if t := c.Doc.Template; t != nil {
			if _, has := t.Colors[name]; has {
				return nil
			}
		}
		return Errorf(CodeValidation, "%s refers to colour %q, which is not in template.colors.", path, s).
			WithHint("Use a hex colour, or add the name with template.set_info first.").WithPath(path)
	}
	return Errorf(CodeValidation, "%s %q is not a colour.", path, s).
		WithHint(`Send "#RRGGBB" or "#RRGGBBAA".`).WithPath(path)
}

// validateStyle checks a style argument (R15): known keys only, each value in range,
// no null unless allowNull (a merge removes the key). caption says whether the
// caption-only keys exist.
func (c *Context) validateStyle(style map[string]any, caption, allowNull bool) *Error {
	const path = "args.style"
	text, capKeys := timeline.StyleKeys()
	keys := text
	if caption {
		keys = append(slices.Clone(text), capKeys...)
	}
	slices.Sort(keys)
	given := make([]string, 0, len(style))
	for k := range style {
		given = append(given, k)
	}
	slices.Sort(given)
	for _, k := range given {
		kp := path + "." + k
		if !slices.Contains(keys, k) {
			return Errorf(CodeValidation, "%s is not a style key.", kp).
				WithHint("The style keys are: %s.", strings.Join(keys, ", ")).WithDetail("allowed", keys).WithPath(kp)
		}
		if style[k] == nil {
			if allowNull {
				continue
			}
			return Errorf(CodeValidation, "%s is null, which is not allowed here.", kp).
				WithHint("Remove the key, or give it a value.").WithPath(kp)
		}
		if e := c.checkStyleValue(kp, k, style[k], styleRules[k]); e != nil {
			return e
		}
	}
	return nil
}

// mergeStyle returns the stored style after merging given into current (a null value
// removes a key) or, with replace, after replacing it (null keys ignored). An empty
// result is nil.
func mergeStyle(current *timeline.CaptionStyle, given map[string]any, replace bool) (*timeline.CaptionStyle, *Error) {
	merged := map[string]any{}
	if !replace {
		maps.Copy(merged, timeline.StyleToMap(current))
	}
	for k, v := range given {
		if v == nil {
			if !replace {
				delete(merged, k)
			}
			continue
		}
		merged[k] = v
	}
	if len(merged) == 0 {
		return nil, nil
	}
	s, err := timeline.StyleFromMap(merged)
	if err != nil {
		return nil, AsError(err)
	}
	return s, nil
}

// checkFont returns E_FONT_UNKNOWN when the family or the weight of a style does not
// exist (R15).
func (c *Context) checkFont(s *timeline.CaptionStyle) *Error {
	const path = "args.style"
	if s == nil || s.FontFamily == nil && s.FontWeight == nil {
		return nil
	}
	family := "Inter"
	if s.FontFamily != nil {
		family = *s.FontFamily
	}
	weights, ok := timeline.FontWeights(c.Doc, family)
	if !ok {
		avail := slices.Clone(timeline.BundledFamilies)
		project := slices.Sorted(maps.Keys(c.Doc.Fonts))
		avail = append(avail, project...)
		slices.Sort(avail)
		message := fmt.Sprintf("Font %q is not available; available families: %s.", family, strings.Join(avail, ", "))
		if len(project) > 0 {
			bundled := slices.Clone(timeline.BundledFamilies)
			slices.Sort(bundled)
			message = fmt.Sprintf("Font %q is not available; bundled families: %s; project fonts: %s.",
				family, strings.Join(bundled, ", "), strings.Join(project, ", "))
		}
		return Errorf(CodeFontUnknown, "%s", message).
			WithHint("Use font_family %q.", "Inter").
			WithDetail("available", avail).
			WithPath(path + ".font_family")
	}
	if s.FontWeight == nil {
		return nil // the default weight falls to the nearest the family has (VV-057)
	}
	weight := *s.FontWeight
	if slices.Contains(weights, weight) {
		return nil
	}
	ws := make([]string, len(weights))
	for i, w := range weights {
		ws[i] = strconv.Itoa(w)
	}
	return Errorf(
		CodeFontUnknown,
		"Font weight %d does not exist for family %q; available weights: %s.",
		weight,
		family,
		strings.Join(ws, ", "),
	).
		WithHint("Use font_weight %d.", weights[len(weights)-1]).
		WithDetail("available", weights).
		WithPath(path + ".font_weight")
}

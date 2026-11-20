package command

import (
	"maps"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// namedSchemas are the schemas of the arguments whose Go type is any or a map of any
// because the apply function checks the shape itself (the schema tag of args.go). Each
// one describes exactly what that check accepts, so a form can be built from it.
var namedSchemas = map[string]func() map[string]any{
	"text_style":                 func() map[string]any { return styleSchema(false, false) },
	"text_style_merge":           func() map[string]any { return styleSchema(false, true) },
	"caption_style_merge":        func() map[string]any { return styleSchema(true, true) },
	"caption_style_or_nil":       func() map[string]any { return nullable(styleSchema(true, false)) },
	"caption_style_merge_or_nil": func() map[string]any { return nullable(styleSchema(true, true)) },
	"speed":                      speedSchema,
	"fps":                        fpsSchema,
	"transition_kind":            func() map[string]any { return nullable(map[string]any{"type": "string", "enum": transitionKinds}) },
	"template_colors":            templateColorsSchema,
	"processor_params":           processorParamsSchema,
	"chroma_color":               chromaColorSchema,
}

const (
	hexColourPattern  = `^#([0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})$`
	namedColourFormat = `^\$[a-z][a-z0-9_]{0,39}$`
)

// nullable lets a schema accept null too: the type becomes [T, "null"] and an enum
// gains null.
func nullable(s map[string]any) map[string]any {
	out := maps.Clone(s)
	if t, ok := s["type"].(string); ok {
		out["type"] = []string{t, "null"}
	}
	if e, ok := s["enum"].([]string); ok {
		withNull := make([]any, 0, len(e)+1)
		for _, v := range e {
			withNull = append(withNull, v)
		}
		out["enum"] = append(withNull, nil)
	}
	return out
}

func styleValueSchema(key string) map[string]any {
	rule := styleRules[key]
	switch {
	case rule.boolean:
		return map[string]any{"type": "boolean"}
	case rule.text:
		return map[string]any{"type": "string", "minLength": 1}
	case rule.colour:
		return map[string]any{
			"type": "string", "pattern": "(" + hexColourPattern + ")|(" + namedColourFormat + ")",
			"description": `"#RRGGBB", "#RRGGBBAA" or "$name" of a template colour`,
		}
	case rule.enum != nil:
		return map[string]any{"type": "string", "enum": slices.Clone(rule.enum)}
	}
	s := map[string]any{"type": "number"}
	if rule.whole {
		s["type"] = "integer"
		s["multipleOf"] = 100
	}
	if rule.min != nil {
		s["minimum"] = *rule.min
	}
	if rule.max != nil {
		s["maximum"] = *rule.max
	}
	if key == "max_width" {
		delete(s, "minimum")
		s["exclusiveMinimum"] = 0
	}
	return s
}

// styleSchema is the partial style of validateStyle: known keys only, each in its
// range; merge lets a value be null, which removes the key.
func styleSchema(caption, merge bool) map[string]any {
	text, capKeys := timeline.StyleKeys()
	keys := slices.Clone(text)
	if caption {
		keys = append(keys, capKeys...)
	}
	slices.Sort(keys)
	props := map[string]any{}
	for _, k := range keys {
		p := styleValueSchema(k)
		if merge {
			p = nullable(p)
		}
		props[k] = p
	}
	return map[string]any{"type": "object", "properties": props, "required": []string{}, "additionalProperties": false}
}

func rationalSchema(den any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"num": map[string]any{"type": "integer", "minimum": 1},
			"den": den,
		},
		"required": []string{"num", "den"}, "additionalProperties": false,
	}
}

func speedSchema() map[string]any {
	return map[string]any{"oneOf": []any{
		map[string]any{
			"type":     "string",
			"pattern":  `^([0-9]+\.?[0-9]{0,3}|\.[0-9]{1,3})x$`,
			"examples": []string{"1.5x"},
		},
		rationalSchema(map[string]any{"type": "integer", "minimum": 1}),
	}}
}

func fpsSchema() map[string]any {
	return map[string]any{"oneOf": []any{
		map[string]any{"type": "string", "pattern": `^[0-9]+(/[0-9]+)?$`, "examples": []string{"30", "30000/1001"}},
		rationalSchema(map[string]any{"type": "integer", "enum": []int{1, 1001}}),
	}}
}

func templateColorsSchema() map[string]any {
	return map[string]any{
		"type":          "object",
		"propertyNames": map[string]any{"pattern": slotName.String()},
		"additionalProperties": map[string]any{
			"type": []string{"string", "null"}, "pattern": hexColourPattern,
		},
	}
}

func chromaColorSchema() map[string]any {
	return map[string]any{
		"type":        []string{"string", "null"},
		"pattern":     "(" + hexColourPattern + ")|(" + namedColourFormat + ")",
		"description": `"#RRGGBB", "#RRGGBBAA" or "$name" of a template colour; null removes the key`,
	}
}

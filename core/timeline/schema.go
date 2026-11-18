package timeline

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Schema returns the JSON Schema (2020-12) of the document, generated from the Go
// types and their tags (ADR-0014, SDD-001 §7). Besides json and default, the tags
// are enum, min, max, gt (exclusive minimum), step, pattern, minlen, maxlen,
// minitems, maxitems, keypattern, maxprops and desc. Unknown fields are allowed at
// every object level (SDD-001 §2 item 9), so no object closes additionalProperties.
func Schema() map[string]any {
	g := &schemaGen{defs: map[string]any{}}
	root := g.object(reflect.TypeFor[Document]())
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = "https://vukem-video.dev/schema/timeline.v1.schema.json"
	root["title"] = "Timeline document"
	root["x-status"] = "frozen"
	root["$defs"] = g.defs
	return root
}

type schemaGen struct{ defs map[string]any }

const (
	colorPattern = `^(#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?|\$[a-z][a-z0-9_]{0,39})$`
	// backgroundPattern is colorPattern plus the keyword of a frame without a background colour.
	backgroundPattern = `^(#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?|\$[a-z][a-z0-9_]{0,39}|transparent)$`
)

var (
	keyframeProperties = slices.Concat(
		[]string{"x", "y", "scale", "rotation", "opacity", "volume"},
		ColourProps,
		MaskProps,
		CropProps,
	)
	rationalType  = reflect.TypeFor[Rational]()
	keyframesType = reflect.TypeFor[Keyframes]()
	colorTypeOf   = reflect.TypeFor[Color]()
)

// ref returns a $ref to the definition of a named struct, generating it once.
func (g *schemaGen) ref(t reflect.Type) map[string]any {
	name := t.Name()
	if _, ok := g.defs[name]; !ok {
		g.defs[name] = nil // reserve the name: types may refer to themselves
		g.defs[name] = g.object(t)
	}
	return map[string]any{"$ref": "#/$defs/" + name}
}

func (g *schemaGen) object(t reflect.Type) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, f := range infoOf(t).fields {
		sf := t.FieldByIndex(f.index)
		s := g.field(t.Name(), f.name, sf)
		if f.hasDef {
			s["default"] = defaultValue(sf.Type, f.def)
		}
		props[f.name] = s
		if !f.hasDef && !f.omitempty {
			required = append(required, f.name)
		}
	}
	slices.Sort(required)
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// field is the schema of one struct field, with its tag constraints.
func (g *schemaGen) field(owner, name string, sf reflect.StructField) map[string]any {
	t := sf.Type
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	s := g.typeSchema(t)
	if owner == "Settings" && name == "fps" {
		s = map[string]any{"allOf": []any{
			s,
			map[string]any{
				"properties": map[string]any{
					"den": map[string]any{"enum": []any{1, 1001}},
					"num": map[string]any{"maximum": 120 * 1001},
				},
			},
		}, "description": "den is 1 or 1001; num/den between 1 and 120."}
		return s
	}
	if owner == "Settings" && name == "background" {
		s["pattern"] = backgroundPattern
	}
	tags(s, sf.Tag, t)
	return s
}

func (g *schemaGen) typeSchema(t reflect.Type) map[string]any {
	switch {
	case t == keyframesType:
		return map[string]any{
			"type":          "object",
			"propertyNames": map[string]any{"enum": keyframeProperties},
			"additionalProperties": map[string]any{
				"type": "array", "maxItems": 100, "items": g.ref(reflect.TypeFor[Keyframe]()),
			},
		}
	case t == colorTypeOf:
		return map[string]any{"type": "string", "pattern": colorPattern}
	case t.Kind() == reflect.Struct:
		return g.ref(t)
	case t.Kind() == reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.typeSchema(t.Elem())}
	case t.Kind() == reflect.Slice:
		return map[string]any{"type": "array", "items": g.typeSchema(t.Elem())}
	case t.Kind() == reflect.String:
		return map[string]any{"type": "string"}
	case t.Kind() == reflect.Bool:
		return map[string]any{"type": "boolean"}
	case t.Kind() == reflect.Int:
		return map[string]any{"type": "integer"}
	case t.Kind() == reflect.Float64:
		return map[string]any{"type": "number"}
	}
	panic("timeline: no schema for " + t.String())
}

func number(s string) any {
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic("timeline: bad number tag " + s)
	}
	return f
}

// tags applies the constraint tags of a field to its schema.
func tags(s map[string]any, tag reflect.StructTag, t reflect.Type) {
	if ref, ok := s["$ref"]; ok && len(tagKeys(tag)) > 0 {
		for k := range s {
			delete(s, k)
		}
		s["allOf"] = []any{map[string]any{"$ref": ref}}
	}
	set := func(key, jsonKey string, conv func(string) any) {
		if v, ok := tag.Lookup(key); ok {
			s[jsonKey] = conv(v)
		}
	}
	str := func(v string) any { return v }
	set("desc", "description", str)
	set("pattern", "pattern", str)
	set("min", "minimum", number)
	set("max", "maximum", number)
	set("gt", "exclusiveMinimum", number)
	set("step", "multipleOf", number)
	set("minlen", "minLength", number)
	set("maxlen", "maxLength", number)
	set("minitems", "minItems", number)
	set("maxitems", "maxItems", number)
	set("maxprops", "maxProperties", number)
	if v, ok := tag.Lookup("keypattern"); ok {
		s["propertyNames"] = map[string]any{"pattern": v}
	}
	if v, ok := tag.Lookup("enum"); ok {
		var vals []any
		for e := range strings.SplitSeq(v, ",") {
			if t.Kind() == reflect.String {
				vals = append(vals, e)
			} else {
				vals = append(vals, number(e))
			}
		}
		s["enum"] = vals
	}
}

func tagKeys(tag reflect.StructTag) []string {
	var keys []string
	for _, k := range []string{"desc", "pattern", "min", "max", "gt", "step", "minlen", "maxlen", "minitems", "maxitems", "maxprops", "keypattern", "enum"} {
		if _, ok := tag.Lookup(k); ok {
			keys = append(keys, k)
		}
	}
	return keys
}

func defaultValue(t reflect.Type, def string) any {
	v := parseDefault(t, def)
	if t == rationalType {
		r, _ := reflect.TypeAssert[Rational](v)
		return map[string]any{"num": r.Num, "den": r.Den}
	}
	return v.Interface()
}

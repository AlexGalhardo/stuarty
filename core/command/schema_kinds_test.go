package command

import (
	"reflect"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// timeArgPaths lists where the arguments type holds a Time, as schema paths.
func timeArgPaths(t reflect.Type, path string) []string {
	if t == timeType {
		return []string{path}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return timeArgPaths(t.Elem(), path)
	case reflect.Slice:
		return timeArgPaths(t.Elem(), path+"[]")
	case reflect.Map:
		return timeArgPaths(t.Elem(), path+"{}")
	case reflect.Struct:
		var out []string
		for _, f := range argFields(t) {
			out = append(out, timeArgPaths(f.fieldType, path+"."+f.name)...)
		}
		return out
	default:
		return nil
	}
}

func timeMarkedPaths(s map[string]any, path string) []string {
	var out []string
	if s["x-vv-kind"] == "time" {
		out = append(out, path)
	}
	if props, ok := s["properties"].(map[string]any); ok {
		for k, v := range props {
			out = append(out, timeMarkedPaths(v.(map[string]any), path+"."+k)...)
		}
	}
	if items, ok := s["items"].(map[string]any); ok {
		out = append(out, timeMarkedPaths(items, path+"[]")...)
	}
	if extra, ok := s["additionalProperties"].(map[string]any); ok {
		out = append(out, timeMarkedPaths(extra, path+"{}")...)
	}
	return out
}

// SDD-012 section 9.2: the generator marks every argument of the core's time type
// with x-vv-kind "time", from the type and not from the name, and marks nothing else.
func TestSchemaMarksExactlyTheTimeArguments(t *testing.T) {
	var total int
	for _, c := range Default().List() {
		want := timeArgPaths(c.argsType, "")
		got := timeMarkedPaths(schemaOf(c.argsType), "")
		slices.Sort(want)
		slices.Sort(got)
		if !slices.Equal(want, got) {
			t.Errorf("%s: time arguments %v, marked %v", c.Name, want, got)
		}
		total += len(want)
	}
	if total == 0 {
		t.Fatal("no time argument found: the walk is broken")
	}
}

func untypedPaths(s map[string]any, path string) []string {
	var out []string
	for _, key := range []string{"oneOf", "anyOf"} {
		if branches, ok := s[key].([]any); ok {
			for _, b := range branches {
				out = append(out, untypedPaths(b.(map[string]any), path)...)
			}
			return out
		}
	}
	if s["type"] == nil {
		return []string{path}
	}
	if props, ok := s["properties"].(map[string]any); ok {
		for k, v := range props {
			out = append(out, untypedPaths(v.(map[string]any), path+"."+k)...)
		}
	}
	if items, ok := s["items"].(map[string]any); ok {
		out = append(out, untypedPaths(items, path+"[]")...)
	}
	if extra, ok := s["additionalProperties"].(map[string]any); ok {
		out = append(out, untypedPaths(extra, path+"{}")...)
	}
	return out
}

// A form is built from args_schema: a property without a type becomes a JSON textarea.
// A new command argument of type any needs a schema tag (args.go).
func TestNoPublicCommandPropertyIsUntyped(t *testing.T) {
	for _, c := range Default().List() {
		if c.Internal {
			continue
		}
		for _, p := range untypedPaths(schemaOf(c.argsType), c.Name) {
			t.Errorf("%s has no JSON type", p)
		}
	}
}

func TestStyleSchemasHaveTheKeysTheCoreAccepts(t *testing.T) {
	text, capKeys := timeline.StyleKeys()
	all := slices.Concat(text, capKeys)
	for name, tc := range map[string]struct {
		keys  []string
		merge bool
	}{
		"text_style": {text, false}, "text_style_merge": {text, true}, "caption_style_merge": {all, true},
	} {
		props := namedSchemas[name]()["properties"].(map[string]any)
		got := make([]string, 0, len(props))
		for k, v := range props {
			got = append(got, k)
			types, isList := v.(map[string]any)["type"].([]string)
			if isList != tc.merge || tc.merge && !slices.Contains(types, "null") {
				t.Errorf("%s.%s: type %v, null allowed should be %v", name, k, v.(map[string]any)["type"], tc.merge)
			}
		}
		slices.Sort(got)
		want := slices.Clone(tc.keys)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: keys %v, want %v", name, got, want)
		}
	}
}

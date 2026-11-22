package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The arguments of a command are a Go struct; its tags are the schema (SDD-002
// §A.2) and drive both the validation of Apply and the JSON Schema of the
// contract generator:
//
//	json:"name"            the argument is required
//	json:"name,omitempty"  the argument is optional (use a pointer when the zero
//	                       value is meaningful)
//	enum:"a,b"             the value is one of these strings
//	min:"1" max:"200"      bounds of a number, or of the length of a string or array
//	desc:"text"            one sentence for the help text
//	internal:"true"        the service fills the field; the published schema leaves it out
//	schema:"name"          a field of type any, or a map of any, takes the named schema of
//	                       namedSchemas (the shape the apply function checks)
//
// A field of type any accepts any JSON value.

type argField struct {
	name      string
	index     int
	required  bool
	enum      []string
	min, max  *float64
	desc      string
	schema    string
	fieldType reflect.Type
}

func argFields(t reflect.Type) []argField {
	var out []argField
	for i := range t.NumField() {
		sf := t.Field(i)
		tag, ok := sf.Tag.Lookup("json")
		if !ok || tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		f := argField{
			name:      name,
			index:     i,
			required:  opts != "omitempty" && opts != "omitzero",
			desc:      sf.Tag.Get("desc"),
			schema:    sf.Tag.Get("schema"),
			fieldType: sf.Type,
		}
		if e, ok := sf.Tag.Lookup("enum"); ok {
			f.enum = strings.Split(e, ",")
		}
		f.min = boundTag(sf, "min")
		f.max = boundTag(sf, "max")
		out = append(out, f)
	}
	return out
}

func boundTag(sf reflect.StructField, key string) *float64 {
	s, ok := sf.Tag.Lookup(key)
	if !ok {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(fmt.Sprintf("command: bad %s tag %q on %s", key, s, sf.Name))
	}
	return &v
}

func kindOf(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case json.Number:
		return "a number"
	case []any:
		return "an array"
	default:
		return "an object"
	}
}

func typeWord(t reflect.Type) string {
	if t == timeType {
		return "a time"
	}
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "a boolean"
	case reflect.Int:
		return "a whole number"
	case reflect.Float64:
		return "a number"
	case reflect.Slice:
		return "an array"
	case reflect.Pointer:
		return typeWord(t.Elem())
	case reflect.Interface:
		return "any value"
	default:
		return "an object"
	}
}

func invalidArg(path, format string, a ...any) *Error {
	return Errorf(CodeValidation, format, a...).WithPath(path)
}

// decodeArgs parses the arguments JSON of an operation into a new value of type t.
// A missing or null argument list is the empty object.
func decodeArgs(raw json.RawMessage, t reflect.Type) (reflect.Value, *Error) {
	var src any = map[string]any{}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.UseNumber()
		if err := dec.Decode(&src); err != nil {
			return reflect.Value{}, invalidArg("args", "The arguments are not valid JSON: %s.", syntaxWord(err)).
				WithHint("Send args as one JSON object, for example the example of the command.")
		}
		if _, err := dec.Token(); !errors.Is(err, io.EOF) {
			return reflect.Value{}, invalidArg("args", "The arguments have data after the JSON object.").
				WithHint("Send args as exactly one JSON object.")
		}
	}
	v := reflect.New(t).Elem()
	if e := decodeArg(v, src, "args"); e != nil {
		return reflect.Value{}, e
	}
	return v, nil
}

func syntaxWord(err error) string {
	if se, ok := errors.AsType[*json.SyntaxError](err); ok {
		return fmt.Sprintf("syntax error at byte %d", se.Offset)
	}
	return "unexpected end of input"
}

// Time is a time argument: a string with a unit ("12.5s", "375f", "00:00:12.500").
// A JSON number is rejected with E_INVALID_TIME (SDD-002 §A.4); parsing the string is
// the job of Context.ParseTime.
type Time string

var timeType = reflect.TypeFor[Time]()

const timeForms = `Use one of "12.5s" (seconds), "375f" (frames) or "00:00:12.500" (timecode).`

func decodeArg(v reflect.Value, src any, path string) *Error {
	t := v.Type()
	if t == timeType {
		s, ok := src.(string)
		if !ok {
			return Errorf(CodeInvalidTime, "%s is %s, but a time is a string with a unit.", path, kindOf(src)).
				WithHint("%s", timeForms).WithPath(path)
		}
		v.SetString(s)
		return nil
	}
	switch t.Kind() {
	case reflect.Interface:
		if src != nil {
			v.Set(reflect.ValueOf(src))
		}
		return nil
	case reflect.Pointer:
		if src == nil {
			return nil
		}
		p := reflect.New(t.Elem())
		if e := decodeArg(p.Elem(), src, path); e != nil {
			return e
		}
		v.Set(p)
		return nil
	case reflect.Struct:
		return decodeStructArg(v, src, path)
	case reflect.Map:
		return decodeMapArg(v, src, path)
	case reflect.Slice:
		return decodeSliceArg(v, src, path)
	case reflect.String:
		s, ok := src.(string)
		if !ok {
			return wrongType(path, t, src)
		}
		v.SetString(s)
	case reflect.Bool:
		b, ok := src.(bool)
		if !ok {
			return wrongType(path, t, src)
		}
		v.SetBool(b)
	case reflect.Int:
		n, ok := src.(json.Number)
		if !ok {
			return wrongType(path, t, src)
		}
		i, err := strconv.ParseInt(string(n), 10, 64)
		if err != nil {
			return invalidArg(path, "%s must be a whole number, but got %s.", path, string(n)).
				WithHint("Send an integer, for example 12.")
		}
		v.SetInt(i)
	case reflect.Float64:
		n, ok := src.(json.Number)
		if !ok {
			return wrongType(path, t, src)
		}
		f, err := strconv.ParseFloat(string(n), 64)
		if err != nil || math.IsInf(f, 0) {
			return invalidArg(path, "%s is out of range: %s.", path, string(n)).WithHint("Send a smaller number.")
		}
		v.SetFloat(f)
	default:
		panic("command: unsupported argument kind " + t.Kind().String())
	}
	return nil
}

func wrongType(path string, t reflect.Type, src any) *Error {
	return invalidArg(path, "%s must be %s, but got %s.", path, typeWord(t), kindOf(src)).
		WithHint("Send %s for %s.", typeWord(t), path)
}

func decodeStructArg(v reflect.Value, src any, path string) *Error {
	m, ok := src.(map[string]any)
	if !ok {
		return wrongType(path, v.Type(), src)
	}
	fields := argFields(v.Type())
	known := make([]string, len(fields))
	for i, f := range fields {
		known[i] = f.name
	}
	slices.Sort(known)
	for _, k := range sortedMapKeys(m) {
		if !slices.Contains(known, k) {
			return invalidArg(path+"."+SafeKey(k), "%s.%s is not an argument of this command.", path, SafeKey(k)).
				WithHint("The arguments are: %s.", strings.Join(known, ", ")).
				WithDetail("allowed", known)
		}
	}
	for _, f := range fields {
		raw, present := m[f.name]
		fp := path + "." + f.name
		if !present ||
			(raw == nil && f.fieldType.Kind() != reflect.Interface && f.fieldType.Kind() != reflect.Pointer) {
			if f.required {
				return invalidArg(fp, "The required argument %s is missing.", fp).
					WithHint("Add %q (%s) to the arguments.", f.name, typeWord(f.fieldType))
			}
			continue
		}
		fv := v.Field(f.index)
		if e := decodeArg(fv, raw, fp); e != nil {
			return e
		}
		if e := checkConstraints(f, fv, fp); e != nil {
			return e
		}
	}
	return nil
}

func decodeMapArg(v reflect.Value, src any, path string) *Error {
	m, ok := src.(map[string]any)
	if !ok {
		return wrongType(path, v.Type(), src)
	}
	out := reflect.MakeMapWithSize(v.Type(), len(m))
	for _, k := range sortedMapKeys(m) {
		e := reflect.New(v.Type().Elem()).Elem()
		if err := decodeArg(e, m[k], path+"."+SafeKey(k)); err != nil {
			return err
		}
		out.SetMapIndex(reflect.ValueOf(k), e)
	}
	v.Set(out)
	return nil
}

func decodeSliceArg(v reflect.Value, src any, path string) *Error {
	a, ok := src.([]any)
	if !ok {
		return wrongType(path, v.Type(), src)
	}
	out := reflect.MakeSlice(v.Type(), len(a), len(a))
	for i, e := range a {
		if err := decodeArg(out.Index(i), e, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	v.Set(out)
	return nil
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func checkConstraints(f argField, v reflect.Value, path string) *Error {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if f.enum != nil && v.Kind() == reflect.String && !slices.Contains(f.enum, v.String()) {
		return invalidArg(path, "%s is %q, which is not one of the accepted values.", path, v.String()).
			WithHint("Use one of: %s.", strings.Join(f.enum, ", ")).
			WithDetail("allowed", f.enum)
	}
	var size float64
	switch v.Kind() {
	case reflect.Int:
		size = float64(v.Int())
	case reflect.Float64:
		size = v.Float()
	case reflect.String:
		size = float64(utf8.RuneCountInString(v.String()))
	case reflect.Slice:
		size = float64(v.Len())
	default:
		return nil
	}
	what := "value"
	switch v.Kind() {
	case reflect.String:
		what = "length"
	case reflect.Slice:
		what = "length"
	default:
	}
	if f.min != nil && size < *f.min || f.max != nil && size > *f.max {
		return invalidArg(path, "The %s of %s is %s, outside the accepted range.", what, path, trimFloat(size)).
			WithHint("Use a %s %s.", what, rangeWord(f))
	}
	return nil
}

func rangeWord(f argField) string {
	switch {
	case f.min != nil && f.max != nil:
		return fmt.Sprintf("between %s and %s", trimFloat(*f.min), trimFloat(*f.max))
	case f.min != nil:
		return "of at least " + trimFloat(*f.min)
	default:
		return "of at most " + trimFloat(*f.max)
	}
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// schemaOf builds the JSON Schema (2020-12 subset) of an arguments type.
func schemaOf(t reflect.Type) map[string]any {
	if t == timeType {
		return map[string]any{
			"type":        "string",
			"x-vv-kind":   "time",
			"description": "a time with a unit: \"12.5s\", \"375f\" or \"00:00:12.500\"",
		}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return schemaOf(t.Elem())
	case reflect.Interface:
		return map[string]any{}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int:
		return map[string]any{"type": "integer"}
	case reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": schemaOf(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaOf(t.Elem())}
	default: // a struct, described below
	}
	props := map[string]any{}
	required := []string{}
	for _, f := range argFields(t) {
		if t.Field(f.index).Tag.Get("internal") == "true" {
			continue
		}
		p := schemaOf(f.fieldType)
		if name := f.schema; name != "" {
			build, ok := namedSchemas[name]
			if !ok {
				panic("command: unknown schema tag " + name)
			}
			p = build()
		}
		if f.desc != "" {
			p["description"] = f.desc
		}
		if f.enum != nil {
			p["enum"] = f.enum
		}
		applyBounds(p, f)
		applyPattern(p, t.Field(f.index).Tag.Get("pattern"))
		props[f.name] = p
		if f.required {
			required = append(required, f.name)
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func applyBounds(p map[string]any, f argField) {
	keys := [2]string{"minimum", "maximum"}
	switch p["type"] {
	case "string":
		keys = [2]string{"minLength", "maxLength"}
	case "array":
		keys = [2]string{"minItems", "maxItems"}
	}
	if f.min != nil {
		p[keys[0]] = *f.min
	}
	if f.max != nil {
		p[keys[1]] = *f.max
	}
}

// applyPattern puts the pattern tag on the string, or on the elements of a list of strings.
func applyPattern(p map[string]any, pattern string) {
	if pattern == "" {
		return
	}
	if items, ok := p["items"].(map[string]any); ok && p["type"] == "array" {
		items["pattern"] = pattern
		return
	}
	p["pattern"] = pattern
}

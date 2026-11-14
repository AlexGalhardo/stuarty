package timeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

var (
	extraType = reflect.TypeFor[Extra]()
	colorType = reflect.TypeFor[Color]()
)

type field struct {
	name      string
	index     []int
	omitempty bool
	def       string
	hasDef    bool
}

type structInfo struct {
	fields []field
	extra  []int
}

var infoCache sync.Map

func infoOf(t reflect.Type) *structInfo {
	if v, ok := infoCache.Load(t); ok {
		if si, ok := v.(*structInfo); ok {
			return si
		}
	}
	si := &structInfo{}
	collect(t, nil, si)
	infoCache.Store(t, si)
	return si
}

func collect(t reflect.Type, prefix []int, si *structInfo) {
	for i := range t.NumField() {
		sf := t.Field(i)
		idx := append(append([]int(nil), prefix...), i)
		if sf.Type == extraType {
			si.extra = idx
			continue
		}
		tag, ok := sf.Tag.Lookup("json")
		if sf.Anonymous && !ok {
			collect(sf.Type, idx, si)
			continue
		}
		if !ok || tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		f := field{name: name, index: idx, omitempty: opts == "omitempty" || opts == "omitzero"}
		f.def, f.hasDef = sf.Tag.Lookup("default")
		si.fields = append(si.fields, f)
	}
}

// parseDefault builds the value of a default tag for a field of type t.
func parseDefault(t reflect.Type, def string) reflect.Value {
	v := reflect.New(t).Elem()
	var err error
	switch t.Kind() {
	case reflect.Float64:
		var f float64
		f, err = strconv.ParseFloat(def, 64)
		v.SetFloat(f)
	case reflect.Int:
		var n int64
		n, err = strconv.ParseInt(def, 10, 64)
		v.SetInt(n)
	case reflect.String:
		v.SetString(def)
	case reflect.Bool:
		var b bool
		b, err = strconv.ParseBool(def)
		v.SetBool(b)
	case reflect.Struct:
		if t != reflect.TypeFor[Rational]() {
			err = errors.New("unsupported struct default")
			break
		}
		num, den, ok := strings.Cut(def, "/")
		var n, d int64
		var e1, e2 error
		n, e1 = strconv.ParseInt(num, 10, 64)
		d, e2 = strconv.ParseInt(den, 10, 64)
		if !ok || e1 != nil || e2 != nil {
			err = errors.New("bad rational default")
		}
		v.Set(reflect.ValueOf(Rational{Num: int(n), Den: int(d)}))
	default:
		err = errors.New("unsupported default kind")
	}
	if err != nil {
		panic(fmt.Sprintf("timeline: bad default tag %q on %s: %v", def, t, err))
	}
	return v
}

func applyDefaults(v reflect.Value) {
	for _, f := range infoOf(v.Type()).fields {
		if f.hasDef {
			fv := v.FieldByIndex(f.index)
			fv.Set(parseDefault(fv.Type(), f.def))
		}
	}
}

func withDefaults[T any]() T {
	var z T
	applyDefaults(reflect.ValueOf(&z).Elem())
	return z
}

// Violation is one broken invariant. Code is a V_* code of SDD-001 §4, Path the
// dotted path of the offending value and ID the identifier of the object it is on.
// Related lists the other objects the violation involves (the track of an item that
// is on the wrong kind of track, the asset of an item whose source range is wrong,
// the item an overlapping item collides with).
type Violation struct {
	Code    string   `json:"code"`
	Path    string   `json:"path"`
	ID      string   `json:"id,omitempty"`
	Message string   `json:"message"`
	Related []string `json:"related,omitempty"`
}

func (v Violation) String() string {
	return v.Code + " at " + v.Path + ": " + v.Message
}

// ViolationsError is the error of Decode: the document is not usable.
type ViolationsError struct {
	Violations []Violation
}

func (e *ViolationsError) Error() string {
	parts := make([]string, len(e.Violations))
	for i, v := range e.Violations {
		parts[i] = v.String()
	}
	return strings.Join(parts, "; ")
}

type decoder struct{ errs []Violation }

func (d *decoder) fail(path, format string, args ...any) {
	d.errs = append(d.errs, Violation{Code: VShape, Path: path, Message: fmt.Sprintf(format, args...)})
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func (d *decoder) decode(v reflect.Value, src any, path string) {
	switch v.Kind() {
	case reflect.Pointer:
		if src == nil {
			return
		}
		p := reflect.New(v.Type().Elem())
		d.decode(p.Elem(), src, path)
		v.Set(p)
	case reflect.Struct:
		d.decodeStruct(v, src, path)
	case reflect.Map:
		m, ok := src.(map[string]any)
		if !ok {
			d.fail(path, "expected an object")
			return
		}
		out := reflect.MakeMapWithSize(v.Type(), len(m))
		for _, k := range sortedKeys(m) {
			e := reflect.New(v.Type().Elem()).Elem()
			d.decode(e, m[k], join(path, k))
			out.SetMapIndex(reflect.ValueOf(k), e)
		}
		v.Set(out)
	case reflect.Slice:
		a, ok := src.([]any)
		if !ok {
			d.fail(path, "expected an array")
			return
		}
		out := reflect.MakeSlice(v.Type(), len(a), len(a))
		for i, e := range a {
			d.decode(out.Index(i), e, fmt.Sprintf("%s[%d]", path, i))
		}
		v.Set(out)
	case reflect.String:
		s, ok := src.(string)
		if !ok {
			d.fail(path, "expected a string")
			return
		}
		if v.Type() == colorType && isHexColor(s) {
			s = strings.ToUpper(s)
		}
		v.SetString(s)
	case reflect.Bool:
		b, ok := src.(bool)
		if !ok {
			d.fail(path, "expected a boolean")
			return
		}
		v.SetBool(b)
	case reflect.Int:
		n, ok := src.(json.Number)
		if !ok {
			d.fail(path, "expected an integer")
			return
		}
		i, err := strconv.ParseInt(string(n), 10, 64)
		if err != nil {
			d.fail(path, "expected an integer, got %s", string(n))
			return
		}
		v.SetInt(i)
	case reflect.Float64:
		n, ok := src.(json.Number)
		if !ok {
			d.fail(path, "expected a number")
			return
		}
		f, err := strconv.ParseFloat(string(n), 64)
		if err != nil {
			d.fail(path, "number out of range: %s", string(n))
			return
		}
		v.SetFloat(f)
	default:
		panic("timeline: unsupported kind " + v.Kind().String())
	}
}

func (d *decoder) decodeStruct(v reflect.Value, src any, path string) {
	m, ok := src.(map[string]any)
	if !ok {
		d.fail(path, "expected an object")
		return
	}
	applyDefaults(v)
	si := infoOf(v.Type())
	known := make(map[string]bool, len(si.fields))
	for _, f := range si.fields {
		known[f.name] = true
		raw, present := m[f.name]
		if !present {
			if !f.hasDef && !f.omitempty {
				d.fail(join(path, f.name), "required field is missing")
			}
			continue
		}
		d.decode(v.FieldByIndex(f.index), raw, join(path, f.name))
	}
	if si.extra != nil {
		var extra Extra
		for k, val := range m {
			if !known[k] {
				if extra == nil {
					extra = Extra{}
				}
				extra[k] = val
			}
		}
		v.FieldByIndex(si.extra).Set(reflect.ValueOf(extra))
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isHexColor(s string) bool {
	if len(s) != 7 && len(s) != 9 || s[0] != '#' {
		return false
	}
	for _, c := range []byte(s[1:]) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// RawDoc is a document as decoded JSON, before it is typed. It is what migrations
// work on (SDD-001 §6). Numbers are json.Number.
type RawDoc map[string]any

// DecodeRaw parses JSON into a RawDoc. The top level must be an object.
func DecodeRaw(data []byte) (RawDoc, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var src any
	if err := dec.Decode(&src); err != nil {
		return nil, &ViolationsError{[]Violation{{Code: VShape, Path: "", Message: "not valid JSON: " + err.Error()}}}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, &ViolationsError{
			[]Violation{{Code: VShape, Path: "", Message: "unexpected data after the JSON document"}},
		}
	}
	m, ok := src.(map[string]any)
	if !ok {
		return nil, &ViolationsError{
			[]Violation{{Code: VShape, Path: "", Message: "the document must be a JSON object"}},
		}
	}
	return RawDoc(m), nil
}

// Decode parses and types a document, migrating an older schema version first.
// It returns a *ViolationsError (V_SHAPE for a value of the wrong type or a missing
// required field, V_SCHEMA_VERSION for a version this binary does not support).
// Decode does not check the invariants; call Validate for that.
func Decode(data []byte) (*Document, error) {
	raw, err := DecodeRaw(data)
	if err != nil {
		return nil, err
	}
	raw, err = Migrate(raw)
	if err != nil {
		return nil, err
	}
	return FromRaw(raw)
}

// FromRaw types a RawDoc of the current schema version.
func FromRaw(raw RawDoc) (*Document, error) {
	d := &decoder{}
	doc := &Document{}
	d.decode(reflect.ValueOf(doc).Elem(), map[string]any(raw), "")
	if len(d.errs) > 0 {
		return nil, &ViolationsError{d.errs}
	}
	return doc, nil
}

// Check decodes data and validates it, returning every violation found.
func Check(data []byte) []Violation {
	doc, err := Decode(data)
	if err != nil {
		if ve, ok := errors.AsType[*ViolationsError](err); ok {
			return ve.Violations
		}
		return []Violation{{Code: VShape, Message: err.Error()}}
	}
	return Validate(doc)
}

// Canonical returns the canonical encoding of a document (SDD-001 §2 item 8): keys
// sorted, no whitespace, shortest numbers, fields equal to their default omitted.
// A non-finite number, which Validate rejects, is written as null.
func Canonical(doc *Document) []byte {
	var buf bytes.Buffer
	writeValue(&buf, reflect.ValueOf(doc))
	return buf.Bytes()
}

func equalsDefault(v reflect.Value, def string) bool {
	return reflect.DeepEqual(v.Interface(), parseDefault(v.Type(), def).Interface())
}

func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	case reflect.Map, reflect.Slice:
		return v.Len() == 0
	default:
		return v.IsZero()
	}
}

func writeValue(buf *bytes.Buffer, v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			buf.WriteString("null")
			return
		}
		writeValue(buf, v.Elem())
	case reflect.Struct:
		writeStruct(buf, v)
	case reflect.Map:
		buf.WriteByte('{')
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeString(buf, k.String())
			buf.WriteByte(':')
			writeValue(buf, v.MapIndex(k))
		}
		buf.WriteByte('}')
	case reflect.Slice:
		buf.WriteByte('[')
		for i := range v.Len() {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeValue(buf, v.Index(i))
		}
		buf.WriteByte(']')
	case reflect.String:
		writeString(buf, v.String())
	case reflect.Bool:
		buf.WriteString(strconv.FormatBool(v.Bool()))
	case reflect.Int:
		buf.WriteString(strconv.FormatInt(v.Int(), 10))
	case reflect.Float64:
		writeFloat(buf, v.Float())
	default:
		panic("timeline: unsupported kind " + v.Kind().String())
	}
}

func writeStruct(buf *bytes.Buffer, v reflect.Value) {
	si := infoOf(v.Type())
	entries := make(map[string][]byte, len(si.fields))
	for _, f := range si.fields {
		fv := v.FieldByIndex(f.index)
		if f.hasDef {
			if equalsDefault(fv, f.def) {
				continue
			}
		} else if f.omitempty && isEmpty(fv) {
			continue
		}
		var b bytes.Buffer
		writeValue(&b, fv)
		if f.omitempty && !f.hasDef && fv.Kind() == reflect.Pointer && b.String() == "{}" {
			continue
		}
		entries[f.name] = b.Bytes()
	}
	if si.extra != nil {
		extra, _ := reflect.TypeAssert[Extra](v.FieldByIndex(si.extra)) // the field is always an Extra
		for k, val := range extra {
			if _, dup := entries[k]; dup {
				continue
			}
			var b bytes.Buffer
			writeAny(&b, val)
			entries[k] = b.Bytes()
		}
	}
	buf.WriteByte('{')
	for i, k := range sortedKeys(entries) {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeString(buf, k)
		buf.WriteByte(':')
		buf.Write(entries[k])
	}
	buf.WriteByte('}')
}

// writeAny writes a value of an Extra (decoded JSON) in canonical form.
func writeAny(buf *bytes.Buffer, v any) {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case map[string]any:
		buf.WriteByte('{')
		for i, k := range sortedKeys(x) {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeString(buf, k)
			buf.WriteByte(':')
			writeAny(buf, x[k])
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeAny(buf, e)
		}
		buf.WriteByte(']')
	case string:
		writeString(buf, x)
	case bool:
		buf.WriteString(strconv.FormatBool(x))
	case float64:
		writeFloat(buf, x)
	case json.Number:
		if i, err := strconv.ParseInt(string(x), 10, 64); err == nil {
			buf.WriteString(strconv.FormatInt(i, 10))
		} else if allDigits(strings.TrimPrefix(string(x), "-")) {
			buf.WriteString(string(x)) // an integer beyond int64 keeps every digit
		} else if f, err := strconv.ParseFloat(string(x), 64); err == nil && !math.IsInf(f, 0) {
			writeFloat(buf, f)
		} else {
			buf.WriteString(string(x))
		}
	default:
		panic(fmt.Sprintf("timeline: unsupported extra value %T", v))
	}
}

func writeFloat(buf *bytes.Buffer, f float64) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		buf.WriteString("null")
		return
	}
	if f == 0 {
		buf.WriteByte('0')
		return
	}
	buf.WriteString(strconv.FormatFloat(f, 'f', -1, 64))
}

// writeString writes a JSON string. HTML characters are not escaped, so the bytes
// stay readable; U+2028, U+2029 and control characters are escaped, and invalid
// UTF-8 becomes U+FFFD.
func writeString(buf *bytes.Buffer, s string) {
	const hex = "0123456789abcdef"
	buf.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			buf.WriteByte('\\')
			buf.WriteRune(r)
		case r == '\n':
			buf.WriteString(`\n`)
		case r == '\r':
			buf.WriteString(`\r`)
		case r == '\t':
			buf.WriteString(`\t`)
		case r < 0x20 || r == ' ' || r == ' ' || r == utf8.RuneError:
			buf.WriteString(`\u`)
			buf.WriteByte(hex[r>>12&15])
			buf.WriteByte(hex[r>>8&15])
			buf.WriteByte(hex[r>>4&15])
			buf.WriteByte(hex[r&15])
		default:
			buf.WriteRune(r)
		}
	}
	buf.WriteByte('"')
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

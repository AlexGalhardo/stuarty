package library

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// FormatError is a front matter the parser refuses (SDD-018 section 3).
type FormatError struct {
	Line int // 1 based, in the file; 0 when no line applies
	Msg  string
}

func (e *FormatError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("front matter, line %d: %s", e.Line, e.Msg)
	}

	return "front matter: " + e.Msg
}

// ValueKind says what a front matter value holds.
type ValueKind int

// The three shapes of a value.
const (
	ScalarValue ValueKind = iota
	ListValue
	MapValue
)

// Value is one value of the front matter: a string, a list of strings or a map
// from string to string. Nothing nests deeper.
type Value struct {
	Kind ValueKind
	Str  string
	// Plain is set on a scalar that the file wrote without quotes (and by SetBool): it
	// is written back without quotes, so a number or a boolean of a key the product does
	// not know keeps its type for another reader.
	Plain bool
	List  []string
	Map   map[string]string
}

// FrontMatter is an ordered set of keys. The order of the file is kept so that
// a rewrite does not move what a person arranged.
type FrontMatter struct {
	keys []string
	vals map[string]Value
}

// NewFrontMatter returns an empty front matter.
func NewFrontMatter() *FrontMatter {
	return &FrontMatter{vals: map[string]Value{}}
}

// Clone copies the front matter.
func (f *FrontMatter) Clone() *FrontMatter {
	c := NewFrontMatter()
	for _, k := range f.keys {
		v := f.vals[k]
		c.set(
			k,
			Value{
				Kind:  v.Kind,
				Str:   v.Str,
				Plain: v.Plain,
				List:  append([]string(nil), v.List...),
				Map:   cloneMap(v.Map),
			},
		)
	}

	return c
}

func cloneMap(m map[string]string) map[string]string {
	return maps.Clone(m)
}

func (f *FrontMatter) set(key string, v Value) {
	if _, ok := f.vals[key]; !ok {
		f.keys = append(f.keys, key)
	}

	f.vals[key] = v
}

// Keys returns the keys in file order.
func (f *FrontMatter) Keys() []string { return append([]string(nil), f.keys...) }

// Has reports whether the key is present.
func (f *FrontMatter) Has(key string) bool { _, ok := f.vals[key]; return ok }

// Scalar returns the string of a scalar key; a list or a map reads as "".
func (f *FrontMatter) Scalar(key string) string {
	if v, ok := f.vals[key]; ok && v.Kind == ScalarValue {
		return v.Str
	}

	return ""
}

// List returns the items of a list key (a scalar reads as a list of one).
func (f *FrontMatter) List(key string) []string {
	v, ok := f.vals[key]
	if !ok {
		return nil
	}

	switch v.Kind {
	case ListValue:
		return append([]string(nil), v.List...)
	case ScalarValue:
		if v.Str != "" {
			return []string{v.Str}
		}
	case MapValue:
	}

	return nil
}

// Map returns a copy of a map key.
func (f *FrontMatter) Map(key string) map[string]string {
	if v, ok := f.vals[key]; ok && v.Kind == MapValue {
		return cloneMap(v.Map)
	}

	return nil
}

// SetScalar sets a string; an empty string removes the key.
func (f *FrontMatter) SetScalar(key, value string) {
	if value == "" {
		f.Delete(key)

		return
	}

	f.set(key, Value{Kind: ScalarValue, Str: value})
}

// SetBool writes true or false.
func (f *FrontMatter) SetBool(key string, value bool) {
	if value {
		f.set(key, Value{Kind: ScalarValue, Str: "true", Plain: true})

		return
	}

	f.set(key, Value{Kind: ScalarValue, Str: "false", Plain: true})
}

// SetList sets a list; an empty list removes the key.
func (f *FrontMatter) SetList(key string, items []string) {
	if len(items) == 0 {
		f.Delete(key)

		return
	}

	f.set(key, Value{Kind: ListValue, List: append([]string(nil), items...)})
}

// SetMap sets a map; an empty map removes the key.
func (f *FrontMatter) SetMap(key string, m map[string]string) {
	if len(m) == 0 {
		f.Delete(key)

		return
	}

	f.set(key, Value{Kind: MapValue, Map: cloneMap(m)})
}

// Delete removes a key.
func (f *FrontMatter) Delete(key string) {
	if _, ok := f.vals[key]; !ok {
		return
	}

	delete(f.vals, key)

	for i, k := range f.keys {
		if k == key {
			f.keys = append(f.keys[:i], f.keys[i+1:]...)

			break
		}
	}
}

var (
	topKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
	mapKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// SplitDocument separates the front matter from the body of a file. The file
// must start with a line "---" and the front matter ends at the next line
// "---". Line endings are read as line feeds, and a leading byte order mark is
// ignored. bodyLine is the line number of the first line of the body.
func SplitDocument(text string) (front, body string, bodyLine int, err error) {
	text = strings.TrimPrefix(text, string(rune(0xFEFF)))
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return "", "", 0, &FormatError{Line: 1, Msg: "the file must start with a line of three hyphens"}
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") != "---" {
			continue
		}

		front = strings.Join(lines[1:i], "\n")
		if len(front) > MaxFrontMatter {
			return "", "", 0, &FormatError{Msg: fmt.Sprintf("it is longer than %d bytes", MaxFrontMatter)}
		}

		return front, strings.Join(lines[i+1:], "\n"), i + 2, nil
	}

	return "", "", 0, &FormatError{Msg: "it is not closed by a line of three hyphens"}
}

// ParseDocument reads a whole file: its front matter and its body.
func ParseDocument(text string) (*FrontMatter, string, error) {
	front, body, _, err := SplitDocument(text)
	if err != nil {
		return nil, "", err
	}

	fm, err := ParseFrontMatter(front)
	if err != nil {
		return nil, "", err
	}

	return fm, body, nil
}

// ParseFrontMatter reads the text between the two fences. Line numbers in an
// error are those of the file (the first front matter line is line 2).
func ParseFrontMatter(front string) (*FrontMatter, error) {
	fm := NewFrontMatter()
	lines := strings.Split(front, "\n")

	for i := 0; i < len(lines); {
		line := lines[i]
		lineNo := i + 2

		if strings.TrimSpace(line) == "" ||
			strings.HasPrefix(strings.TrimSpace(line), "#") && line[0] != ' ' && line[0] != '\t' {
			i++

			continue
		}

		if line[0] == ' ' || line[0] == '\t' {
			return nil, &FormatError{Line: lineNo, Msg: "unexpected indentation"}
		}

		key, rest, ok := strings.Cut(line, ":")
		if !ok || !topKey.MatchString(key) {
			return nil, &FormatError{Line: lineNo, Msg: "expected key: value"}
		}

		if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
			return nil, &FormatError{Line: lineNo, Msg: "expected a space after the colon"}
		}

		if fm.Has(key) {
			return nil, &FormatError{Line: lineNo, Msg: "the key " + command.SafeKey(key) + " appears twice"}
		}

		val, next, err := parseValue(lines, i, strings.TrimSpace(rest))
		if err != nil {
			return nil, err
		}

		fm.set(key, val)
		i = next
	}

	return fm, nil
}

// parseValue reads the value of the key on lines[i]; rest is what follows the
// colon on that line. It returns the index of the next unread line.
func parseValue(lines []string, i int, rest string) (Value, int, error) {
	lineNo := i + 2

	switch {
	case rest == "" || strings.HasPrefix(rest, "#"):
		return parseNested(lines, i)
	case rest[0] == '|' || rest[0] == '>':
		return parseBlockScalar(lines, i, rest)
	case rest[0] == '[':
		items, err := parseFlowList(rest, lineNo)
		if err != nil {
			return Value{}, 0, err
		}

		return Value{Kind: ListValue, List: items}, i + 1, nil
	case rest[0] == '{':
		return Value{}, 0, &FormatError{
			Line: lineNo,
			Msg:  "flow maps are not supported; write the map on indented lines",
		}
	}

	s, err := parseScalar(rest, lineNo)
	if err != nil {
		return Value{}, 0, err
	}

	return Value{
		Kind:  ScalarValue,
		Str:   s,
		Plain: !strings.HasPrefix(rest, `"`) && !strings.HasPrefix(rest, "'"),
	}, i + 1, nil
}

// parseNested reads what follows a key with nothing after its colon: a block
// list, a one-level block map, or an empty value.
func parseNested(lines []string, i int) (Value, int, error) {
	j := i + 1
	for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
		j++
	}

	if j >= len(lines) ||
		!strings.HasPrefix(lines[j], " ") && !strings.HasPrefix(lines[j], "\t") && !strings.HasPrefix(lines[j], "- ") &&
			lines[j] != "-" {
		return Value{Kind: ScalarValue}, i + 1, nil
	}

	first := strings.TrimLeft(lines[j], " \t")
	if first == "-" || strings.HasPrefix(first, "- ") {
		return parseBlockList(lines, j)
	}

	return parseBlockMap(lines, j)
}

func parseBlockList(lines []string, start int) (Value, int, error) {
	v := Value{Kind: ListValue}
	j := start

	for ; j < len(lines); j++ {
		line := lines[j]
		if strings.TrimSpace(line) == "" {
			continue
		}

		t := strings.TrimLeft(line, " \t")
		if t != "-" && !strings.HasPrefix(t, "- ") {
			break
		}

		item := strings.TrimSpace(strings.TrimPrefix(t, "-"))
		if strings.HasPrefix(item, "#") {
			item = ""
		}

		if isMapEntry(item) {
			return Value{}, 0, &FormatError{Line: j + 2, Msg: "a list item cannot be a map"}
		}

		s, err := parseScalar(item, j+2)
		if err != nil {
			return Value{}, 0, err
		}

		v.List = append(v.List, s)
	}

	return v, j, nil
}

// isMapEntry reports whether an unquoted list item reads "key: value".
func isMapEntry(item string) bool {
	if item == "" || item[0] == '"' || item[0] == '\'' {
		return false
	}

	k, rest, ok := strings.Cut(item, ":")

	return ok && mapKey.MatchString(k) && (rest == "" || rest[0] == ' ')
}

func parseBlockMap(lines []string, start int) (Value, int, error) {
	v := Value{Kind: MapValue, Map: map[string]string{}}
	indent := -1
	j := start

	for ; j < len(lines); j++ {
		line := lines[j]
		if strings.TrimSpace(line) == "" {
			continue
		}

		lead := len(line) - len(strings.TrimLeft(line, " \t"))
		if lead == 0 {
			break
		}

		if strings.TrimSpace(line)[0] == '#' {
			continue
		}

		if indent < 0 {
			indent = lead
		}

		if lead != indent {
			return Value{}, 0, &FormatError{Line: j + 2, Msg: "a map nests one level only, with one indentation"}
		}

		k, rest, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || !mapKey.MatchString(k) || rest != "" && rest[0] != ' ' {
			return Value{}, 0, &FormatError{Line: j + 2, Msg: "expected key: value"}
		}

		if _, dup := v.Map[k]; dup {
			return Value{}, 0, &FormatError{Line: j + 2, Msg: "the key " + command.SafeKey(k) + " appears twice"}
		}

		rest = strings.TrimSpace(rest)
		if rest != "" && (rest[0] == '|' || rest[0] == '>' || rest[0] == '[' || rest[0] == '{') {
			return Value{}, 0, &FormatError{Line: j + 2, Msg: "a map value must be a single line string"}
		}

		s, err := parseScalar(rest, j+2)
		if err != nil {
			return Value{}, 0, err
		}

		v.Map[k] = s
	}

	return v, j, nil
}

func parseBlockScalar(lines []string, i int, header string) (Value, int, error) {
	lineNo := i + 2
	folded := header[0] == '>'

	if strings.Trim(header[1:], "+-0123456789 ") != "" && !strings.HasPrefix(strings.TrimSpace(header[1:]), "#") {
		return Value{}, 0, &FormatError{Line: lineNo, Msg: "unsupported block scalar header"}
	}

	j := i + 1
	indent := -1

	var out []string

	for ; j < len(lines); j++ {
		line := lines[j]
		if strings.TrimSpace(line) == "" {
			out = append(out, "")

			continue
		}

		lead := len(line) - len(strings.TrimLeft(line, " \t"))
		if lead == 0 {
			break
		}

		if indent < 0 {
			indent = lead
		}

		if lead < indent {
			break
		}

		out = append(out, line[indent:])
	}

	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}

	if !folded {
		return Value{Kind: ScalarValue, Str: strings.Join(out, "\n")}, j, nil
	}

	var b strings.Builder

	for k, l := range out {
		switch {
		case k == 0:
		case l == "":
			b.WriteByte('\n')

			continue
		case out[k-1] == "":
		default:
			b.WriteByte(' ')
		}

		b.WriteString(l)
	}

	return Value{Kind: ScalarValue, Str: b.String()}, j, nil
}

// parseFlowList reads "[a, 'b', "c"]" on one line.
func parseFlowList(s string, lineNo int) ([]string, error) {
	if !strings.HasSuffix(s, "]") {
		return nil, &FormatError{Line: lineNo, Msg: "a flow list must close on the same line"}
	}

	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return nil, nil
	}

	var (
		items []string
		cur   strings.Builder
		quote rune
		esc   bool
	)

	flush := func() error {
		item, err := parseScalar(strings.TrimSpace(cur.String()), lineNo)
		if err != nil {
			return err
		}

		items = append(items, item)
		cur.Reset()

		return nil
	}

	for _, r := range inner {
		switch {
		case quote != 0:
			cur.WriteRune(r)

			switch {
			case esc:
				esc = false
			case r == '\\' && quote == '"':
				esc = true
			case r == quote:
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r

			cur.WriteRune(r)
		case r == ',':
			if err := flush(); err != nil {
				return nil, err
			}
		case r == '[' || r == '{':
			return nil, &FormatError{Line: lineNo, Msg: "a list inside a list is not supported"}
		default:
			cur.WriteRune(r)
		}
	}

	if quote != 0 {
		return nil, &FormatError{Line: lineNo, Msg: "a quote is not closed"}
	}

	if strings.TrimSpace(cur.String()) != "" {
		if err := flush(); err != nil {
			return nil, err
		}
	}

	return items, nil
}

// parseScalar reads a plain, double quoted or single quoted scalar.
func parseScalar(s string, lineNo int) (string, error) {
	if s == "" {
		return "", nil
	}

	switch s[0] {
	case '&', '*', '!', '%', '@', '`':
		return "", &FormatError{Line: lineNo, Msg: "anchors, aliases, tags and directives are not supported"}
	case '"':
		return parseDouble(s, lineNo)
	case '\'':
		return parseSingle(s, lineNo)
	case '|', '>', '[', '{':
		return "", &FormatError{Line: lineNo, Msg: "this value must be a plain or quoted string"}
	}

	if i := strings.Index(s, " #"); i >= 0 {
		s = s[:i]
	}

	return strings.TrimSpace(s), nil
}

func parseDouble(s string, lineNo int) (string, error) {
	var b strings.Builder

	rs := []rune(s)
	for i := 1; i < len(rs); i++ {
		switch r := rs[i]; r {
		case '\\':
			i++
			if i >= len(rs) {
				return "", &FormatError{Line: lineNo, Msg: "a quoted string ends with a backslash"}
			}

			switch rs[i] {
			case '"', '\\', '/':
				b.WriteRune(rs[i])
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				return "", &FormatError{Line: lineNo, Msg: `unsupported escape \` + string(rs[i])}
			}
		case '"':
			if tail := strings.TrimSpace(string(rs[i+1:])); tail != "" && !strings.HasPrefix(tail, "#") {
				return "", &FormatError{Line: lineNo, Msg: "text after a closing quote"}
			}

			return b.String(), nil
		default:
			b.WriteRune(r)
		}
	}

	return "", &FormatError{Line: lineNo, Msg: "a quoted string is not closed"}
}

func parseSingle(s string, lineNo int) (string, error) {
	var b strings.Builder

	rs := []rune(s)
	for i := 1; i < len(rs); i++ {
		if rs[i] != '\'' {
			b.WriteRune(rs[i])

			continue
		}

		if i+1 < len(rs) && rs[i+1] == '\'' {
			b.WriteByte('\'')

			i++

			continue
		}

		if tail := strings.TrimSpace(string(rs[i+1:])); tail != "" && !strings.HasPrefix(tail, "#") {
			return "", &FormatError{Line: lineNo, Msg: "text after a closing quote"}
		}

		return b.String(), nil
	}

	return "", &FormatError{Line: lineNo, Msg: "a quoted string is not closed"}
}

// Render writes the front matter without its fences. Maps are written with
// their keys sorted, so the output depends on the values only.
func (f *FrontMatter) Render() (string, error) {
	var b strings.Builder

	for _, k := range f.keys {
		v := f.vals[k]

		switch v.Kind {
		case ScalarValue:
			s, err := renderScalar(v.Str, v.Plain)
			if err != nil {
				return "", fmt.Errorf("%s: %w", k, err)
			}

			b.WriteString(k + ": " + s + "\n")
		case ListValue:
			parts := make([]string, len(v.List))

			for i, item := range v.List {
				s, err := renderScalar(item, false)
				if err != nil {
					return "", fmt.Errorf("%s: %w", k, err)
				}

				parts[i] = s
			}

			b.WriteString(k + ": [" + strings.Join(parts, ", ") + "]\n")
		case MapValue:
			b.WriteString(k + ":\n")

			keys := make([]string, 0, len(v.Map))
			for mk := range v.Map {
				keys = append(keys, mk)
			}

			sort.Strings(keys)

			for _, mk := range keys {
				if !mapKey.MatchString(mk) {
					return "", fmt.Errorf("%s: the key %q is not a valid map key", k, command.SafeKey(mk))
				}

				s, err := renderScalar(v.Map[mk], false)
				if err != nil {
					return "", fmt.Errorf("%s.%s: %w", k, command.SafeKey(mk), err)
				}

				b.WriteString("  " + mk + ": " + s + "\n")
			}
		}
	}

	if b.Len() > MaxFrontMatter {
		return "", fmt.Errorf("the front matter is longer than %d bytes", MaxFrontMatter)
	}

	return b.String(), nil
}

var plainSafe = regexp.MustCompile(`^[A-Za-z0-9_./][A-Za-z0-9_ ./+()-]*$`)

// yamlWords are plain strings that another YAML reader would take for something
// other than a string.
var yamlWords = map[string]bool{
	"true": true, "false": true, "yes": true, "no": true, "on": true, "off": true, "null": true, "y": true, "n": true,
}

var numberLike = regexp.MustCompile(`^[-+]?\d[0-9_.eE+-]*$`)

func renderScalar(s string, plain bool) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("the text is not valid UTF-8")
	}

	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return "", errors.New("the text has a control character")
		}
	}

	if s != "" && plainSafe.MatchString(s) && !strings.HasSuffix(s, " ") &&
		(plain || !yamlWords[strings.ToLower(s)] && !numberLike.MatchString(s)) {
		return s, nil
	}

	var b strings.Builder

	b.WriteByte('"')

	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}

	b.WriteByte('"')

	return b.String(), nil
}

// Compose writes a whole file: the front matter between its fences, then the
// body.
func Compose(f *FrontMatter, body string) (string, error) {
	front, err := f.Render()
	if err != nil {
		return "", err
	}

	return "---\n" + front + "---\n" + body, nil
}

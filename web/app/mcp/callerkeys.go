package mcp

import (
	"bytes"
	"cmp"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// safeKey is the shape of an argument key that an error may repeat: it cannot break a line, close a quote, reorder the
// text or fill the error block.
var safeKey = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// withoutCallerKeys takes out of an error every key of the arguments that is not safeKey. The keys are chosen by the
// caller, the error text goes to a model, and many layers repeat a key they refuse (the arguments of an operation, a
// style, a slot, a briefing answer): this is the one place all of them pass through, so a new one cannot forge a
// "hint:" line (SDD-003 section 1.2 rule 9).
func withoutCallerKeys(err error, raw json.RawMessage) error {
	r := newKeyRedactor(raw)
	if r == nil {
		return err
	}

	out := *asServiceError(err)
	out.Message, out.Hint, out.Path = r.Replace(out.Message), r.Replace(out.Hint), r.Replace(out.Path)

	if out.Details != nil {
		out.Details = redactDetails(out.Details, r)
	}

	return &out
}

// keyRedactor replaces the unsafe keys of one call.
type keyRedactor struct {
	exact *strings.Replacer
	// forms are the keys as a message writes them, plain and quoted: what a message cut inside a key ends with.
	forms []string
}

// cutMark is what command.Shortened leaves where it cut a text.
var cutMark = regexp.MustCompile(`\.\.\. \(\d+ characters\)`)

// newKeyRedactor is nil when the arguments have no unsafe key.
func newKeyRedactor(raw json.RawMessage) *keyRedactor {
	found := map[string]bool{}
	// A syntax error stops the walk; what it read before is still taken out.
	_ = collectKeys(json.NewDecoder(bytes.NewReader(raw)), found)

	keys := make([]string, 0, len(found))
	forms := make([]string, 0, 2*len(found))

	for k := range found {
		if k == "" || safeKey.MatchString(k) {
			continue
		}

		keys = append(keys, k)
		forms = append(forms, k, strconv.Quote(k))

		// A message repeats a long key cut, with the mark of the cut.
		if short := command.Shortened(k); short != k {
			keys = append(keys, short)
		}
	}

	if len(keys) == 0 {
		return nil
	}

	// The longest first, so that a key inside another does not leave the rest of the longer one behind.
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), cmp.Compare(a, b))
	})

	pairs := make([]string, 0, 4*len(keys))

	for _, k := range keys {
		pairs = append(pairs, strconv.Quote(k), `"?"`)
	}

	for _, k := range keys {
		pairs = append(pairs, k, "?")
	}

	return &keyRedactor{exact: strings.NewReplacer(pairs...), forms: forms}
}

// Replace takes the keys out of s, whole or cut: a message that holds a key can itself be shortened inside the key.
func (k *keyRedactor) Replace(s string) string {
	s = k.exact.Replace(s)

	locs := cutMark.FindAllStringIndex(s, -1)
	for i := len(locs) - 1; i >= 0; i-- {
		start, end := locs[i][0], locs[i][1]
		if n := k.keyTail(s[:start]); n > 0 {
			s = s[:start-n] + "?" + s[end:]
		}
	}

	return s
}

// keyTail is the length of the longest start of a key that text ends with.
func (k *keyRedactor) keyTail(text string) int {
	best := 0

	for _, f := range k.forms {
		for n := min(len(f), len(text)); n > best; n-- {
			if strings.HasSuffix(text, f[:n]) {
				best = n

				break
			}
		}
	}

	return best
}

// collectKeys reads every object key of one JSON value from the token stream, so that the key of a member repeated in
// the same object is seen too.
func collectKeys(dec *json.Decoder, into map[string]bool) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}

	open, ok := t.(json.Delim)
	if !ok {
		return nil
	}

	for dec.More() {
		if open == '{' {
			k, kerr := dec.Token()
			if kerr != nil {
				return kerr
			}

			if s, isString := k.(string); isString {
				into[s] = true
			}
		}

		if err := collectKeys(dec, into); err != nil {
			return err
		}
	}

	_, err = dec.Token()

	return err
}

// redactDetails rewrites the strings of the details through their JSON form, which is what the envelope prints. Details
// that cannot be read back are dropped rather than sent as they are.
func redactDetails(details map[string]any, r *keyRedactor) map[string]any {
	raw, err := json.Marshal(details)
	if err != nil {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var doc map[string]any
	if dec.Decode(&doc) != nil {
		return nil
	}

	out, _ := redactValue(doc, r).(map[string]any)

	return out
}

func redactValue(v any, r *keyRedactor) any {
	switch x := v.(type) {
	case string:
		return r.Replace(x)
	case []any:
		for i := range x {
			x[i] = redactValue(x[i], r)
		}

		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[r.Replace(k)] = redactValue(e, r)
		}

		return out
	default:
		return v
	}
}

package logging

import (
	"log/slog"
	"strings"
)

// replacement stands for every character that could forge or hide a log line.
const replacement = '?'

// unsafe is true for the control characters (U+0000 to U+001F, U+007F to
// U+009F, line feed and tab included), the bidirectional controls (U+202A to
// U+202E, U+2066 to U+2069, U+200E, U+200F, U+061C) and the line and paragraph
// separators U+2028 and U+2029 (SDD-007 section 8.2, T-25).
func unsafe(r rune) bool {
	switch {
	case r < 0x20, r >= 0x7F && r <= 0x9F:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	}

	return r == 0x200E || r == 0x200F || r == 0x061C || r == 0x2028 || r == 0x2029
}

// Sanitize replaces each unsafe character of s by "?" and invalid UTF-8 by
// U+FFFD, so that the value is one line of printable text.
func Sanitize(s string) string {
	s = strings.ToValidUTF8(s, "�")

	if strings.IndexFunc(s, unsafe) < 0 {
		return s
	}

	return strings.Map(func(r rune) rune {
		if unsafe(r) {
			return replacement
		}

		return r
	}, s)
}

// sanitizeAttr cleans the strings of an attribute, groups and the values of
// errors and stringers included, which a handler would otherwise format as they
// are.
func sanitizeAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()

	switch v.Kind() {
	case slog.KindString:
		return slog.String(Sanitize(a.Key), Sanitize(v.String()))
	case slog.KindGroup:
		group := v.Group()
		clean := make([]slog.Attr, len(group))

		for i, g := range group {
			clean[i] = sanitizeAttr(g)
		}

		return slog.Attr{Key: Sanitize(a.Key), Value: slog.GroupValue(clean...)}
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return slog.String(Sanitize(a.Key), Sanitize(err.Error()))
		}

		if s, ok := v.Any().(interface{ String() string }); ok {
			return slog.String(Sanitize(a.Key), Sanitize(s.String()))
		}
	case slog.KindBool, slog.KindDuration, slog.KindFloat64, slog.KindInt64, slog.KindTime, slog.KindUint64,
		slog.KindLogValuer:
	}

	return slog.Attr{Key: Sanitize(a.Key), Value: v}
}

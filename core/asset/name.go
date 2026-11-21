package asset

import (
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxName is the longest display name of a file, in characters (SDD-011 section 14.8).
const MaxName = 120

// RenameTo is the display name a file takes when a person asks for requested, given the name it has now. The name
// is trimmed; it cannot be empty, hold a path separator or a control character; and it keeps the extension of the
// current name: a name that does not end with it (compared without regard to case) gets it appended, so a person
// edits the part before the dot. The result is at most MaxName characters. A refusal is a *RuleError with the
// reason name_invalid.
func RenameTo(current, requested string) (string, *RuleError) {
	name := strings.TrimSpace(requested)
	ext := path.Ext(current)

	refuse := func(why string) (string, *RuleError) {
		return "", &RuleError{Path: "name", Reason: "name_invalid", Detail: why}
	}

	if name == "" {
		return refuse("the name is empty")
	}

	for _, r := range name {
		switch {
		case r == '/' || r == '\\':
			return refuse("the name has a path separator")
		case unicode.IsControl(r):
			return refuse("the name has a control character")
		}
	}

	if ext != "" && !strings.HasSuffix(strings.ToLower(name), strings.ToLower(ext)) {
		name += ext
	}

	if stem := strings.TrimSuffix(strings.ToLower(name), strings.ToLower(ext)); strings.TrimSpace(stem) == "" {
		return refuse("the name has nothing before the extension")
	}

	if n := utf8.RuneCountInString(name); n > MaxName {
		return refuse(fmt.Sprintf("the name has %d characters; at most %d", n, MaxName))
	}

	return name, nil
}

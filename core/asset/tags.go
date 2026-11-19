package asset

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The limits of the tags of one asset (SDD-011 section 14.3).
const (
	MaxTags      = 20
	MaxTagLength = 40
)

// NormalizeTag turns what a person typed into the form that is stored and searched: trimmed, lower case,
// a leading # removed, runs of white space one hyphen. ok is false when nothing valid is left: empty, too long,
// or a character other than a letter, a digit, a hyphen or an underscore.
//
// The text arrives already in NFC: core/ imports only the standard library (ADR-0001), so the composition is
// done by the caller (service/assets). A decomposed accent is a combining mark, not a letter, and is refused.
func NormalizeTag(s string) (tag string, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "#")
	s = strings.ToLower(strings.Join(strings.Fields(s), "-"))

	if s == "" || utf8.RuneCountInString(s) > MaxTagLength {
		return "", false
	}

	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return "", false
		}
	}

	return s, true
}

// NormalizeTags normalizes a list: invalid is the first entry that cannot be a tag, and the result is sorted
// with duplicates kept once.
func NormalizeTags(in []string) (tags []string, bad *RuleError) {
	for _, t := range in {
		n, ok := NormalizeTag(t)
		if !ok {
			return nil, &RuleError{
				Path: "tags", Reason: "tags_invalid",
				Detail: fmt.Sprintf(
					"%q is not a tag; use 1 to %d letters, digits, hyphens or underscores", t, MaxTagLength,
				),
			}
		}

		tags = append(tags, n)
	}

	slices.Sort(tags)

	return slices.Compact(tags), nil
}

// ApplyTags adds and removes tags from a set that is already normalized. The result is sorted, and a result of
// more than MaxTags is refused.
func ApplyTags(have, add, remove []string) ([]string, *RuleError) {
	added, bad := NormalizeTags(add)
	if bad != nil {
		return nil, bad
	}

	removed, bad := NormalizeTags(remove)
	if bad != nil {
		return nil, bad
	}

	out := slices.Clone(have)
	out = append(out, added...)
	out = slices.DeleteFunc(out, func(t string) bool { return slices.Contains(removed, t) })

	slices.Sort(out)
	out = slices.Compact(out)

	if len(out) > MaxTags {
		return nil, &RuleError{
			Path: "tags", Reason: "tags_invalid",
			Detail: fmt.Sprintf("an asset holds at most %d tags; this would be %d", MaxTags, len(out)),
		}
	}

	return out, nil
}

package asset

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Media kinds of the assets table that the library lists.
const (
	KindImage = "image"
	KindVideo = "video"
	KindAudio = "audio"
	KindLUT   = "lut"
)

// The three sections over the library (SDD-011 section 14.6).
const (
	SectionVisual = "visual"
	SectionVideo  = "video"
	SectionAudio  = "audio"
)

// SectionKinds are the media kinds of a section; nil for a name that is not a section.
func SectionKinds(section string) []string {
	switch section {
	case SectionVisual:
		return []string{KindImage, KindLUT}
	case SectionVideo:
		return []string{KindVideo}
	case SectionAudio:
		return []string{KindAudio}
	}

	return nil
}

// Entry is what a filter looks at: one file of the library as the service assembled it.
type Entry struct {
	MediaKind string
	Origin    Origin
	Reusable  bool
	Tags      []string
	// Projects are the identifiers of the productions and of the timelines that use the file.
	Projects []string
}

// Filter picks files out of the library. A zero member does not filter.
type Filter struct {
	Kind     string // image, video, audio or lut
	Section  string // visual, video or audio
	Tag      string
	Project  string // a production or a timeline identifier
	Origin   OriginKind
	Reusable *bool
}

// Validate normalizes the tag and refuses a name that is not known.
func (f *Filter) Validate() *RuleError {
	if f.Kind != "" && !slices.Contains([]string{KindImage, KindVideo, KindAudio, KindLUT}, f.Kind) {
		return &RuleError{
			Path:   "kind",
			Reason: "kind_unknown",
			Detail: fmt.Sprintf("the kind %q is not known; use image, video, audio or lut", f.Kind),
		}
	}

	if f.Section != "" && SectionKinds(f.Section) == nil {
		return &RuleError{
			Path:   "section",
			Reason: "section_unknown",
			Detail: fmt.Sprintf("the section %q is not known; use visual, video or audio", f.Section),
		}
	}

	if f.Origin != "" && !f.Origin.Valid() {
		return &RuleError{
			Path:   "origin",
			Reason: "origin_kind_unknown",
			Detail: fmt.Sprintf("the origin kind %q is not known", f.Origin),
		}
	}

	if f.Tag != "" {
		t, ok := NormalizeTag(f.Tag)
		if !ok {
			return &RuleError{Path: "tag", Reason: "tags_invalid", Detail: fmt.Sprintf("%q is not a tag", f.Tag)}
		}

		f.Tag = t
	}

	return nil
}

// Match reports whether the entry passes every member that is set. Validate has already seen the filter.
func (f *Filter) Match(e *Entry) bool {
	if f.Kind != "" && e.MediaKind != f.Kind {
		return false
	}

	if f.Section != "" && !slices.Contains(SectionKinds(f.Section), e.MediaKind) {
		return false
	}

	if f.Tag != "" && !slices.Contains(e.Tags, f.Tag) {
		return false
	}

	if f.Project != "" && !slices.Contains(e.Projects, f.Project) {
		return false
	}

	if f.Origin != "" && e.Origin.Kind != f.Origin {
		return false
	}

	if f.Reusable != nil && e.Reusable != *f.Reusable {
		return false
	}

	return true
}

// The limits of a full-text query.
const (
	MaxTerms      = 8
	MaxTermLength = 64
)

// SearchQuery turns the words a person typed into the MATCH expression of the full-text index: each word a
// quoted prefix term, all of them required. Anything that is not a letter or a digit separates words, so no
// operator, column filter or quote can reach the index. ok is false when no word is left.
func SearchQuery(text string) (match string, ok bool) {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	if len(words) == 0 {
		return "", false
	}

	if len(words) > MaxTerms {
		words = words[:MaxTerms]
	}

	terms := make([]string, len(words))

	for i, w := range words {
		if r := []rune(w); len(r) > MaxTermLength {
			w = string(r[:MaxTermLength])
		}

		terms[i] = `"` + w + `"*`
	}

	return strings.Join(terms, " "), true
}

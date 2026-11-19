// Package publish is the pure rule set of a publication (ADR-0032, ADR-0035, SDD-020): what a post is,
// what a network allows, and the states an upload goes through. It does no I/O: the service gathers the
// facts about the files (size and format) and a connector declares the limits of its network.
package publish

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Visibility is who may watch a published video.
type Visibility string

// The visibilities of a post. Private is the default: a publication goes out hidden unless the person
// said otherwise.
const (
	Private  Visibility = "private"
	Unlisted Visibility = "unlisted"
	Public   Visibility = "public"
)

// Network names.
const (
	NetworkYouTube = "youtube"
)

// File is what the rules need to know of a file: its size and its format (the extension without a dot).
type File struct {
	Bytes  int64  `json:"bytes"`
	Format string `json:"format"`
}

// Post is one publication to one network (SDD-020 section 3).
type Post struct {
	// Output is the exported video.
	Output File `json:"output"`
	// Thumbnail is the exported still, optional.
	Thumbnail *File `json:"thumbnail,omitempty"`

	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Visibility  Visibility `json:"visibility,omitempty"`
	// MadeForKids must be set: the person declares it, nothing is guessed.
	MadeForKids *bool `json:"made_for_kids"`
	// AIDisclosure says the video has realistic altered or synthetic content.
	AIDisclosure bool `json:"ai_disclosure,omitempty"`
}

// Limits are what a network allows, with where and when that was read.
type Limits struct {
	TitleMax            int          `json:"title_max"`
	DescriptionMaxBytes int          `json:"description_max_bytes"`
	TagsMaxChars        int          `json:"tags_max_chars"`
	FileMaxBytes        int64        `json:"file_max_bytes"`
	ThumbnailMaxBytes   int64        `json:"thumbnail_max_bytes"`
	Formats             []string     `json:"formats"`
	ThumbnailFormats    []string     `json:"thumbnail_formats"`
	Visibilities        []Visibility `json:"visibilities"`
	// NeedsKidsDeclaration is true when the network asks whether the video is made for kids.
	NeedsKidsDeclaration bool `json:"needs_kids_declaration"`
	// Forbidden lists characters a title and a description may not hold.
	Forbidden string `json:"forbidden,omitempty"`
	// Source is the official page the limits come from and Read the date it was read.
	Source string `json:"source"`
	Read   string `json:"read"`
}

// Issue codes.
const (
	CodeRequired      = "required"
	CodeTooLong       = "too_long"
	CodeForbiddenChar = "forbidden_character"
	CodeUnsupported   = "unsupported_format"
	CodeTooLarge      = "too_large"
	CodeNotAllowed    = "not_allowed"
	CodeInvalid       = "invalid"
)

// Issue is one problem of a post: where it is, a stable code and a message for a person.
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// EffectiveVisibility is the visibility the post will have: private when it says none.
func (p *Post) EffectiveVisibility() Visibility {
	if p.Visibility == "" {
		return Private
	}

	return p.Visibility
}

// TagChars counts the characters the tags take on YouTube: the tags and the commas between them, and a
// tag that holds a space counts as if it were wrapped in quotation marks (SDD-020 section 2).
func TagChars(tags []string) int {
	n := 0

	for i, t := range tags {
		n += utf8.RuneCountInString(t)

		if strings.ContainsRune(t, ' ') {
			n += 2
		}

		if i > 0 {
			n++
		}
	}

	return n
}

type adder func(path, code, format string, args ...any)

// Validate checks a post against the limits of a network and returns every problem at once, in a fixed
// order. An empty result means the post may be submitted.
func Validate(p Post, l Limits) []Issue {
	var out []Issue

	add := func(path, code, format string, args ...any) {
		out = append(out, Issue{Path: path, Code: code, Message: fmt.Sprintf(format, args...)})
	}

	title := strings.TrimSpace(p.Title)

	switch {
	case title == "":
		add("title", CodeRequired, "the title is required.")
	case utf8.RuneCountInString(title) > l.TitleMax:
		add("title", CodeTooLong, "the title has more than %d characters.", l.TitleMax)
	default:
		textIssues("title", title, l.Forbidden, add)
	}

	if len(p.Description) > l.DescriptionMaxBytes {
		add("description", CodeTooLong, "the description has more than %d bytes.", l.DescriptionMaxBytes)
	} else {
		textIssues("description", p.Description, l.Forbidden, add)
	}

	if TagChars(p.Tags) > l.TagsMaxChars {
		add("tags", CodeTooLong, "the tags take more than %d characters, commas included.", l.TagsMaxChars)
	}

	for i, t := range p.Tags {
		if strings.TrimSpace(t) == "" || strings.ContainsFunc(t, unicode.IsControl) {
			add(fmt.Sprintf("tags[%d]", i), CodeInvalid, "a tag is empty or has a control character.")
		}
	}

	if v := p.EffectiveVisibility(); !slices.Contains(l.Visibilities, v) {
		add(
			"visibility",
			CodeNotAllowed,
			"the visibility %q is not one of %s.",
			string(v),
			visibilityList(l.Visibilities),
		)
	}

	if l.NeedsKidsDeclaration && p.MadeForKids == nil {
		add(
			"made_for_kids",
			CodeRequired,
			"say whether the video is made for kids: the network asks the channel owner.",
		)
	}

	fileIssues("output", p.Output, l.Formats, l.FileMaxBytes, add)

	if p.Thumbnail != nil {
		fileIssues("thumbnail", *p.Thumbnail, l.ThumbnailFormats, l.ThumbnailMaxBytes, add)
	}

	return out
}

func textIssues(path, text, forbidden string, add adder) {
	if !utf8.ValidString(text) {
		add(path, CodeInvalid, "the %s is not valid UTF-8.", path)

		return
	}

	if strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) {
		add(path, CodeInvalid, "the %s has a control character.", path)
	}

	if forbidden != "" && strings.ContainsAny(text, forbidden) {
		add(path, CodeForbiddenChar, "the %s may not hold any of %q.", path, forbidden)
	}
}

func fileIssues(path string, f File, formats []string, maxBytes int64, add adder) {
	switch {
	case f.Bytes <= 0:
		add(path, CodeRequired, "the %s file is empty or missing.", path)
	case f.Bytes > maxBytes:
		add(path, CodeTooLarge, "the %s file is larger than %d bytes.", path, maxBytes)
	}

	if !slices.Contains(formats, strings.ToLower(f.Format)) {
		add(path, CodeUnsupported, "the %s format %q is not one of %s.", path, f.Format, strings.Join(formats, ", "))
	}
}

func visibilityList(v []Visibility) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = string(x)
	}

	return strings.Join(parts, ", ")
}

// State is the state of a publication (SDD-020 section 4).
type State string

// The states of a publication.
const (
	StatePrepared        State = "prepared"
	StateWaitingApproval State = "waiting_approval"
	StateUploading       State = "uploading"
	StateProcessing      State = "processing"
	StatePublished       State = "published"
	StateFailed          State = "failed"
)

var moves = map[State][]State{
	StatePrepared:        {StateWaitingApproval},
	StateWaitingApproval: {StateUploading, StateFailed},
	StateUploading:       {StateProcessing, StatePublished, StateFailed},
	StateProcessing:      {StatePublished, StateFailed},
}

// Can says whether a publication may move from one state to another. A final state moves nowhere.
func Can(from, to State) bool { return slices.Contains(moves[from], to) }

// Final says whether a state is the end of a publication.
func (s State) Final() bool { return s == StatePublished || s == StateFailed }

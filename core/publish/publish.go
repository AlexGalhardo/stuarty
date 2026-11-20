// Package publish is the pure rule set of a publication (ADR-0032, ADR-0035, SDD-020): what a post is,
// what a network allows, and the states an upload goes through. It does no I/O: the service gathers the
// facts about the files (size and format) and a connector declares the limits of its network.
package publish

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/core/writing"
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
	NetworkYouTube   = "youtube"
	NetworkInstagram = "instagram"
	NetworkFacebook  = "facebook"
	NetworkX         = "x"
	NetworkTikTok    = "tiktok"
)

// Invisible says whether r is a character that shows nothing or changes the direction of the text around
// it: the zero-width space and joiner marks, the byte order mark, the bidirectional controls and the tag
// characters. A title with one can read as another word to the person who approves it (SDD-020 section 3);
// the zero-width joiner and non-joiner stay, since emoji and several scripts need them.
func Invisible(r rune) bool {
	switch r {
	case 0x200B, 0x2060, 0xFEFF, 0x061C, 0x200E, 0x200F:
		return true
	}

	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || (r >= 0xE0000 && r <= 0xE007F)
}

// File is what the rules need to know of a file: its size and its format (the extension without a dot).
type File struct {
	Bytes  int64  `json:"bytes"`
	Format string `json:"format"`
	// Media is what the probe measured on a video file; nil for a file that is not one, or when it is not known.
	Media *Media `json:"media,omitempty"`
}

// Media is the properties of a video file, measured on the written file by the export (SDD-020 section 18).
type Media struct {
	Width      int   `json:"width"`
	Height     int   `json:"height"`
	FPSNum     int   `json:"fps_num"`
	FPSDen     int   `json:"fps_den"`
	DurationMS int64 `json:"duration_ms"`
	// VideoCodec and AudioCodec are the names of the probe (h264, hevc, aac); AudioCodec is empty for a video
	// with no sound.
	VideoCodec string `json:"video_codec"`
	AudioCodec string `json:"audio_codec,omitempty"`
}

// ParseFPS reads a frame rate the way an export records it: "30" or "30000/1001". It returns 0, 0 for any
// other text.
func ParseFPS(s string) (num, den int) {
	n, d, found := strings.Cut(strings.TrimSpace(s), "/")
	if !found {
		d = "1"
	}

	num, err := strconv.Atoi(n)
	if err != nil || num <= 0 {
		return 0, 0
	}

	den, err = strconv.Atoi(d)
	if err != nil || den <= 0 {
		return 0, 0
	}

	return num, den
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
	// Preset is the id of the export preset of core/export whose limits (length, frame rate, size, picture)
	// the video must fit; empty for a network that has no such limits. The limits are written once, there.
	Preset string `json:"preset,omitempty"`
	// VideoCodecs and AudioCodecs are the codecs the network takes, by the names of the probe; empty is any.
	VideoCodecs []string `json:"video_codecs,omitempty"`
	AudioCodecs []string `json:"audio_codecs,omitempty"`
	// CaptionMax is the most characters of the caption (Caption); HashtagsMax the most hashtags in it. Zero
	// is no limit.
	CaptionMax  int `json:"caption_max,omitempty"`
	HashtagsMax int `json:"hashtags_max,omitempty"`
	// DailyPosts is how many posts the network lets the API publish in a moving 24 hours; zero is not stated.
	DailyPosts int `json:"daily_posts,omitempty"`
	// CaptionNetwork is the id of the network in the table of limits of SDD-019 whose count (the weighted
	// length of X, a link weighing 23) decides the length of the caption; empty for the plain counts above.
	CaptionNetwork string `json:"caption_network,omitempty"`
	// NoCaption is true when the network takes no text with the file (the draft of TikTok): the title is only
	// the name the post has in Stuarty, and its description and tags are not sent, so no limit is checked on them.
	NoCaption bool `json:"no_caption,omitempty"`
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

	if !l.NoCaption {
		textFieldIssues(&p, &l, add)
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
	mediaIssues(p.Output, &l, add)
	captionIssues(&p, &l, add)

	if p.Thumbnail != nil {
		if len(l.ThumbnailFormats) == 0 {
			add("thumbnail", CodeNotAllowed, "this network takes no separate thumbnail.")
		} else {
			fileIssues("thumbnail", *p.Thumbnail, l.ThumbnailFormats, l.ThumbnailMaxBytes, add)
		}
	}

	return out
}

// textFieldIssues checks the description and the tags, the text a network sends with the video.
func textFieldIssues(p *Post, l *Limits, add adder) {
	if len(p.Description) > l.DescriptionMaxBytes {
		add("description", CodeTooLong, "the description has more than %d bytes.", l.DescriptionMaxBytes)
	} else {
		textIssues("description", p.Description, l.Forbidden, add)
	}

	if TagChars(p.Tags) > l.TagsMaxChars {
		add("tags", CodeTooLong, "the tags take more than %d characters, commas included.", l.TagsMaxChars)
	}

	for i, t := range p.Tags {
		if strings.TrimSpace(t) == "" || strings.ContainsFunc(t, unicode.IsControl) ||
			strings.ContainsFunc(t, Invisible) {
			add(fmt.Sprintf("tags[%d]", i), CodeInvalid, "a tag is empty or has a control character.")
		}
	}
}

// Caption is the text a network that has no title shows with a video: the title, the description and the
// tags as hashtags, each part on its own paragraph. Spaces inside a tag are removed, since a hashtag is one
// word.
func Caption(p *Post) string {
	var parts []string

	if t := strings.TrimSpace(p.Title); t != "" {
		parts = append(parts, t)
	}

	if d := strings.TrimSpace(p.Description); d != "" {
		parts = append(parts, d)
	}

	var tags []string

	for _, t := range p.Tags {
		if w := strings.Join(strings.Fields(t), ""); w != "" {
			tags = append(tags, "#"+strings.TrimPrefix(w, "#"))
		}
	}

	if len(tags) > 0 {
		parts = append(parts, strings.Join(tags, " "))
	}

	return strings.Join(parts, "\n\n")
}

// Hashtags counts the hashtags of a text: a # followed by a letter, a digit or an underscore.
func Hashtags(text string) int {
	n, rs := 0, []rune(text)

	for i, r := range rs {
		if r == '#' && i+1 < len(rs) && (unicode.IsLetter(rs[i+1]) || unicode.IsDigit(rs[i+1]) || rs[i+1] == '_') {
			n++
		}
	}

	return n
}

func captionIssues(p *Post, l *Limits, add adder) {
	if l.CaptionNetwork != "" && !l.NoCaption {
		for _, pr := range writing.ValidateCaption(Caption(p), l.CaptionNetwork).Problems {
			if pr.Code == writing.ProblemTooLong {
				add(
					"description",
					CodeTooLong,
					"the caption (the title, the description and the tags as hashtags): %s.",
					pr.Message,
				)
			}
		}
	}

	if l.CaptionMax == 0 && l.HashtagsMax == 0 {
		return
	}

	caption := Caption(p)

	if l.CaptionMax > 0 {
		if n := utf8.RuneCountInString(caption); n > l.CaptionMax {
			add(
				"description",
				CodeTooLong,
				"the caption (the title, the description and the tags as hashtags) has %d characters; the network allows %d.",
				n,
				l.CaptionMax,
			)
		}
	}

	if l.HashtagsMax > 0 {
		if n := Hashtags(caption); n > l.HashtagsMax {
			add("tags", CodeTooLong, "the caption has %d hashtags; the network allows %d.", n, l.HashtagsMax)
		}
	}
}

// findingWhat names what a finding of core/export is about, for the sentence of the issue.
var findingWhat = map[string]string{
	export.CodeDurationTooLong:    "length",
	export.CodeDurationTooShort:   "length",
	export.CodeFPSTooHigh:         "frame rate",
	export.CodeFPSTooLow:          "frame rate",
	export.CodeResolutionTooSmall: "size in pixels",
	export.CodeResolutionTooLarge: "size in pixels",
	export.CodeAspectRatio:        "aspect ratio",
}

// mediaIssues checks the properties of the video against the preset of the network and the codecs it takes.
// The size of the file is the business of FileMaxBytes, so that finding of the preset is not repeated.
func mediaIssues(f File, l *Limits, add adder) {
	if l.Preset == "" && len(l.VideoCodecs) == 0 && len(l.AudioCodecs) == 0 {
		return
	}

	m := f.Media
	if m == nil || m.Width <= 0 || m.Height <= 0 || m.DurationMS <= 0 {
		add(
			"output", CodeRequired,
			"the properties of the video (length, size, frame rate and codecs) are not known: export it again.",
		)

		return
	}

	if l.Preset != "" {
		pr, ok := export.Lookup(l.Preset)
		if !ok {
			add("output", CodeInvalid, "the export preset %q of this network is not known.", l.Preset)

			return
		}

		for _, fd := range pr.Check(export.Input{
			Width: m.Width, Height: m.Height, FPSNum: m.FPSNum, FPSDen: m.FPSDen, DurationMS: m.DurationMS,
			Bytes: f.Bytes,
		}) {
			if fd.Code == export.CodeSizeTooLarge {
				continue
			}

			add(
				"output", strings.ToLower(strings.TrimPrefix(fd.Code, "X_")),
				"the video's %s does not fit %s: the limit is %s and the file has %s.",
				findingWhat[fd.Code], pr.Label, fd.Limit, fd.Got,
			)
		}
	}

	codecIssues("video", m.VideoCodec, l.VideoCodecs, add)

	if m.AudioCodec != "" {
		codecIssues("audio", m.AudioCodec, l.AudioCodecs, add)
	}
}

func codecIssues(what, codec string, allowed []string, add adder) {
	if len(allowed) > 0 && !slices.Contains(allowed, strings.ToLower(codec)) {
		add("output", CodeUnsupported, "the %s codec %q is not one of %s.", what, codec, strings.Join(allowed, ", "))
	}
}

func textIssues(path, text, forbidden string, add adder) {
	if !utf8.ValidString(text) {
		add(path, CodeInvalid, "the %s is not valid UTF-8.", path)

		return
	}

	if strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) {
		add(path, CodeInvalid, "the %s has a control character.", path)
	}

	if strings.ContainsFunc(text, Invisible) {
		add(path, CodeInvalid, "the %s has an invisible or direction-changing character.", path)
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
	// StateSentAsDraft is a file the network took as a draft in the person's own app, to be finished and
	// published there (TikTok, SDD-020 section 38). It is not published.
	StateSentAsDraft State = "sent_as_draft"
)

var moves = map[State][]State{
	StatePrepared:        {StateWaitingApproval},
	StateWaitingApproval: {StateUploading, StateFailed},
	StateUploading:       {StateProcessing, StatePublished, StateSentAsDraft, StateFailed},
	StateProcessing:      {StatePublished, StateSentAsDraft, StateFailed},
}

// Can says whether a publication may move from one state to another. A final state moves nowhere.
func Can(from, to State) bool { return slices.Contains(moves[from], to) }

// Final says whether a state is the end of a publication.
func (s State) Final() bool { return s == StatePublished || s == StateSentAsDraft || s == StateFailed }

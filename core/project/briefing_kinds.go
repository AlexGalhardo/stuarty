package project

import (
	"slices"
	"strings"
)

// ProductionKind is the kind of production a brief is for (SDD-018 section 17, epic 39). It selects the set of
// questions the briefing asks: the questions are data, one set per kind, so the form, the CLI, the HTTP API and an
// agent in a chat ask the ones that fit what is being made.
type ProductionKind string

// The kinds of production.
const (
	ProductionShort      ProductionKind = "short"      // a short vertical video (the first complete flow)
	ProductionCommercial ProductionKind = "commercial" // a product commercial of 15 or 30 seconds
	ProductionLong       ProductionKind = "long"       // a long talking video
)

// The bounds of the duration answer of a commercial and of a long video, in seconds.
const (
	MinCommercialSeconds = 5
	MaxCommercialSeconds = 90
	MinLongSeconds       = 120
	MaxLongSeconds       = 14400
)

// ProductionKinds returns the kinds of production, in a fixed order.
func ProductionKinds() []ProductionKind {
	return []ProductionKind{ProductionShort, ProductionCommercial, ProductionLong}
}

// KindOrShort is the kind a surface sends as text: an empty one is the short video, the default of the briefing.
// The text is not checked here; BriefingQuestionsFor and CheckAnswersFor refuse an unknown kind.
func KindOrShort(s string) ProductionKind {
	if s == "" {
		return ProductionShort
	}

	return ProductionKind(s)
}

// Known reports whether k is a kind of production.
func (k ProductionKind) Known() bool {
	return slices.Contains(ProductionKinds(), k)
}

var commercialQuestions = []Question{
	{
		ID: "product", Heading: "Product", Text: "What is the product, in one sentence?",
		Kind: AnswerText, Required: true, MaxChars: 400,
	},
	{ID: "audience", Heading: "Audience", Text: "Who is it for?", Kind: AnswerText, Required: true, MaxChars: 400},
	{
		ID: "problem", Heading: "Problem", Text: "Which problem of that person does it solve?",
		Kind: AnswerText, Required: true, MaxChars: 400,
	},
	{
		ID: "offer", Heading: "Offer", Text: "What is the offer: the price, a discount, a bonus, a deadline?",
		Kind: AnswerText, Required: true, MaxChars: 400,
	},
	{
		ID: "call_to_action", Heading: "Call to action", Text: "What should the viewer do next?",
		Kind: AnswerText, Required: true, MaxChars: 200,
	},
	{
		ID: "platform", Heading: "Platform", Text: "Where will it run?", Kind: AnswerChoice, Required: true,
		Options: []string{
			"youtube-shorts", "tiktok", "instagram-reels", "facebook-reels", "youtube", "instagram-feed",
			"facebook-feed", "other",
		},
	},
	{
		ID: "format", Heading: "Format", Text: "Which aspect ratio comes first?", Kind: AnswerChoice, Required: true,
		Options: []string{"vertical-9x16", "square-1x1", "horizontal-16x9"},
	},
	{
		ID: "duration", Heading: "Duration (seconds)", Text: "How long, in seconds (15 and 30 are the usual)?",
		Kind: AnswerSeconds, Required: true, Min: MinCommercialSeconds, Max: MaxCommercialSeconds,
	},
	{
		ID: "other_formats", Heading: "Other formats",
		Text: "Which other aspect ratios (9:16, 1:1, 16:9) must the same commercial come in?",
		Kind: AnswerText, MaxChars: 200,
	},
	{
		ID: "product_shots", Heading: "Product shots",
		Text: "Which shots of the product exist, and which must still be filmed?", Kind: AnswerText, MaxChars: 1000,
	},
	{
		ID: "proof", Heading: "Proof", Text: "What proof is there: reviews, numbers, a demonstration?",
		Kind: AnswerText, MaxChars: 1000,
	},
	{ID: "tone", Heading: "Tone", Text: "Which tone of voice?", Kind: AnswerText, MaxChars: 200},
	{
		ID: "references", Heading: "References", Text: "Any commercials to follow (links, brands)?",
		Kind: AnswerText, MaxChars: 2000,
	},
	{ID: "must_include", Heading: "Must appear", Text: "What must appear in it?", Kind: AnswerText, MaxChars: 1000},
	{
		ID: "must_avoid", Heading: "Must not appear", Text: "What must not appear in it (claims, words, competitors)?",
		Kind: AnswerText, MaxChars: 1000,
	},
	{ID: "deadline", Heading: "Deadline", Text: "By when (YYYY-MM-DD)?", Kind: AnswerDate},
}

var longQuestions = []Question{
	{ID: "topic", Heading: "Topic", Text: "What is the video about?", Kind: AnswerText, Required: true, MaxChars: 400},
	{
		ID: "audience", Heading: "Audience", Text: "Who is this video for?",
		Kind: AnswerText, Required: true, MaxChars: 400,
	},
	{
		ID: "goal", Heading: "Goal", Text: "What should the viewer know, do or feel after watching?",
		Kind: AnswerText, Required: true, MaxChars: 400,
	},
	{
		ID: "speakers", Heading: "Speakers", Text: "Who speaks, and who is on camera?",
		Kind: AnswerText, Required: true, MaxChars: 400,
	},
	{
		ID: "platform", Heading: "Platform", Text: "Where will it be published?", Kind: AnswerChoice, Required: true,
		Options: []string{"youtube", "vimeo", "linkedin", "facebook", "other"},
	},
	{
		ID: "format", Heading: "Format", Text: "Which format?", Kind: AnswerChoice, Required: true,
		Options: []string{"horizontal-16x9", "vertical-9x16", "square-1x1"},
	},
	{
		ID: "duration", Heading: "Duration (seconds)", Text: "How long should the edited video be, in seconds?",
		Kind: AnswerSeconds, Required: true, Min: MinLongSeconds, Max: MaxLongSeconds,
	},
	{
		ID: "cut_style", Heading: "Cut style", Text: "How tight should the cut be?", Kind: AnswerChoice, Required: true,
		Options: []string{"tight", "natural", "relaxed"},
	},
	{
		ID: "chapters", Heading: "Chapters", Text: "Which chapters or topics do you expect, in order?",
		Kind: AnswerText, MaxChars: 1000,
	},
	{
		ID: "keep", Heading: "Keep", Text: "What must stay even if it is a repeat or a pause?",
		Kind: AnswerText, MaxChars: 1000,
	},
	{
		ID: "short_from_it", Heading: "Short from it", Text: "Also extract a short from it?",
		Kind: AnswerChoice, Options: []string{"yes", "no"},
	},
	{ID: "tone", Heading: "Tone", Text: "Which tone of voice?", Kind: AnswerText, MaxChars: 200},
	{
		ID: "references", Heading: "References", Text: "Any references to follow (links, creators, videos)?",
		Kind: AnswerText, MaxChars: 2000,
	},
	{ID: "must_include", Heading: "Must appear", Text: "What must appear in it?", Kind: AnswerText, MaxChars: 1000},
	{
		ID: "must_avoid", Heading: "Must not appear", Text: "What must not appear in it?",
		Kind: AnswerText, MaxChars: 1000,
	},
	{ID: "deadline", Heading: "Deadline", Text: "By when (YYYY-MM-DD)?", Kind: AnswerDate},
}

// questionSet is the table of a kind, nil for an unknown one. It is never handed out: callers get a copy.
func questionSet(k ProductionKind) []Question {
	switch k {
	case ProductionShort:
		return briefingQuestions
	case ProductionCommercial:
		return commercialQuestions
	case ProductionLong:
		return longQuestions
	default:
		return nil
	}
}

func unknownKind(k ProductionKind) *RuleError {
	names := make([]string, 0, 3)
	for _, known := range ProductionKinds() {
		names = append(names, string(known))
	}

	return validation("kind", "send one of: "+strings.Join(names, ", "), "%q is not a kind of production", string(k))
}

// BriefingQuestionsFor returns the questions of the briefing of a kind of production, in the order they are
// asked. The caller may change the result. BriefingQuestions is the set of ProductionShort.
func BriefingQuestionsFor(k ProductionKind) ([]Question, *RuleError) {
	set := questionSet(k)
	if set == nil {
		return nil, unknownKind(k)
	}

	out := make([]Question, len(set))

	for i, q := range set {
		q.Options = append([]string(nil), q.Options...)
		out[i] = q
	}

	return out, nil
}

// CheckAnswersFor is CheckAnswers for the questions of a kind of production.
func CheckAnswersFor(k ProductionKind, answers map[string]string) (map[string]string, *RuleError) {
	set := questionSet(k)
	if set == nil {
		return nil, unknownKind(k)
	}

	return checkAnswers(set, answers)
}

// BriefingMarkdownFor is BriefingMarkdown for the questions of a kind of production; answers come from
// CheckAnswersFor. An unknown kind gives the brief of no answers.
func BriefingMarkdownFor(k ProductionKind, answers map[string]string) string {
	return briefingMarkdown(questionSet(k), answers)
}

// ParseBriefingFor is ParseBriefing for the questions of a kind of production.
func ParseBriefingFor(k ProductionKind, text string) (answers map[string]string, ok bool) {
	set := questionSet(k)
	if set == nil {
		return nil, false
	}

	return parseBriefing(set, text)
}

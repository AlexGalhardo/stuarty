package project

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// AnswerKind is the shape of the answer to a briefing question.
type AnswerKind string

// The kinds of answer a question may ask for.
const (
	AnswerText    AnswerKind = "text"    // free text
	AnswerChoice  AnswerKind = "choice"  // one of Options
	AnswerSeconds AnswerKind = "seconds" // a whole number of seconds, MinSeconds to MaxSeconds
	AnswerDate    AnswerKind = "date"    // a calendar date, YYYY-MM-DD
)

// The bounds of a duration answer: a short video is at most three minutes long.
const (
	MinSeconds = 1
	MaxSeconds = 180
)

// BriefTitle is the first line of a brief that the briefing wrote.
const BriefTitle = "# Brief"

// Question is one question of the briefing (RF-301). The questions are data, so the form, the CLI, the HTTP API
// and an agent in a chat all ask the same ones.
type Question struct {
	ID       string     `json:"id"`
	Heading  string     `json:"heading"` // the heading of the answer in the brief
	Text     string     `json:"text"`    // the question, in English; the UI shows its own translation by id
	Kind     AnswerKind `json:"kind"`
	Required bool       `json:"required"`
	Options  []string   `json:"options,omitempty"` // the answers a choice allows
	MaxChars int        `json:"max_chars,omitempty"`
	Min      int        `json:"min,omitempty"` // a duration: the least and the most, in seconds
	Max      int        `json:"max,omitempty"`
}

var briefingQuestions = []Question{
	{
		ID:       "audience",
		Heading:  "Audience",
		Text:     "Who is this video for?",
		Kind:     AnswerText,
		Required: true,
		MaxChars: 400,
	},
	{
		ID:       "goal",
		Heading:  "Goal",
		Text:     "What should the viewer do or feel after watching?",
		Kind:     AnswerText,
		Required: true,
		MaxChars: 400,
	},
	{
		ID: "platform", Heading: "Platform", Text: "Where will it be published?", Kind: AnswerChoice, Required: true,
		Options: []string{"youtube-shorts", "tiktok", "instagram-reels", "facebook-reels", "other"},
	},
	{
		ID: "format", Heading: "Format", Text: "Which format?", Kind: AnswerChoice, Required: true,
		Options: []string{"vertical-9x16", "square-1x1", "horizontal-16x9"},
	},
	{
		ID:       "duration",
		Heading:  "Duration (seconds)",
		Text:     "How long, in seconds?",
		Kind:     AnswerSeconds,
		Required: true,
		Min:      MinSeconds,
		Max:      MaxSeconds,
	},
	{ID: "tone", Heading: "Tone", Text: "Which tone of voice?", Kind: AnswerText, MaxChars: 200},
	{
		ID:       "references",
		Heading:  "References",
		Text:     "Any references to follow (links, creators, videos)?",
		Kind:     AnswerText,
		MaxChars: 2000,
	},
	{ID: "must_include", Heading: "Must appear", Text: "What must appear in it?", Kind: AnswerText, MaxChars: 1000},
	{
		ID:       "must_avoid",
		Heading:  "Must not appear",
		Text:     "What must not appear in it?",
		Kind:     AnswerText,
		MaxChars: 1000,
	},
	{ID: "deadline", Heading: "Deadline", Text: "By when (YYYY-MM-DD)?", Kind: AnswerDate},
}

// BriefingQuestions returns the questions of the briefing, in the order they are asked. The caller may change
// the result.
func BriefingQuestions() []Question {
	out := make([]Question, len(briefingQuestions))

	for i, q := range briefingQuestions {
		q.Options = append([]string(nil), q.Options...)
		out[i] = q
	}

	return out
}

// CheckAnswers validates the answers to the briefing and returns them normalised: white space trimmed at both
// ends, line ends as LF, an empty answer left out. Every required question must be answered, and an id that is
// not a question is refused, so that a typo never passes silently. The answers replace the whole briefing.
func CheckAnswers(answers map[string]string) (map[string]string, *RuleError) {
	return checkAnswers(briefingQuestions, answers)
}

func checkAnswers(set []Question, answers map[string]string) (map[string]string, *RuleError) {
	known := make(map[string]Question, len(set))
	for _, q := range set {
		known[q.ID] = q
	}

	for id := range answers {
		if _, ok := known[id]; !ok {
			return nil, validation(
				"answers."+id,
				"send only the ids of BriefingQuestions",
				"%q is not a question of the briefing",
				id,
			)
		}
	}

	out := make(map[string]string, len(answers))

	for _, q := range set {
		a := strings.TrimSpace(strings.ReplaceAll(answers[q.ID], "\r\n", "\n"))
		path := "answers." + q.ID

		if a == "" {
			if q.Required {
				return nil, validation(path, "answer every required question", "the question %q needs an answer", q.ID)
			}

			continue
		}

		if v := checkAnswer(&q, a, path); v != nil {
			return nil, v
		}

		out[q.ID] = a
	}

	return out, nil
}

func checkAnswer(q *Question, a, path string) *RuleError {
	if !utf8.ValidString(a) {
		return validation(path, "send UTF-8 text", "the answer is not valid UTF-8")
	}

	for _, r := range a {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || isBidiControl(r) {
			return validation(path, "send text with no control or direction character",
				"the answer has a control or text direction character")
		}
	}

	switch q.Kind {
	case AnswerText:
		if n := utf8.RuneCountInString(a); n > q.MaxChars {
			return validation(path, fmt.Sprintf("send at most %d characters", q.MaxChars),
				"the answer has %d characters; the most is %d", n, q.MaxChars)
		}
	case AnswerChoice:
		if slices.Contains(q.Options, a) {
			return nil
		}

		return validation(path, "send one of: "+strings.Join(q.Options, ", "), "%q is not an option of %q", a, q.ID)
	case AnswerSeconds:
		n, err := strconv.Atoi(a)
		if err != nil || n < q.Min || n > q.Max || strconv.Itoa(n) != a {
			return validation(path, fmt.Sprintf("send a whole number from %d to %d", q.Min, q.Max),
				"%q is not a number of seconds from %d to %d", a, q.Min, q.Max)
		}
	case AnswerDate:
		if _, err := time.Parse(time.DateOnly, a); err != nil {
			return validation(path, "send a date as YYYY-MM-DD", "%q is not a date as YYYY-MM-DD", a)
		}
	}

	return nil
}

// BriefingMarkdown writes the brief of validated answers (CheckAnswers): the title, then one section per
// answered question, in the order of the questions. A line of an answer that starts with # gets a backslash, so
// an answer never makes a heading of its own; ParseBriefing takes it away.
func BriefingMarkdown(answers map[string]string) string {
	return briefingMarkdown(briefingQuestions, answers)
}

func briefingMarkdown(set []Question, answers map[string]string) string {
	var b strings.Builder

	b.WriteString(BriefTitle + "\n")

	for _, q := range set {
		a, ok := answers[q.ID]
		if !ok || a == "" {
			continue
		}

		b.WriteString("\n## " + q.Heading + "\n\n")

		for i, line := range strings.Split(a, "\n") {
			if i > 0 {
				b.WriteByte('\n')
			}

			if strings.HasPrefix(strings.TrimLeft(line, "\\"), "#") {
				b.WriteByte('\\')
			}

			b.WriteString(line)
		}

		b.WriteByte('\n')
	}

	return b.String()
}

// ParseBriefing reads the answers back from a brief. ok is false when the text is not a brief that the briefing
// wrote (no title, a heading that is not a question's, a section twice, an answer CheckAnswers refuses, or any
// text BriefingMarkdown would not write), so a brief written by hand is never taken for one and never
// rewritten by accident.
func ParseBriefing(text string) (answers map[string]string, ok bool) {
	return parseBriefing(briefingQuestions, text)
}

func parseBriefing(set []Question, text string) (answers map[string]string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != BriefTitle {
		return nil, false
	}

	byHeading := make(map[string]string, len(set))
	for _, q := range set {
		byHeading[q.Heading] = q.ID
	}

	answers = map[string]string{}
	current := ""

	var body []string

	flush := func() {
		if current != "" {
			answers[current] = strings.TrimSpace(strings.Join(body, "\n"))
		}

		body = body[:0]
	}

	for _, line := range lines[1:] {
		if h, found := strings.CutPrefix(line, "## "); found {
			id, known := byHeading[strings.TrimSpace(h)]
			if _, dup := answers[id]; !known || dup {
				return nil, false
			}

			flush()

			current = id
			answers[id] = ""

			continue
		}

		if current == "" {
			if strings.TrimSpace(line) != "" {
				return nil, false
			}

			continue
		}

		if strings.HasPrefix(strings.TrimLeft(line, "\\"), "#") && strings.HasPrefix(line, "\\") {
			line = line[1:]
		}

		body = append(body, line)
	}

	flush()

	// The shape is not enough: a person may write the same headings by hand, with a long text or without the
	// required answers. Only a text whose answers pass CheckAnswers and that the briefing writes again as it
	// is counts, so that answering again never loses a word of it.
	checked, v := checkAnswers(set, answers)
	if v != nil {
		return nil, false
	}

	if strings.TrimRight(briefingMarkdown(set, checked), "\n") != strings.TrimRight(strings.Join(lines, "\n"), "\n") {
		return nil, false
	}

	return checked, true
}

// CheckBriefingReplace is the rule that keeps a brief a person wrote (SDD-005 section 5.12.1): the answers of
// the briefing replace an empty brief or one the briefing wrote, and a brief written by hand only with replace.
func CheckBriefingReplace(current string, replace bool) *RuleError {
	if replace || strings.TrimSpace(current) == "" {
		return nil
	}

	if _, guided := ParseBriefing(current); guided {
		return nil
	}

	return &RuleError{
		Code: "E_INVALID_STATE", Path: "replace",
		Hint:    "send replace true to replace it, or clear it first with the brief operation and an empty text",
		Message: "the brief was written by hand and is not replaced",
	}
}

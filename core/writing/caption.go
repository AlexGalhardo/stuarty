package writing

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Normalizer turns a text into its Unicode NFC form. A network that counts after NFC (X, SDD-019 section 5.1) is
// measured on what it returns. core/ carries no Unicode tables of its own (ADR-0001, internal/tools/importcheck), so
// the caller passes one: the service passes norm.NFC.String of golang.org/x/text. nil measures the text as it is.
type Normalizer func(string) string

// Variant is the part of a caption that goes to one network; Network is empty for the common variant, the text
// before the first network heading.
type Variant struct {
	Network string `json:"network"`
	Text    string `json:"text"`
}

// ParseCaption splits the body of a caption into its variants (SDD-019 section 5): a heading of level 2 whose text
// is the identifier of a network starts the variant of that network. The common variant comes first and is left out
// when it is blank and some network variant exists; a body with no network heading is one common variant.
func ParseCaption(body string) []Variant {
	var (
		out  []Variant
		cur  = Variant{}
		text []string
	)

	end := func() {
		cur.Text = strings.TrimSpace(strings.Join(text, "\n"))
		out = append(out, cur)
		text = nil
	}

	for line := range strings.Lines(strings.ReplaceAll(body, "\r\n", "\n")) {
		line = strings.TrimRight(line, "\n")

		if id, ok := networkHeading(line); ok {
			end()

			cur = Variant{Network: id}

			continue
		}

		text = append(text, line)
	}

	end()

	if len(out) > 1 && out[0].Network == "" && out[0].Text == "" {
		out = out[1:]
	}

	return out
}

func networkHeading(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, "## ")
	if !ok {
		return "", false
	}

	id := strings.ToLower(strings.TrimSpace(rest))
	if _, known := Lookup(id); !known {
		return "", false
	}

	return id, true
}

// CaptionCounts are what a variant measures: its length in the unit of its network and what it holds.
type CaptionCounts struct {
	Length   int `json:"length"`
	Hashtags int `json:"hashtags"`
	Mentions int `json:"mentions"`
	Links    int `json:"links"`
}

// The codes of a problem of a caption (SDD-019 section 5.2).
const (
	ProblemEmpty        = "empty"
	ProblemTooLong      = "too_long"
	ProblemTooManyTags  = "too_many_hashtags"
	ProblemTooManyMen   = "too_many_mentions"
	ProblemForbidden    = "forbidden_character"
	ProblemUnknownNet   = "unknown_network"
	maxMentionNameRunes = 30
)

// CaptionProblem is one thing wrong with a variant, with the limit and the actual value so that a person sees how far.
type CaptionProblem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Limit   int    `json:"limit"`
	Actual  int    `json:"actual"`
	Unit    Unit   `json:"unit,omitempty"`
}

// CaptionReport is the validation of one variant against one network.
type CaptionReport struct {
	Network string `json:"network"`
	// Name is the name of the network in the table of limits, for a person to read; empty for the common variant.
	Name           string           `json:"name,omitempty"`
	Unit           Unit             `json:"unit,omitempty"`
	MaxLength      int              `json:"max_length"`
	Counts         CaptionCounts    `json:"counts"`
	LinksClickable bool             `json:"links_clickable"`
	Problems       []CaptionProblem `json:"problems"`
	OK             bool             `json:"ok"`
}

// ValidateCaption measures the text of a variant against the limits of a network and reports every limit it
// breaks (SDD-019 section 5.2). An empty network is the common variant: it is measured in characters and has no
// limit. An unknown network is one problem. nfc is the normalization of a network that counts after NFC.
func ValidateCaption(text, network string, nfc Normalizer) CaptionReport {
	text = strings.TrimSpace(text)
	r := CaptionReport{Network: network, Problems: []CaptionProblem{}}

	n, known := Lookup(network)
	if network != "" && !known {
		r.Problems = append(r.Problems, CaptionProblem{
			Code: ProblemUnknownNet, Message: fmt.Sprintf("%q is not a network of the table of limits", network),
		})
		r.Counts = measure(text, Network{Unit: UnitCharacters}, nfc)

		return r
	}

	if !known {
		n = Network{Unit: UnitCharacters, LinksClickable: true}
	}

	r.Unit, r.MaxLength, r.LinksClickable = n.Unit, n.MaxLength, n.LinksClickable
	r.Name = n.Name
	r.Counts = measure(text, n, nfc)

	if !strings.ContainsFunc(text, func(c rune) bool { return !unicode.IsSpace(c) && !unicode.Is(unicode.Cf, c) }) {
		r.Problems = append(
			r.Problems,
			CaptionProblem{Code: ProblemEmpty, Message: "the variant has no visible character"},
		)
	}

	r.Problems = append(r.Problems, limitProblems(text, &n, r.Counts)...)
	r.OK = len(r.Problems) == 0

	return r
}

func limitProblems(text string, n *Network, c CaptionCounts) []CaptionProblem {
	var out []CaptionProblem

	if n.MaxLength > 0 && c.Length > n.MaxLength {
		out = append(out, CaptionProblem{
			Code:   ProblemTooLong,
			Limit:  n.MaxLength,
			Actual: c.Length,
			Unit:   n.Unit,
			Message: fmt.Sprintf(
				"%s allows %d %s and the text has %d",
				n.Name,
				n.MaxLength,
				unitName(n.Unit),
				c.Length,
			),
		})
	}

	if n.MaxHashtags > 0 && c.Hashtags > n.MaxHashtags {
		out = append(out, CaptionProblem{
			Code: ProblemTooManyTags, Limit: n.MaxHashtags, Actual: c.Hashtags,
			Message: fmt.Sprintf("%s allows %d hashtags and the text has %d", n.Name, n.MaxHashtags, c.Hashtags),
		})
	}

	if n.MaxMentions > 0 && c.Mentions > n.MaxMentions {
		out = append(out, CaptionProblem{
			Code: ProblemTooManyMen, Limit: n.MaxMentions, Actual: c.Mentions,
			Message: fmt.Sprintf("%s allows %d mentions and the text has %d", n.Name, n.MaxMentions, c.Mentions),
		})
	}

	if n.Forbidden == "" {
		return out
	}

	bad := 0

	for _, ch := range text {
		if strings.ContainsRune(n.Forbidden, ch) {
			bad++
		}
	}

	if bad > 0 {
		out = append(out, CaptionProblem{
			Code: ProblemForbidden, Actual: bad,
			Message: fmt.Sprintf("%s refuses the characters %s and the text has %d of them", n.Name,
				quoted(n.Forbidden), bad),
		})
	}

	return out
}

func quoted(set string) string {
	parts := make([]string, 0, len(set))
	for _, r := range set {
		parts = append(parts, string(r))
	}

	return strings.Join(parts, " ")
}

func unitName(u Unit) string {
	switch u {
	case UnitBytes:
		return "bytes"
	case UnitUTF16:
		return "UTF-16 code units"
	case UnitWeighted:
		return "weighted characters"
	case UnitCharacters:
		return "characters"
	}

	return string(u)
}

// measure counts a variant for a network.
func measure(text string, n Network, nfc Normalizer) CaptionCounts {
	links := findLinks(text)

	var c CaptionCounts

	c.Links = len(links)

	masked := maskLinks(text, links)
	c.Hashtags = countHashtags(masked)
	c.Mentions = countMentions(masked)

	switch n.Unit {
	case UnitBytes:
		c.Length = len(text)
	case UnitUTF16:
		for _, r := range text {
			c.Length++
			if r >= 0x10000 {
				c.Length++
			}
		}
	case UnitWeighted:
		if nfc != nil {
			c.Length = weightedLength(nfc(text), n.LinkWeight)
		} else {
			c.Length = weightedLength(text, n.LinkWeight)
		}
	case UnitCharacters:
		c.Length = utf8.RuneCountInString(text)
	}

	return c
}

type span struct{ start, end int } // byte offsets, end exclusive

// findLinks finds the http and https links of text: the scheme at the start of a word, up to the next white space,
// without the punctuation that closes a sentence.
func findLinks(text string) []span {
	var out []span

	low := strings.ToLower(text)

	for i := 0; i < len(text); {
		j := -1

		for _, scheme := range []string{"https://", "http://"} {
			if strings.HasPrefix(low[i:], scheme) {
				j = i + len(scheme)

				break
			}
		}

		if j < 0 || (i > 0 && isWordRune(lastRune(text[:i]))) {
			i++

			continue
		}

		end := j
		for end < len(text) {
			r, size := utf8.DecodeRuneInString(text[end:])
			if unicode.IsSpace(r) {
				break
			}

			end += size
		}

		for end > j && strings.ContainsRune(".,;:!?)]}'\"", rune(text[end-1])) {
			end--
		}

		if end > j {
			out = append(out, span{i, end})
			i = end

			continue
		}

		i = j
	}

	return out
}

func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)

	return r
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// maskLinks replaces every link by a space, so that a fragment or a user part of a link is not a hashtag or a mention.
func maskLinks(text string, links []span) string {
	if len(links) == 0 {
		return text
	}

	var b strings.Builder

	prev := 0

	for _, l := range links {
		b.WriteString(text[prev:l.start])
		b.WriteByte(' ')

		prev = l.end
	}

	b.WriteString(text[prev:])

	return b.String()
}

func countHashtags(text string) int {
	n := 0
	runes := []rune(text)

	for i := 0; i < len(runes); i++ {
		if runes[i] != '#' || (i > 0 && (isWordRune(runes[i-1]) || runes[i-1] == '&')) {
			continue
		}

		j, letter := i+1, false
		for j < len(runes) && isWordRune(runes[j]) {
			letter = letter || unicode.IsLetter(runes[j])
			j++
		}

		if j > i+1 && letter {
			n++
		}

		i = j - 1
	}

	return n
}

func countMentions(text string) int {
	n := 0
	runes := []rune(text)

	for i := 0; i < len(runes); i++ {
		if runes[i] != '@' || (i > 0 && (isWordRune(runes[i-1]) || runes[i-1] == '.')) {
			continue
		}

		j := i + 1
		for j < len(runes) && j-i <= maxMentionNameRunes && (isWordRune(runes[j]) || runes[j] == '.') {
			j++
		}

		name := strings.TrimRight(string(runes[i+1:j]), ".")
		if name != "" {
			n++
		}

		i = j - 1
	}

	return n
}

// weightedLength is the length of text for X (SDD-019 section 5.1), on the text after Unicode NFC (measure applies
// it): a link weighing linkWeight, an emoji weighing 2 however many code points it is made of, and any other
// character weighing 1 when it is in the ranges X counts once and 2 otherwise.
func weightedLength(text string, linkWeight int) int {
	links := findLinks(text)
	total := len(links) * linkWeight
	runes := []rune(maskLinks(text, links))

	for i := 0; i < len(runes); {
		w, n := clusterWeight(runes[i:])
		total += w
		i += n
	}

	// maskLinks put one space for each link; take those back.
	return total - len(links)
}

// emojiWeight is what X counts for an emoji, however many code points it is made of: a presentation selector, a
// skin tone, a joined sequence, a flag, a keycap or a tag sequence is one emoji (docs.x.com, counting characters).
const emojiWeight = 2

// clusterWeight is the weight of the character that starts rs and the number of runes it takes: one rune, or a
// whole emoji sequence.
func clusterWeight(rs []rune) (weight, size int) {
	if isRegionalIndicator(rs[0]) && len(rs) > 1 && isRegionalIndicator(rs[1]) {
		return emojiWeight, 2
	}

	weight, size = runeWeight(rs[0]), 1
	emoji := false

	for size < len(rs) {
		r := rs[size]

		switch {
		case isEmojiModifier(r):
			emoji, size = true, size+1
		case r == 0xFE0E: // text presentation: the character stays what it is
			size++
		case r == 0x200D && size+1 < len(rs) && (emoji || weight == emojiWeight) &&
			runeWeight(rs[size+1]) == emojiWeight:
			emoji, size = true, size+2
		default:
			return emojiOr(weight, emoji), size
		}
	}

	return emojiOr(weight, emoji), size
}

// isEmojiModifier reports the code points that make the character before them an emoji and add nothing: the emoji
// presentation selector, the keycap, the skin tones and the tags of a subdivision flag.
func isEmojiModifier(r rune) bool {
	return r == 0xFE0F || r == 0x20E3 || r >= 0x1F3FB && r <= 0x1F3FF || r >= 0xE0020 && r <= 0xE007F
}

func emojiOr(weight int, emoji bool) int {
	if emoji {
		return emojiWeight
	}

	return weight
}

func isRegionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

func runeWeight(r rune) int {
	switch {
	case r <= 4351, r >= 8192 && r <= 8205, r >= 8208 && r <= 8223, r >= 8242 && r <= 8247:
		return 1
	}

	return 2
}

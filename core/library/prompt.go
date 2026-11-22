package library

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// Prompt is a reusable text with variables (SDD-018 section 6).
type Prompt struct {
	Name        string
	Description string
	Tags        []string
	Defaults    map[string]string
	Actor       string
	Created     string
	Updated     string
	Body        string

	fm *FrontMatter
}

var (
	variableUse  = regexp.MustCompile(`\{\{\s*([a-z][a-z0-9_]{0,39})\s*\}\}`)
	variableName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
)

// ParsePrompt reads the text of a prompt file.
func ParsePrompt(text string) (Prompt, error) {
	fm, body, err := ParseDocument(text)
	if err != nil {
		return Prompt{}, err
	}

	return Prompt{
		Name: fm.Scalar("name"), Description: fm.Scalar("description"), Tags: fm.List("tags"),
		Defaults: fm.Map("defaults"), Actor: fm.Scalar("actor"),
		Created: fm.Scalar("created"), Updated: fm.Scalar("updated"), Body: body, fm: fm,
	}, nil
}

// Variables lists the variables a text uses, in order of first use.
func Variables(body string) []string {
	var out []string

	seen := map[string]bool{}

	for _, m := range variableUse.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true

			out = append(out, m[1])
		}
	}

	return out
}

// Validate applies the rules of a prompt.
func (p *Prompt) Validate() error {
	if err := ValidName(p.Name); err != nil {
		return fieldError("name", err)
	}

	if err := checkLine("description", p.Description, MaxDescription, true); err != nil {
		return err
	}

	if err := ValidTags(p.Tags); err != nil {
		return fieldError("tags", err)
	}

	if strings.TrimSpace(p.Body) == "" {
		return fieldError("body", errors.New("it is required"))
	}

	if len(p.Body) > MaxPromptBody {
		return fieldError("body", fmt.Errorf("the body is longer than %d bytes", MaxPromptBody))
	}

	if !utf8.ValidString(p.Body) {
		return fieldError("body", errors.New("the text is not valid UTF-8"))
	}

	used := map[string]bool{}
	for _, v := range Variables(p.Body) {
		used[v] = true
	}

	for k, v := range p.Defaults {
		if !variableName.MatchString(k) {
			return fieldError("defaults", fmt.Errorf("%q is not a variable name", command.SafeKey(k)))
		}

		if !used[k] {
			return fieldError("defaults", fmt.Errorf("the text does not use the variable %q", command.SafeKey(k)))
		}

		if len(v) > MaxValueLength {
			return fieldError("defaults", fmt.Errorf("the default of %q is longer than %d bytes", k, MaxValueLength))
		}
	}

	return nil
}

// Marshal writes the file of the prompt.
func (p Prompt) Marshal() (string, error) {
	fm := baseFrontMatter(p.fm)
	fm.SetScalar("name", p.Name)
	fm.SetScalar("description", p.Description)
	fm.SetList("tags", p.Tags)
	fm.SetMap("defaults", p.Defaults)
	fm.SetScalar("actor", p.Actor)
	fm.SetScalar("created", p.Created)
	fm.SetScalar("updated", p.Updated)

	return Compose(fm, p.Body)
}

// RenderError says which variables stopped a rendering.
type RenderError struct {
	Missing []string // used by the text, given no value and having no default
	Unknown []string // given a value, but not used by the text
	TooLong []string // a value longer than the limit
}

func (e *RenderError) Error() string {
	var parts []string
	if len(e.Missing) > 0 {
		parts = append(parts, "no value for "+strings.Join(e.Missing, ", "))
	}

	if len(e.Unknown) > 0 {
		parts = append(parts, "the text does not use "+strings.Join(e.Unknown, ", "))
	}

	if len(e.TooLong) > 0 {
		parts = append(
			parts,
			fmt.Sprintf("a value over %d bytes for %s", MaxValueLength, strings.Join(e.TooLong, ", ")),
		)
	}

	return strings.Join(parts, "; ")
}

// Render replaces each variable of the text once. A value is never scanned for
// variables, so a value cannot bring in another one. A variable with no value
// takes its default; with neither, or with a value the text does not use, the
// rendering fails and names them.
func Render(p *Prompt, values map[string]string) (string, error) {
	used := Variables(p.Body)
	isUsed := map[string]bool{}

	for _, v := range used {
		isUsed[v] = true
	}

	var rerr RenderError

	for k, v := range values {
		if !isUsed[k] {
			if key := command.SafeKey(k); !slices.Contains(rerr.Unknown, key) {
				rerr.Unknown = append(rerr.Unknown, key)
			}

			continue
		}

		if len(v) > MaxValueLength {
			rerr.TooLong = append(rerr.TooLong, k)
		}
	}

	resolved := map[string]string{}

	for _, v := range used {
		if val, ok := values[v]; ok {
			resolved[v] = val
		} else if def, ok := p.Defaults[v]; ok {
			resolved[v] = def
		} else {
			rerr.Missing = append(rerr.Missing, v)
		}
	}

	sort.Strings(rerr.Unknown)
	sort.Strings(rerr.TooLong)

	if len(rerr.Missing)+len(rerr.Unknown)+len(rerr.TooLong) > 0 {
		return "", &rerr
	}

	var b strings.Builder

	last := 0

	for _, m := range variableUse.FindAllStringSubmatchIndex(p.Body, -1) {
		b.WriteString(p.Body[last:m[0]])
		b.WriteString(resolved[p.Body[m[2]:m[3]]])

		last = m[1]
	}

	b.WriteString(p.Body[last:])

	return b.String(), nil
}

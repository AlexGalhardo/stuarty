package asset

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The forms of the members of a render by another tool (SDD-011 section 15.2): tokens, not free text, so they are
// not delimited as untrusted.
var (
	engineForm    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)
	renderKeyForm = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,119}$`)
)

// OriginKind says where a file came from.
type OriginKind string

// The origin kinds (SDD-000 section 16).
const (
	Imported           OriginKind = "imported"
	Generated          OriginKind = "generated"
	FetchedByAgent     OriginKind = "fetched_by_agent"
	RenderedExternally OriginKind = "rendered_externally"
)

// Kinds lists the origin kinds in the order the interfaces show them.
func Kinds() []OriginKind {
	return []OriginKind{Imported, Generated, FetchedByAgent, RenderedExternally}
}

// Valid reports whether k is one of the kinds.
func (k OriginKind) Valid() bool {
	switch k {
	case Imported, Generated, FetchedByAgent, RenderedExternally:
		return true
	}

	return false
}

// The caps of the text of an origin, in characters (SDD-011 section 14.2).
const (
	MaxSource      = 2000
	MaxPrompt      = 8000
	MaxRightsNote  = 2000
	MaxDescription = 2000
	MaxProvider    = 100
	MaxModel       = 100
)

// Origin is where a file came from and what the user recorded about its rights. Source, Prompt and RightsNote are
// text from outside and are untrusted wherever they are shown.
type Origin struct {
	Kind       OriginKind `json:"kind"`
	Source     string     `json:"source,omitempty"      untrusted:"true"` // a path, a URL or a tool name, as plain text
	Provider   string     `json:"provider,omitempty"`
	Model      string     `json:"model,omitempty"`
	Prompt     string     `json:"prompt,omitempty"      untrusted:"true"`
	CostMicros *int64     `json:"cost_micros,omitempty"`                  // millionths of a US dollar
	RightsNote string     `json:"rights_note,omitempty" untrusted:"true"` // the user's own record, never a licence check

	// The members of a file rendered by another tool (kind rendered_externally, SDD-011 section 15.2): the engine
	// and its version, and the key the agent chose for this animation, which stays the same when it is rendered again.
	Engine        string `json:"engine,omitempty"`
	EngineVersion string `json:"engine_version,omitempty"`
	RenderKey     string `json:"render_key,omitempty"`
}

// RuleError is a refusal with a stable reason, for E_VALIDATION (SDD-000 section 16).
type RuleError struct {
	Path   string // origin.<member>, tags, description
	Reason string // origin_kind_unknown, source_required, rights_note_required, ... too_long, cost_invalid, tags_invalid
	Detail string
}

func (v *RuleError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", v.Path, v.Detail, v.Reason)
}

// Normalize fills the default kind and trims the text. An empty kind is imported.
func (o Origin) Normalize() Origin {
	if o.Kind == "" {
		o.Kind = Imported
	}

	o.Source = strings.TrimSpace(o.Source)
	o.Provider = strings.TrimSpace(o.Provider)
	o.Model = strings.TrimSpace(o.Model)
	o.Prompt = strings.TrimSpace(o.Prompt)
	o.RightsNote = strings.TrimSpace(o.RightsNote)
	o.Engine = strings.TrimSpace(o.Engine)
	o.EngineVersion = strings.TrimSpace(o.EngineVersion)
	o.RenderKey = strings.TrimSpace(o.RenderKey)

	return o
}

// Validate checks an origin that Normalize has already seen: the kind is known, what the kind requires is there,
// no text is over its cap and the cost is not negative (SDD-011 section 14.2).
func (o Origin) Validate() *RuleError {
	if !o.Kind.Valid() {
		return &RuleError{
			Path:   "origin.kind",
			Reason: "origin_kind_unknown",
			Detail: fmt.Sprintf(
				"the origin kind %q is not known; use imported, generated, fetched_by_agent or rendered_externally",
				o.Kind,
			),
		}
	}

	required := func(member, value string) *RuleError {
		if value != "" {
			return nil
		}

		return &RuleError{
			Path: "origin." + member, Reason: member + "_required",
			Detail: fmt.Sprintf("an origin of kind %s must give %s", o.Kind, member),
		}
	}

	var rules []*RuleError

	switch o.Kind {
	case Generated:
		rules = append(rules, required("provider", o.Provider), required("model", o.Model))
	case FetchedByAgent:
		rules = append(rules, required("source", o.Source), required("rights_note", o.RightsNote))
	case RenderedExternally:
		rules = append(rules, required("source", o.Source))
	case Imported:
	}

	for _, v := range rules {
		if v != nil {
			return v
		}
	}

	if v := o.checkRender(); v != nil {
		return v
	}

	for _, c := range []struct {
		member, value string
		max           int
	}{
		{"source", o.Source, MaxSource},
		{"prompt", o.Prompt, MaxPrompt},
		{"rights_note", o.RightsNote, MaxRightsNote},
		{"provider", o.Provider, MaxProvider},
		{"model", o.Model, MaxModel},
	} {
		if n := utf8.RuneCountInString(c.value); n > c.max {
			return &RuleError{
				Path: "origin." + c.member, Reason: "too_long",
				Detail: fmt.Sprintf("%s has %d characters; at most %d", c.member, n, c.max),
			}
		}
	}

	if o.CostMicros != nil && *o.CostMicros < 0 {
		return &RuleError{Path: "origin.cost_micros", Reason: "cost_invalid", Detail: "the cost cannot be negative"}
	}

	return nil
}

// checkRender holds the members of a file rendered by another tool to their kind and their form: they belong to
// rendered_externally only, and each is a token (SDD-011 section 14.2).
func (o Origin) checkRender() *RuleError {
	for _, m := range []struct {
		member, value string
		form          *regexp.Regexp
	}{
		{"engine", o.Engine, engineForm},
		{"engine_version", o.EngineVersion, engineForm},
		{"render_key", o.RenderKey, renderKeyForm},
	} {
		switch {
		case m.value == "":
		case o.Kind != RenderedExternally:
			return &RuleError{
				Path: "origin." + m.member, Reason: "origin_member_not_allowed",
				Detail: fmt.Sprintf("%s belongs to an origin of kind rendered_externally, not %s", m.member, o.Kind),
			}
		case !m.form.MatchString(m.value):
			return &RuleError{
				Path:   "origin." + m.member,
				Reason: "origin_member_invalid",
				Detail: fmt.Sprintf(
					"%s must be letters, digits and . _ - + (and : / in render_key), at most %d characters",
					m.member,
					map[bool]int{true: 120, false: 64}[m.member == "render_key"],
				),
			}
		}
	}

	return nil
}

// CheckDescription holds the description of an entry to its cap.
func CheckDescription(s string) *RuleError {
	if n := utf8.RuneCountInString(s); n > MaxDescription {
		return &RuleError{
			Path: "description", Reason: "too_long",
			Detail: fmt.Sprintf("the description has %d characters; at most %d", n, MaxDescription),
		}
	}

	return nil
}

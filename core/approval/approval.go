// Package approval is the pure rule set of the approval queue (ADR-0032 items 3 to 5, SDD-015): what
// an action needs before it runs (a person, a rule or a refusal), which rule matches, and the cost
// ceilings. It does no I/O and reads no clock: the service gathers the rules, the ceilings and the
// amounts spent today and asks Decide.
package approval

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits of SDD-015 section 3.
const (
	// MaxSummary is the longest summary of a request, in characters.
	MaxSummary = 500
	// MaxReason is the longest note of a rejection, in characters.
	MaxReason = 500
	// MaxRules is the most rules a workspace may hold.
	MaxRules = 100
	// MaxMicros bounds every amount, so that no sum overflows: a billion units of currency.
	MaxMicros = int64(1_000_000_000) * 1_000_000
	// MicrosPerUnit is the number of micros in one unit of a currency.
	MicrosPerUnit = int64(1_000_000)
)

// Money is an amount in a currency, in millionths of its unit, so that no sum rounds.
type Money struct {
	Micros   int64  `json:"micros"`
	Currency string `json:"currency"`
}

// Action is what is about to run: the question Decide answers.
type Action struct {
	Kind       string // the kind of the job, for example "publish.post"
	Connection string // the destination connection; empty when it has none
	Project    string // the project; empty when it has none
	Cost       Money  // the estimate; Micros 0 is no cost
}

// Rule approves a kind of action without asking a person (ADR-0032 item 4). A rule is written only
// from a privileged browser session.
type Rule struct {
	ID         string
	Kind       string
	Connection string // empty means any
	Project    string // empty means any
	Currency   string
	PerItem    *int64 // micros; nil means not set
	PerDay     *int64 // micros; nil means not set
	Enabled    bool
}

// Ceiling bounds spending (ADR-0032 item 5). Project empty is the ceiling of the workspace.
type Ceiling struct {
	Project  string
	Currency string
	PerItem  *int64
	PerDay   *int64
}

// Spent is what was spent today, in the currency of the action: the real cost of what finished and
// the estimate of what is reserved.
type Spent struct {
	Workspace int64            // every action
	Project   int64            // the actions of the project of the action
	ByRule    map[string]int64 // the actions each rule covers
}

// Verdict is the kind of an Outcome.
type Verdict int

// The three outcomes of Decide.
const (
	// NeedsPerson puts the action in the approval queue.
	NeedsPerson Verdict = iota + 1
	// AutoApproved lets the action run because a rule the user wrote covers it.
	AutoApproved
	// Refused stops the action: it is invalid or over a ceiling.
	Refused
)

func (v Verdict) String() string {
	switch v {
	case NeedsPerson:
		return "needs_person"
	case AutoApproved:
		return "auto_approved"
	case Refused:
		return "refused"
	}

	return "unknown"
}

// Reason is why an action was refused.
type Reason string

// The reasons of a refusal.
const (
	ReasonInvalid Reason = "invalid"
	ReasonPerItem Reason = "per_item"
	ReasonPerDay  Reason = "per_day"
)

// Scope is whose ceiling refused the action.
type Scope string

// The scopes of a ceiling.
const (
	ScopeWorkspace Scope = "workspace"
	ScopeProject   Scope = "project"
)

// Refusal is the reason of a Refused outcome, with what an actionable message needs.
type Refusal struct {
	Reason   Reason
	Scope    Scope // empty for ReasonInvalid
	Limit    int64 // the ceiling, in micros
	Spent    int64 // spent today in that scope, in micros
	Estimate int64
	Currency string
	Message  string // for ReasonInvalid
}

// Outcome is the answer of Decide.
type Outcome struct {
	Verdict Verdict
	RuleID  string   // set for AutoApproved
	Refusal *Refusal // set for Refused
}

// Decide is SDD-015 section 2. The ceilings come first and always: no rule and no person lifts one.
// A rule then approves the action when it matches; otherwise a person decides.
func Decide(a Action, rules []Rule, ceilings []Ceiling, spent Spent) Outcome {
	if msg := validAction(a); msg != "" {
		return Outcome{Verdict: Refused, Refusal: &Refusal{Reason: ReasonInvalid, Message: msg}}
	}

	if a.Cost.Micros > 0 {
		if r := overCeiling(a, ceilings, spent); r != nil {
			return Outcome{Verdict: Refused, Refusal: r}
		}
	}

	if id, ok := matchRule(a, rules, spent); ok {
		return Outcome{Verdict: AutoApproved, RuleID: id}
	}

	return Outcome{Verdict: NeedsPerson}
}

func validAction(a Action) string {
	switch {
	case a.Kind == "":
		return "the action has no kind"
	case a.Cost.Micros < 0 || a.Cost.Micros > MaxMicros:
		return "the estimated cost is out of range"
	case a.Cost.Micros > 0 && !currencyRe.MatchString(a.Cost.Currency):
		return "an estimated cost needs a currency of three upper-case letters"
	}

	return ""
}

// overCeiling returns the first ceiling the action is over: the project's before the workspace's,
// the limit per item before the limit per day.
func overCeiling(a Action, ceilings []Ceiling, spent Spent) *Refusal {
	for _, scope := range []Scope{ScopeProject, ScopeWorkspace} {
		for i := range ceilings {
			c := &ceilings[i]
			if c.Currency != a.Cost.Currency || scopeOf(c) != scope || scope == ScopeProject && c.Project != a.Project {
				continue
			}

			so := spent.Workspace
			if scope == ScopeProject {
				so = spent.Project
			}

			if c.PerItem != nil && a.Cost.Micros > *c.PerItem {
				return &Refusal{
					Reason: ReasonPerItem, Scope: scope, Limit: *c.PerItem, Spent: so, Estimate: a.Cost.Micros,
					Currency: c.Currency,
				}
			}

			if c.PerDay != nil && overDay(so, a.Cost.Micros, *c.PerDay) {
				return &Refusal{
					Reason: ReasonPerDay, Scope: scope, Limit: *c.PerDay, Spent: so, Estimate: a.Cost.Micros,
					Currency: c.Currency,
				}
			}
		}
	}

	return nil
}

func scopeOf(c *Ceiling) Scope {
	if c.Project == "" {
		return ScopeWorkspace
	}

	return ScopeProject
}

// matchRule returns the id of the most specific rule that covers the action, the first in the order
// given among equals.
func matchRule(a Action, rules []Rule, spent Spent) (string, bool) {
	best, bestScore := "", -1

	for i := range rules {
		r := &rules[i]
		if !covers(r, a, spent) {
			continue
		}

		score := 0
		if r.Connection != "" {
			score++
		}

		if r.Project != "" {
			score++
		}

		if score > bestScore {
			best, bestScore = r.ID, score
		}
	}

	return best, bestScore >= 0
}

func covers(r *Rule, a Action, spent Spent) bool {
	if !r.Enabled || r.Kind != a.Kind {
		return false
	}

	if r.Connection != "" && r.Connection != a.Connection || r.Project != "" && r.Project != a.Project {
		return false
	}

	if a.Cost.Micros == 0 {
		return true
	}

	// A rule with no limit per item never approves a costly action: a rule written for posts must not
	// approve a generation.
	if r.PerItem == nil || r.Currency != a.Cost.Currency || a.Cost.Micros > *r.PerItem {
		return false
	}

	return r.PerDay == nil || !overDay(spent.ByRule[r.ID], a.Cost.Micros, *r.PerDay)
}

// overDay reports whether spent plus cost is over limit, without adding them: a recorded cost may be the
// largest int64 (a cost that does not fit saturates there, core/provider), and the sum would wrap to a
// negative day that passes every limit.
func overDay(spent, cost, limit int64) bool {
	return spent > limit || cost > limit-max(spent, 0)
}

// FieldError is a validation failure; Path names the member of the request.
type FieldError struct {
	Path    string
	Message string
}

func (e *FieldError) Error() string { return e.Path + ": " + e.Message }

func bad(path, format string, args ...any) *FieldError {
	return &FieldError{Path: path, Message: fmt.Sprintf(format, args...)}
}

var (
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
	kindRe     = regexp.MustCompile(`^[a-z0-9_.:-]{1,80}$`)
	idRe       = regexp.MustCompile(`^[a-z]{3}_[0-9a-hjkmnp-tv-z]{10}$`)
)

func validID(prefix, s string) bool { return strings.HasPrefix(s, prefix+"_") && idRe.MatchString(s) }

// ValidKind reports whether s is a kind of action: 1 to 80 characters of [a-z0-9_.:-].
func ValidKind(s string) bool { return kindRe.MatchString(s) }

// NormalizeRule checks a rule as a person wrote it and returns it in stored form.
func NormalizeRule(r Rule) (Rule, *FieldError) {
	r.Kind = strings.TrimSpace(r.Kind)

	if !kindRe.MatchString(r.Kind) {
		return r, bad("kind", "must be 1 to 80 characters of a-z, 0-9, _, ., : and -")
	}

	if r.Connection != "" && !validID("con", r.Connection) {
		return r, bad("connection", "must be a connection id (con_ and 10 characters), or empty for any")
	}

	if r.Project != "" && !validID("prj", r.Project) {
		return r, bad("project", "must be a project id (prj_ and 10 characters), or empty for any")
	}

	if ferr := checkLimits("", r.Currency, r.PerItem, r.PerDay); ferr != nil {
		return r, ferr
	}

	return r, nil
}

// NormalizeCeiling checks a ceiling as a person wrote it.
func NormalizeCeiling(c Ceiling) (Ceiling, *FieldError) {
	if c.Project != "" && !validID("prj", c.Project) {
		return c, bad("project", "must be a project id (prj_ and 10 characters), or empty for the workspace")
	}

	if !currencyRe.MatchString(c.Currency) {
		return c, bad("currency", "must be three upper-case letters, for example USD")
	}

	if ferr := checkLimits("", c.Currency, c.PerItem, c.PerDay); ferr != nil {
		return c, ferr
	}

	return c, nil
}

func checkLimits(prefix, currency string, perItem, perDay *int64) *FieldError {
	for _, l := range []struct {
		path string
		v    *int64
	}{{"per_item_micros", perItem}, {"per_day_micros", perDay}} {
		if l.v == nil {
			continue
		}

		if *l.v < 0 || *l.v > MaxMicros {
			return bad(prefix+l.path, "must be 0 to %d", MaxMicros)
		}

		if !currencyRe.MatchString(currency) {
			return bad(prefix+"currency", "a limit needs a currency of three upper-case letters")
		}
	}

	return nil
}

// ValidSummary reports whether s is a summary of a request: valid UTF-8 of 1 to MaxSummary
// characters with no control or format character other than a line feed.
func ValidSummary(s string) bool { return validText(s, 1, MaxSummary) }

// ValidReason reports whether s is the note of a rejection: as a summary, and it may be empty.
func ValidReason(s string) bool { return validText(s, 0, MaxReason) }

// validText is the text a person reads before deciding. An agent writes it, so it must not look like
// something else: every control (Cc, which holds the C1 controls), every format character (Cf: the
// bidirectional controls, the zero-width characters, the byte order mark, the tag characters) and
// the line and paragraph separators are refused, and so is invalid UTF-8. The service refuses the
// further characters service/untrusted drops from text an agent reads.
func validText(s string, minLen, maxLen int) bool {
	if !utf8.ValidString(s) {
		return false
	}

	n := utf8.RuneCountInString(s)
	if n < minLen || n > maxLen {
		return false
	}

	for _, r := range s {
		if r != '\n' && (unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) ||
			unicode.Is(unicode.Zp, r)) {
			return false
		}
	}

	return true
}

package approval

import (
	"strings"
	"testing"
)

const (
	conA = "con_aaaaaaaaaa"
	conB = "con_bbbbbbbbbb"
	prjA = "prj_aaaaaaaaaa"
	prjB = "prj_bbbbbbbbbb"
)

func usd(micros int64) Money { return Money{Micros: micros, Currency: "USD"} }

func TestDecide(t *testing.T) {
	post := Action{Kind: "publish.post", Connection: conA, Project: prjA}
	gen := Action{Kind: "generate.image", Connection: conA, Project: prjA, Cost: usd(500_000)}

	tests := []struct {
		name     string
		action   Action
		rules    []Rule
		ceilings []Ceiling
		spent    Spent
		want     Verdict
		ruleID   string
		reason   Reason
		scope    Scope
		limit    int64
	}{
		{name: "no rule needs a person", action: post, want: NeedsPerson},
		{name: "no cost and no rule needs a person", action: gen, want: NeedsPerson},
		{
			name: "a matching rule approves a free action", action: post,
			rules: []Rule{{ID: "rul_1", Kind: "publish.post", Enabled: true}},
			want:  AutoApproved, ruleID: "rul_1",
		},
		{
			name: "a disabled rule does not match", action: post,
			rules: []Rule{{ID: "rul_1", Kind: "publish.post"}},
			want:  NeedsPerson,
		},
		{
			name: "another kind does not match", action: post,
			rules: []Rule{{ID: "rul_1", Kind: "publish.story", Enabled: true}},
			want:  NeedsPerson,
		},
		{
			name: "another connection does not match", action: post,
			rules: []Rule{{ID: "rul_1", Kind: "publish.post", Connection: conB, Enabled: true}},
			want:  NeedsPerson,
		},
		{
			name: "another project does not match", action: post,
			rules: []Rule{{ID: "rul_1", Kind: "publish.post", Project: prjB, Enabled: true}},
			want:  NeedsPerson,
		},
		{
			name: "a rule for the connection and project matches", action: post,
			rules: []Rule{{ID: "rul_1", Kind: "publish.post", Connection: conA, Project: prjA, Enabled: true}},
			want:  AutoApproved, ruleID: "rul_1",
		},
		{
			name: "the most specific rule wins over an earlier one", action: post,
			rules: []Rule{
				{ID: "rul_any", Kind: "publish.post", Enabled: true},
				{ID: "rul_con", Kind: "publish.post", Connection: conA, Enabled: true},
				{ID: "rul_both", Kind: "publish.post", Connection: conA, Project: prjA, Enabled: true},
			},
			want: AutoApproved, ruleID: "rul_both",
		},
		{
			name: "equal rules: the first wins", action: post,
			rules: []Rule{
				{ID: "rul_1", Kind: "publish.post", Project: prjA, Enabled: true},
				{ID: "rul_2", Kind: "publish.post", Connection: conA, Enabled: true},
			},
			want: AutoApproved, ruleID: "rul_1",
		},
		{
			name:   "a rule with a ceiling approves a cost within it",
			action: gen,
			rules: []Rule{
				{ID: "rul_1", Kind: "generate.image", Currency: "USD", PerItem: new(int64(500_000)), Enabled: true},
			},
			want:   AutoApproved,
			ruleID: "rul_1",
		},
		{
			name: "a rule with no limit per item never approves a costly action", action: gen,
			rules: []Rule{{ID: "rul_1", Kind: "generate.image", Enabled: true}},
			want:  NeedsPerson,
		},
		{
			name:   "a rule with only a limit per day never approves a costly action",
			action: gen,
			rules: []Rule{
				{ID: "rul_1", Kind: "generate.image", Currency: "USD", PerDay: new(int64(9_000_000)), Enabled: true},
			},
			want: NeedsPerson,
		},
		{
			name:   "a cost over the limit per item of the rule goes to a person",
			action: gen,
			rules: []Rule{
				{ID: "rul_1", Kind: "generate.image", Currency: "USD", PerItem: new(int64(499_999)), Enabled: true},
			},
			want: NeedsPerson,
		},
		{
			name:   "a rule in another currency does not match",
			action: gen,
			rules: []Rule{
				{ID: "rul_1", Kind: "generate.image", Currency: "EUR", PerItem: new(int64(9_000_000)), Enabled: true},
			},
			want: NeedsPerson,
		},
		{
			name: "a rule at its limit per day matches", action: gen,
			rules: []Rule{
				{
					ID:       "rul_1",
					Kind:     "generate.image",
					Currency: "USD",
					PerItem:  new(int64(500_000)),
					PerDay:   new(int64(1_000_000)),
					Enabled:  true,
				},
			},
			spent: Spent{ByRule: map[string]int64{"rul_1": 500_000}},
			want:  AutoApproved, ruleID: "rul_1",
		},
		{
			name: "a rule over its limit per day goes to a person", action: gen,
			rules: []Rule{
				{
					ID:       "rul_1",
					Kind:     "generate.image",
					Currency: "USD",
					PerItem:  new(int64(500_000)),
					PerDay:   new(int64(1_000_000)),
					Enabled:  true,
				},
			},
			spent: Spent{ByRule: map[string]int64{"rul_1": 500_001}},
			want:  NeedsPerson,
		},
		{
			name: "a rule over its day falls back to the next matching rule", action: gen,
			rules: []Rule{
				{
					ID:         "rul_a",
					Kind:       "generate.image",
					Connection: conA,
					Currency:   "USD",
					PerItem:    new(int64(500_000)),
					PerDay:     new(int64(500_000)),
					Enabled:    true,
				},
				{ID: "rul_b", Kind: "generate.image", Currency: "USD", PerItem: new(int64(500_000)), Enabled: true},
			},
			spent: Spent{ByRule: map[string]int64{"rul_a": 1}},
			want:  AutoApproved, ruleID: "rul_b",
		},
		{
			name: "the workspace limit per item refuses", action: gen,
			ceilings: []Ceiling{{Currency: "USD", PerItem: new(int64(499_999))}},
			want:     Refused, reason: ReasonPerItem, scope: ScopeWorkspace, limit: 499_999,
		},
		{
			name: "the workspace limit per item allows the limit itself", action: gen,
			ceilings: []Ceiling{{Currency: "USD", PerItem: new(int64(500_000))}},
			want:     NeedsPerson,
		},
		{
			name: "the workspace limit per day refuses", action: gen,
			ceilings: []Ceiling{{Currency: "USD", PerDay: new(int64(1_000_000))}},
			spent:    Spent{Workspace: 500_001},
			want:     Refused, reason: ReasonPerDay, scope: ScopeWorkspace, limit: 1_000_000,
		},
		{
			name: "the workspace limit per day allows reaching it", action: gen,
			ceilings: []Ceiling{{Currency: "USD", PerDay: new(int64(1_000_000))}},
			spent:    Spent{Workspace: 500_000},
			want:     NeedsPerson,
		},
		{
			name: "the project limit per day refuses", action: gen,
			ceilings: []Ceiling{{Project: prjA, Currency: "USD", PerDay: new(int64(600_000))}},
			spent:    Spent{Project: 200_000},
			want:     Refused, reason: ReasonPerDay, scope: ScopeProject, limit: 600_000,
		},
		{
			name: "the project limit per item refuses", action: gen,
			ceilings: []Ceiling{{Project: prjA, Currency: "USD", PerItem: new(int64(1))}},
			want:     Refused, reason: ReasonPerItem, scope: ScopeProject, limit: 1,
		},
		{
			name: "the ceiling of another project does not apply", action: gen,
			ceilings: []Ceiling{{Project: prjB, Currency: "USD", PerItem: new(int64(1))}},
			want:     NeedsPerson,
		},
		{
			name: "a ceiling in another currency does not apply", action: gen,
			ceilings: []Ceiling{{Currency: "EUR", PerItem: new(int64(1))}},
			want:     NeedsPerson,
		},
		{
			name:   "the project ceiling is read before the workspace one",
			action: gen,
			ceilings: []Ceiling{
				{Currency: "USD", PerItem: new(int64(1))},
				{Project: prjA, Currency: "USD", PerItem: new(int64(2))},
			},
			want:   Refused,
			reason: ReasonPerItem,
			scope:  ScopeProject,
			limit:  2,
		},
		{
			name:   "a ceiling refuses even when a rule would approve",
			action: gen,
			rules: []Rule{
				{ID: "rul_1", Kind: "generate.image", Currency: "USD", PerItem: new(int64(900_000)), Enabled: true},
			},
			ceilings: []Ceiling{{Currency: "USD", PerItem: new(int64(100_000))}},
			want:     Refused,
			reason:   ReasonPerItem,
			scope:    ScopeWorkspace,
			limit:    100_000,
		},
		{
			name: "a free action is never refused by a ceiling", action: post,
			ceilings: []Ceiling{{Currency: "USD", PerItem: new(int64(0)), PerDay: new(int64(0))}},
			want:     NeedsPerson,
		},
		{
			name: "a ceiling of zero refuses any cost", action: gen,
			ceilings: []Ceiling{{Currency: "USD", PerDay: new(int64(0))}},
			want:     Refused, reason: ReasonPerDay, scope: ScopeWorkspace, limit: 0,
		},
		{name: "an action with no kind is invalid", action: Action{}, want: Refused, reason: ReasonInvalid},
		{
			name: "a negative cost is invalid", action: Action{Kind: "k", Cost: usd(-1)},
			want: Refused, reason: ReasonInvalid,
		},
		{
			name: "a cost over the range is invalid", action: Action{Kind: "k", Cost: usd(MaxMicros + 1)},
			want: Refused, reason: ReasonInvalid,
		},
		{
			name: "a cost with no currency is invalid", action: Action{Kind: "k", Cost: Money{Micros: 1}},
			want: Refused, reason: ReasonInvalid,
		},
		{
			name:   "a cost with a lower-case currency is invalid",
			action: Action{Kind: "k", Cost: Money{Micros: 1, Currency: "usd"}},
			want:   Refused,
			reason: ReasonInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(tt.action, tt.rules, tt.ceilings, tt.spent)
			if got.Verdict != tt.want {
				t.Fatalf("verdict = %v, want %v (%+v)", got.Verdict, tt.want, got)
			}

			if got.RuleID != tt.ruleID {
				t.Errorf("rule = %q, want %q", got.RuleID, tt.ruleID)
			}

			if tt.want != Refused {
				if got.Refusal != nil {
					t.Errorf("a refusal on %v", got.Verdict)
				}

				return
			}

			r := got.Refusal
			if r == nil || r.Reason != tt.reason || r.Scope != tt.scope || r.Limit != tt.limit {
				t.Fatalf("refusal = %+v, want reason %s scope %s limit %d", r, tt.reason, tt.scope, tt.limit)
			}

			if tt.reason != ReasonInvalid && (r.Estimate != tt.action.Cost.Micros || r.Currency != "USD") {
				t.Errorf("the refusal does not carry the estimate: %+v", r)
			}

			if tt.reason == ReasonInvalid && r.Message == "" {
				t.Error("an invalid refusal has no message")
			}
		})
	}
}

func TestVerdictString(t *testing.T) {
	for v, want := range map[Verdict]string{
		NeedsPerson: "needs_person", AutoApproved: "auto_approved", Refused: "refused", 0: "unknown",
	} {
		if got := v.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", v, got, want)
		}
	}
}

func TestNormalizeRule(t *testing.T) {
	tests := []struct {
		name string
		in   Rule
		path string
	}{
		{"valid free rule", Rule{Kind: "publish.post"}, ""},
		{
			"valid with everything",
			Rule{
				Kind:       " generate.image ",
				Connection: conA,
				Project:    prjA,
				Currency:   "USD",
				PerItem:    new(int64(1)),
				PerDay:     new(int64(2)),
			},
			"",
		},
		{"no kind", Rule{}, "kind"},
		{"upper-case kind", Rule{Kind: "Publish"}, "kind"},
		{"long kind", Rule{Kind: strings.Repeat("a", 81)}, "kind"},
		{"bad connection", Rule{Kind: "k", Connection: "x"}, "connection"},
		{"a project id as the connection", Rule{Kind: "k", Connection: prjA}, "connection"},
		{"bad project", Rule{Kind: "k", Project: "prj_x"}, "project"},
		{"a limit with no currency", Rule{Kind: "k", PerItem: new(int64(1))}, "currency"},
		{"a negative limit", Rule{Kind: "k", Currency: "USD", PerDay: new(int64(-1))}, "per_day_micros"},
		{"a limit over the range", Rule{Kind: "k", Currency: "USD", PerItem: new(MaxMicros + 1)}, "per_item_micros"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ferr := NormalizeRule(tt.in)
			if tt.path == "" {
				if ferr != nil {
					t.Fatalf("unexpected error: %v", ferr)
				}

				if got.Kind != strings.TrimSpace(tt.in.Kind) {
					t.Errorf("kind = %q, not trimmed", got.Kind)
				}

				return
			}

			if ferr == nil || ferr.Path != tt.path {
				t.Fatalf("error = %v, want path %q", ferr, tt.path)
			}

			if ferr.Error() == "" {
				t.Error("an empty error text")
			}
		})
	}
}

func TestNormalizeCeiling(t *testing.T) {
	tests := []struct {
		name string
		in   Ceiling
		path string
	}{
		{"workspace", Ceiling{Currency: "USD", PerDay: new(int64(10))}, ""},
		{"project", Ceiling{Project: prjA, Currency: "EUR", PerItem: new(int64(10))}, ""},
		{"no currency", Ceiling{PerDay: new(int64(1))}, "currency"},
		{"bad project", Ceiling{Project: "x", Currency: "USD"}, "project"},
		{"negative", Ceiling{Currency: "USD", PerItem: new(int64(-5))}, "per_item_micros"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ferr := NormalizeCeiling(tt.in)
			if (ferr == nil) != (tt.path == "") || ferr != nil && ferr.Path != tt.path {
				t.Fatalf("error = %v, want path %q", ferr, tt.path)
			}
		})
	}
}

func TestValidSummary(t *testing.T) {
	tests := map[string]struct {
		in   string
		want bool
	}{
		"text":         {"Post the launch video to the channel", true},
		"two lines":    {"a\nb", true},
		"empty":        {"", false},
		"too long":     {strings.Repeat("a", MaxSummary+1), false},
		"at the limit": {strings.Repeat("é", MaxSummary), true},
		"control":      {"a\x00b", false},
		"escape":       {"a\x1b[31m", false},
		"delete":       {"a\x7f", false},
	}

	for name, tt := range tests {
		if got := ValidSummary(tt.in); got != tt.want {
			t.Errorf("%s: ValidSummary = %v, want %v", name, got, tt.want)
		}
	}

	if !ValidKind("publish.post") || ValidKind("Bad Kind") {
		t.Error("ValidKind is wrong")
	}
}

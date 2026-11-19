package approval

import (
	"context"
	"errors"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
)

// memStore is the part of Store that Evaluate reads, over slices.
type memStore struct {
	Store
	rules    []Rule
	ceilings []Ceiling
	entries  []Entry
}

func (m *memStore) ListRules(context.Context) ([]Rule, error)       { return m.rules, nil }
func (m *memStore) ListCeilings(context.Context) ([]Ceiling, error) { return m.ceilings, nil }

func (m *memStore) Spent(_ context.Context, q SpentQuery) (int64, error) {
	var sum int64

	for i := range m.entries {
		e := &m.entries[i]
		if e.State == LedgerReleased || e.Day != q.Day || e.Currency != q.Currency {
			continue
		}

		if q.Project != "" && e.ProjectID != q.Project || q.Kind != "" && e.Kind != q.Kind ||
			q.Connection != "" && e.ConnectionID != q.Connection {
			continue
		}

		if e.ActualMicros != nil {
			sum += *e.ActualMicros
		} else {
			sum += e.EstimateMicros
		}
	}

	return sum, nil
}

func TestPrivilegedCaller(t *testing.T) {
	tests := []struct {
		name string
		c    Caller
		want error
	}{
		{"the local token with every scope", Caller{Actor: "local:cli", Admin: true}, ErrNotPrivileged},
		{"the label of the browser sent by the token", Caller{Actor: "local:ui", Admin: true}, ErrNotPrivileged},
		{"an admin key", Caller{Actor: "key:key_aaaaaaaaaa", Admin: true, OwnerUserID: "usr_a"}, ErrNotPrivileged},
		{"the stdio MCP server", Caller{Actor: "local:mcp", Admin: true}, ErrNotPrivileged},
		{"a browser session", Caller{Actor: "local:ui", Browser: true, Admin: true}, nil},
		{"a network session", Caller{Actor: "user:usr_a", Browser: true, OwnerUserID: "usr_a"}, nil},
	}

	for _, tt := range tests {
		if got := CheckPrivileged(tt.c); !errors.Is(got, tt.want) {
			t.Errorf("%s: CheckPrivileged = %v, want %v", tt.name, got, tt.want)
		}

		if got := CheckMayDecide(tt.c, tt.c.OwnerUserID); !errors.Is(got, tt.want) {
			t.Errorf("%s: CheckMayDecide for the own request = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCheckMayDecide(t *testing.T) {
	member := Caller{Actor: "user:usr_a", Browser: true, OwnerUserID: "usr_a"}
	admin := Caller{Actor: "user:usr_b", Browser: true, OwnerUserID: "usr_b", Admin: true}
	local := Caller{Actor: "local:ui", Browser: true, Admin: true}

	if err := CheckMayDecide(member, "usr_a"); err != nil {
		t.Errorf("a member decides own request: %v", err)
	}

	if err := CheckMayDecide(member, "usr_c"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a member deciding another user request = %v, want ErrNotAllowed", err)
	}

	if err := CheckMayDecide(admin, "usr_a"); err != nil {
		t.Errorf("an administrator decides any request: %v", err)
	}

	if err := CheckMayDecide(local, ""); err != nil {
		t.Errorf("local mode has no owner: %v", err)
	}
}

func TestEvaluate(t *testing.T) {
	day := time.Date(2026, 11, 19, 23, 59, 0, 0, time.UTC)
	today := Day(day)
	conn, proj := "con_aaaaaaaaaa", "prj_aaaaaaaaaa"
	actual := int64(300_000)
	perItem, perDay, wsDay := int64(500_000), int64(600_000), int64(1_000_000)

	m := &memStore{
		rules: []Rule{
			{
				ID: "rul_a", Kind: "generate.image", Currency: "USD", PerItem: &perItem, PerDay: &perDay, Enabled: true,
			},
			{ID: "rul_free", Kind: "publish.post", Enabled: true},
		},
		ceilings: []Ceiling{{Currency: "USD", PerDay: &wsDay}},
		entries: []Entry{
			{
				Kind:           "generate.image",
				ConnectionID:   conn,
				ProjectID:      proj,
				Currency:       "USD",
				Day:            today,
				State:          LedgerSettled,
				EstimateMicros: 900_000,
				ActualMicros:   &actual,
			},
			{
				Kind: "generate.image", ConnectionID: conn, ProjectID: "prj_bbbbbbbbbb", Currency: "USD", Day: today,
				State: LedgerReserved, EstimateMicros: 200_000,
			},
			{Kind: "generate.image", Currency: "USD", Day: today, State: LedgerReleased, EstimateMicros: 9_000_000},
			{
				Kind:           "generate.image",
				Currency:       "USD",
				Day:            "2026-11-18",
				State:          LedgerSettled,
				EstimateMicros: 9_000_000,
			},
			{Kind: "generate.image", Currency: "EUR", Day: today, State: LedgerSettled, EstimateMicros: 9_000_000},
		},
	}

	gen := core.Action{
		Kind: "generate.image", Connection: conn, Project: proj, Cost: core.Money{Micros: 200_000, Currency: "USD"},
	}

	out, err := Evaluate(context.Background(), m, gen, day)
	if err != nil {
		t.Fatal(err)
	}

	// Today: 300k real (not the 900k estimate) + 200k reserved = 500k; the rule covers 500k of its 600k day.
	if out.Verdict != core.NeedsPerson {
		t.Fatalf("200k with 500k spent on a rule of 600k a day = %v, want needs_person", out.Verdict)
	}

	gen.Cost.Micros = 100_000

	if out, err = Evaluate(context.Background(), m, gen, day); err != nil || out.Verdict != core.AutoApproved ||
		out.RuleID != "rul_a" {
		t.Fatalf("100k within the day of the rule = %+v, %v", out, err)
	}

	// The workspace ceiling counts every project: 500k spent + 500k = 1M is the limit, 500_001 is over.
	gen.Cost.Micros = 500_001

	if out, err = Evaluate(context.Background(), m, gen, day); err != nil || out.Verdict != core.Refused {
		t.Fatalf("over the workspace day = %+v, %v", out, err)
	}

	if r := out.Refusal; r.Spent != 500_000 || r.Scope != core.ScopeWorkspace || r.Reason != core.ReasonPerDay {
		t.Fatalf("refusal = %+v", r)
	}

	free := core.Action{Kind: "publish.post", Connection: conn, Project: proj}
	if out, err = Evaluate(context.Background(), m, free, day); err != nil || out.RuleID != "rul_free" {
		t.Fatalf("a free action = %+v, %v", out, err)
	}
}

func TestViews(t *testing.T) {
	at := int64(1_700_000_000_000)
	row := Row{
		ID:           "apr_a",
		Actor:        "key:k",
		Kind:         "k",
		Summary:      "s",
		ConnectionID: "con_a",
		Estimate:     core.Money{Micros: 5, Currency: "USD"},
		State:        StateApproved,
		DecidedBy:    "rule:rul_a",
		RuleID:       "rul_a",
		CreatedMs:    at,
		DecidedMs:    &at,
		ExpiresMs:    at + 1000,
	}

	v := View(&row)
	if v.ConnectionID == nil || *v.ConnectionID != "con_a" || v.ProjectID != nil || v.JobID != nil ||
		v.DecidedAt == nil || !v.ExpiresAt.After(v.CreatedAt) || v.RuleID == nil || *v.RuleID != "rul_a" {
		t.Errorf("View = %+v", v)
	}

	rv := Rule{ID: "rul_a", Kind: "k", Enabled: true, CreatedBy: "local:ui", CreatedMs: at}.ViewOf()
	if rv.ConnectionID != nil || rv.ProjectID != nil || !rv.Enabled || rv.CreatedAt.UnixMilli() != at {
		t.Errorf("rule view = %+v", rv)
	}

	cv := Ceiling{Project: "prj_a", Currency: "USD", UpdatedMs: at}.ViewOf()
	if cv.ProjectID == nil || *cv.ProjectID != "prj_a" || cv.UpdatedAt.UnixMilli() != at {
		t.Errorf("ceiling view = %+v", cv)
	}
}

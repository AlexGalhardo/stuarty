package approval

import (
	"context"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
)

// The ceilings are enforced by Evaluate over the ledger: the amounts spent today come from the
// ledger entries (the real cost of what finished, the estimate of what is reserved), and a refusal
// carries what an actionable message needs (SDD-015 section 6).

var noon = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func ceilingStore(ceilings []Ceiling, entries ...Entry) *memStore {
	return &memStore{ceilings: ceilings, entries: entries}
}

func entry(project string, state string, estimate int64, actual *int64) Entry {
	return Entry{
		Kind: "generate.image", ProjectID: project, Currency: "USD", Day: Day(noon), State: state,
		EstimateMicros: estimate, ActualMicros: actual,
	}
}

func decide(t *testing.T, m *memStore, project string, micros int64) core.Outcome {
	t.Helper()

	out, err := Evaluate(context.Background(), m, core.Action{
		Kind: "generate.image", Project: project, Cost: core.Money{Micros: micros, Currency: "USD"},
	}, noon)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func TestCeiling_PerItem(t *testing.T) {
	t.Parallel()

	item := int64(1_000_000)
	m := ceilingStore([]Ceiling{{Currency: "USD", PerItem: &item}})

	if out := decide(t, m, "", item); out.Verdict != core.NeedsPerson {
		t.Errorf("an estimate at the limit = %v, want it allowed", out.Verdict)
	}

	out := decide(t, m, "", item+1)
	if out.Verdict != core.Refused || out.Refusal.Reason != core.ReasonPerItem || out.Refusal.Limit != item ||
		out.Refusal.Estimate != item+1 || out.Refusal.Scope != core.ScopeWorkspace {
		t.Errorf("an estimate over the limit = %+v", out)
	}
}

func TestCeiling_PerDay(t *testing.T) {
	t.Parallel()

	day := int64(2_000_000)
	settled := int64(700_000)

	m := ceilingStore(
		[]Ceiling{{Currency: "USD", PerDay: &day}},
		// The settled entry counts at its real cost, not its estimate; the reserved one at its estimate.
		entry("", LedgerSettled, 1_500_000, &settled),
		entry("", LedgerReserved, 800_000, nil),
		// A released entry, another day and another currency count for nothing.
		entry("", LedgerReleased, 5_000_000, nil),
		Entry{
			Kind:           "generate.image",
			Currency:       "USD",
			Day:            "2026-11-18",
			State:          LedgerSettled,
			EstimateMicros: 9_000_000,
		},
		Entry{Kind: "generate.image", Currency: "EUR", Day: Day(noon), State: LedgerSettled, EstimateMicros: 9_000_000},
	)

	// 700k + 800k = 1.5M spent: 500k more reaches the limit, 500_001 passes it.
	if out := decide(t, m, "", 500_000); out.Verdict != core.NeedsPerson {
		t.Errorf("reaching the daily limit = %v, want it allowed", out.Verdict)
	}

	out := decide(t, m, "", 500_001)
	if out.Verdict != core.Refused || out.Refusal.Reason != core.ReasonPerDay || out.Refusal.Spent != 1_500_000 ||
		out.Refusal.Limit != day {
		t.Errorf("passing the daily limit = %+v", out)
	}

	// The next UTC day starts empty.
	next, err := Evaluate(context.Background(), m, core.Action{
		Kind: "generate.image", Cost: core.Money{Micros: 2_000_000, Currency: "USD"},
	}, noon.Add(12*time.Hour))
	if err != nil || next.Verdict != core.NeedsPerson {
		t.Errorf("the next day = %+v, %v, want an empty day", next, err)
	}
}

func TestCeiling_PerProject(t *testing.T) {
	t.Parallel()

	projectDay, workspaceDay := int64(1_000_000), int64(5_000_000)

	m := ceilingStore(
		[]Ceiling{
			{Project: "prj_aaaaaaaaaa", Currency: "USD", PerDay: &projectDay},
			{Currency: "USD", PerDay: &workspaceDay},
		},
		entry("prj_aaaaaaaaaa", LedgerReserved, 900_000, nil),
		entry("prj_bbbbbbbbbb", LedgerReserved, 3_000_000, nil),
	)

	// The project is nearly at its own ceiling although the workspace has room.
	out := decide(t, m, "prj_aaaaaaaaaa", 200_000)
	if out.Verdict != core.Refused || out.Refusal.Scope != core.ScopeProject || out.Refusal.Spent != 900_000 {
		t.Errorf("a project over its ceiling = %+v", out)
	}

	// Another project is not bound by it, only by the workspace ceiling: 3.9M spent, 1.1M more fits.
	if out := decide(t, m, "prj_bbbbbbbbbb", 1_100_000); out.Verdict != core.NeedsPerson {
		t.Errorf("another project = %+v, want it allowed", out)
	}

	// The workspace ceiling counts every project: 1.1M + 3.9M = 5M is the limit.
	if out := decide(
		t,
		m,
		"prj_bbbbbbbbbb",
		1_100_001,
	); out.Verdict != core.Refused ||
		out.Refusal.Scope != core.ScopeWorkspace {
		t.Errorf("over the workspace ceiling = %+v", out)
	}
}

// Splitting a costly action into many small ones does not get past the daily ceiling.
func TestCeiling_SplitIntoSmallRequestsStillCounts(t *testing.T) {
	t.Parallel()

	day := int64(1_000_000)
	m := ceilingStore([]Ceiling{{Currency: "USD", PerDay: &day}})

	allowed := 0

	for range 20 {
		if out := decide(t, m, "", 100_000); out.Verdict == core.Refused {
			break
		}

		allowed++

		m.entries = append(m.entries, entry("", LedgerReserved, 100_000, nil))
	}

	if allowed != 10 {
		t.Errorf("%d small requests were allowed, want 10 (1M in 100k steps)", allowed)
	}
}

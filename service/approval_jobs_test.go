package service

import (
	"context"
	"testing"
)

// The lifecycle of an approvable job (waiting, approved, rejected, cancelled, expired, auto-approved)
// needs the SQLite store, which service/ may not import (ADR-0001); it is tested in
// web/app/servicetest (TestJobApproval_*). This test is the part that needs none.
func TestJobApproval_RegisteredKindsAreChecked(t *testing.T) {
	t.Parallel()

	prep := func(context.Context, JobStartRequest) (Prepared, error) { return Prepared{}, nil }
	run := func(context.Context, JobRun) (JobOutcome, error) { return JobOutcome{}, nil }

	good, err := registerKinds([]ApprovableKind{
		{Name: "publish.post", Prepare: prep, Run: run},
		{Name: "generate.image", Scope: ScopeMedia, Prepare: prep, Run: run},
	})
	if err != nil {
		t.Fatal(err)
	}

	if good["publish.post"].Scope != ScopeRender || good["generate.image"].Scope != ScopeMedia {
		t.Errorf(
			"scopes = %q and %q, want render by default and the one named",
			good["publish.post"].Scope,
			good["generate.image"].Scope,
		)
	}

	for name, kinds := range map[string][]ApprovableKind{
		"a built-in kind":   {{Name: JobExport, Prepare: prep, Run: run}},
		"ingest":            {{Name: JobIngest, Prepare: prep, Run: run}},
		"an upper-case one": {{Name: "Publish", Prepare: prep, Run: run}},
		"no name":           {{Prepare: prep, Run: run}},
		"no Prepare":        {{Name: "a.b", Run: run}},
		"no Run":            {{Name: "a.b", Prepare: prep}},
		"an unknown scope":  {{Name: "a.b", Scope: "root", Prepare: prep, Run: run}},
		"a duplicate":       {{Name: "a.b", Prepare: prep, Run: run}, {Name: "a.b", Prepare: prep, Run: run}},
	} {
		if _, err := registerKinds(kinds); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestJobApproval_OnlyAKindThatNeedsApprovalWaits(t *testing.T) {
	t.Parallel()

	s := &svc{}
	for _, kind := range builtinKinds {
		if s.kindOf(kind) != nil {
			t.Errorf("the built-in kind %s needs approval", kind)
		}
	}

	if JobFinal(JobWaitingApproval) {
		t.Error("waiting_approval is not a final state")
	}
}

package servicetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// SDD-005 section 18 check 5.
func TestCommit(t *testing.T) {
	t.Parallel()

	t.Run("base equal to current", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()

		res, err := e.addText(ui, project, track, 1, "0s", "hello")
		if err != nil {
			t.Fatal(err)
		}

		if res.Revision != 2 || res.BaseRevision != 1 || !res.Applied || res.DryRun || res.Replayed || res.Actor != ui {
			t.Errorf("result = %+v", res)
		}

		if len(res.Created) != 1 || !strings.HasPrefix(res.Created[0], "itm_") || len(res.Effective) != 1 ||
			res.Effective[0].Op != "text.add" {
			t.Errorf("created %v effective %+v", res.Created, res.Effective)
		}

		if len(res.Effective[0].Created) != 1 || res.Effective[0].Created[0] != res.Created[0] {
			t.Errorf("per-operation created = %v", res.Effective[0].Created)
		}

		if len(res.Diff) != 1 || res.Diff[0].Kind != command.KindItemAdded ||
			!strings.Contains(res.Description, "item "+res.Created[0]) {
			t.Errorf("diff %+v description %q", res.Diff, res.Description)
		}

		if h := e.head(
			project,
		); h.Revision != 2 || h.Summary.Items != 1 || h.Summary.Tracks != 1 ||
			h.Summary.DurationFrames != 30 {
			t.Errorf("head after commit = %+v", h)
		}

		// The stored operation carries the identifier the service generated.
		row, err := e.store.Operations().Get(context.Background(), project, 2)
		if err != nil || !strings.Contains(string(row.Ops), res.Created[0]) || row.Kind != service.OpEdit ||
			row.BaseRevision != 1 {
			t.Errorf("log row = %+v, %v", row, err)
		}
	})

	t.Run("base older without a conflict applies to the current document", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		e.mustEdit(
			ui,
			project,
			1,
			op(
				"text.add",
				fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"a","id":"itm_00000000aa"}`, track),
			),
		)
		e.mustEdit(
			ui,
			project,
			2,
			op(
				"text.add",
				fmt.Sprintf(`{"track_id":%q,"start":"2s","duration":"1s","text":"b","id":"itm_00000000bb"}`, track),
			),
		)

		// agent planned against r3, ui changed itm_aa in r4; the agent only touches itm_bb.
		e.mustEdit(ui, project, 3, op("item.move", `{"item_id":"itm_00000000aa","start":"4s"}`))

		res, err := e.edit(agent, project, 3, op("item.move", `{"item_id":"itm_00000000bb","start":"6s"}`))
		if err != nil || res.Revision != 5 || res.BaseRevision != 3 {
			t.Fatalf("result = %+v, %v", res, err)
		}
	})

	t.Run("base older with a conflict", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		e.mustEdit(
			ui,
			project,
			1,
			op(
				"text.add",
				fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"a","id":"itm_00000000aa"}`, track),
			),
		)
		e.mustEdit(ui, project, 2, op("item.move", `{"item_id":"itm_00000000aa","start":"4s"}`))

		_, err := e.edit(agent, project, 2, op("item.move", `{"item_id":"itm_00000000aa","start":"6s"}`))
		se := serviceError(t, err)

		if se.Code != command.CodeRevisionConflict || !se.Retryable || se.ExitCode() != 2 {
			t.Fatalf("err = %+v", se)
		}

		conflict, _ := se.Details["conflict"].(map[string]any)
		if conflict["collection"] != "items" || conflict["id"] != "itm_00000000aa" ||
			conflict["revision"] != int64(3) ||
			conflict["actor"] != ui {
			t.Errorf("conflict = %v", conflict)
		}

		if se.Details["base_revision"] != int64(2) || se.Details["current_revision"] != int64(3) ||
			se.Details["diff_truncated"] != false {
			t.Errorf("details = %v", se.Details)
		}

		if !strings.Contains(se.Hint, "details.diff") {
			t.Errorf("the hint does not name details.diff: %q", se.Hint)
		}

		diff, _ := se.Details["diff"].(string)
		if !strings.HasPrefix(diff, "r3 "+ui+"\n") || !strings.Contains(diff, "itm_00000000aa") {
			t.Errorf("diff text = %q", diff)
		}

		if h := e.head(project); h.Revision != 3 {
			t.Errorf("a refused batch moved the project to r%d", h.Revision)
		}
	})

	t.Run("the same actor on an older base never conflicts with itself", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		e.mustEdit(ui, project, 1, op("text.add",
			fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"a","id":"itm_00000000aa"}`, track)))

		// Two batches planned against r2 by one actor, both on the same item.
		first, err := e.edit(agent, project, 2, op("item.move", `{"item_id":"itm_00000000aa","start":"4s"}`))
		if err != nil || first.Revision != 3 {
			t.Fatalf("first = %+v, %v", first, err)
		}

		second, err := e.edit(agent, project, 2, op("item.move", `{"item_id":"itm_00000000aa","start":"6s"}`))
		if err != nil || second.Revision != 4 {
			t.Fatalf("second = %+v, %v", second, err)
		}
	})

	t.Run("base newer is a validation error", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()

		_, err := e.addText(ui, project, track, 7, "0s", "x")
		if se := serviceError(t, err); se.Code != command.CodeValidation || se.Path != "base_revision" {
			t.Errorf("err = %+v", se)
		}
	})

	t.Run("a failing operation writes nothing", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		before, ops := string(e.document(project)), e.operationCount(project)

		_, err := e.edit(ui, project, 1,
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"ok"}`, track)),
			op("item.move", `{"item_id":"itm_0000000999","start":"1s"}`))

		se := serviceError(t, err)
		if se.OpIndex == nil || *se.OpIndex != 1 || se.Code != command.CodeNotFound {
			t.Errorf("err = %+v, want op_index 1", se)
		}

		if string(e.document(project)) != before || e.operationCount(project) != ops || e.head(project).Revision != 1 {
			t.Error("a failed batch changed the project")
		}
	})

	t.Run("dry run", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		before, published := string(e.document(project)), len(e.events)

		res, err := e.svc.Edit(e.as(ui), service.EditRequest{
			ProjectID:    project,
			BaseRevision: 1,
			DryRun:       true,
			Operations: []command.Operation{
				op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"x"}`, track)),
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		if res.Applied || !res.DryRun || res.Revision != 2 || len(res.Diff) != 1 { // would commit as r2
			t.Errorf("dry run = %+v", res)
		}

		if string(e.document(project)) != before || e.head(project).Revision != 1 || len(e.events) != published {
			t.Error("a dry run wrote or published something")
		}
	})

	t.Run("shape checks", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, _ := e.withTrack()
		many := make([]command.Operation, 201)

		for i := range many {
			many[i] = op("track.add", `{"kind":"visual"}`)
		}

		for name, tt := range map[string]struct {
			req  service.EditRequest
			code string
		}{
			"no operations":  {service.EditRequest{ProjectID: project, BaseRevision: 1}, command.CodeValidation},
			"201 operations": {service.EditRequest{ProjectID: project, BaseRevision: 1, Operations: many}, service.CodePayloadTooLarge},
			"huge arguments": {service.EditRequest{ProjectID: project, BaseRevision: 1, Operations: []command.Operation{op("track.add", `{"name":"`+strings.Repeat("x", 600<<10)+`"}`)}}, service.CodePayloadTooLarge},
			"negative base":  {service.EditRequest{ProjectID: project, BaseRevision: -1, Operations: many[:1]}, command.CodeValidation},
			"short key":      {service.EditRequest{ProjectID: project, BaseRevision: 1, Operations: many[:1], IdempotencyKey: "short"}, command.CodeValidation},
			"bad key chars":  {service.EditRequest{ProjectID: project, BaseRevision: 1, Operations: many[:1], IdempotencyKey: "has space in it"}, command.CodeValidation},
			"bad json args":  {service.EditRequest{ProjectID: project, BaseRevision: 1, Operations: []command.Operation{op("track.add", `{`)}}, command.CodeValidation},
		} {
			if _, err := e.svc.Edit(e.as(ui), tt.req); code(err) != tt.code {
				t.Errorf("%s: err = %v, want %s", name, err, tt.code)
			}
		}
	})

	t.Run("a lagging base is refused without reading the log", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t)
		project, track := e.withTrack()
		id := "itm_00000000aa"
		e.mustEdit(
			ui,
			project,
			1,
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"a","id":%q}`, track, id)),
		)

		for i := range 1002 {
			e.mustEdit(
				ui,
				project,
				int64(2+i),
				op("item.move", fmt.Sprintf(`{"item_id":%q,"start":"%df"}`, id, 40+i%2*40)),
			)
		}

		_, err := e.edit(agent, project, 1, op("item.move", fmt.Sprintf(`{"item_id":%q,"start":"1s"}`, id)))
		se := serviceError(t, err)

		if se.Code != command.CodeRevisionConflict || se.Details["conflict"] != nil || se.Hint == "" {
			t.Errorf("err = %+v", se)
		}
	})
}

// SDD-005 section 18 check 9 and SDD-007 T-24.
func TestSecurity_T24_IdempotencyScopedToActor(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack()
	key := "retry-key-0001"
	add := op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"once"}`, track))

	send := func(actor string, base int64, ops ...command.Operation) (*service.EditResult, error) {
		return e.svc.Edit(
			e.as(actor),
			service.EditRequest{ProjectID: project, BaseRevision: base, Operations: ops, IdempotencyKey: key},
		)
	}

	first, err := send(ui, 1, add)
	if err != nil || first.Replayed {
		t.Fatalf("first = %+v, %v", first, err)
	}

	rows := e.operationCount(project)

	// The same key and operations return the stored result and write nothing, even
	// with a newer base revision (it is not part of the request hash).
	again, err := send(ui, 2, add)
	if err != nil || !again.Replayed || again.Revision != first.Revision ||
		strings.Join(again.Created, ",") != strings.Join(first.Created, ",") {
		t.Fatalf("replay = %+v, %v", again, err)
	}

	if e.operationCount(project) != rows || e.head(project).Revision != 2 {
		t.Error("a replay wrote something")
	}

	// Different operations under the same key.
	other := op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"3s","duration":"1s","text":"other"}`, track))
	if _, err := send(ui, 2, other); code(err) != command.CodeIdempotencyMismatch {
		t.Errorf("different operations = %v, want E_IDEMPOTENCY_MISMATCH", err)
	}

	// Another actor may use the same string: it does not read the first actor's result.
	foreign, err := send(agent, 2, other)
	if err != nil || foreign.Replayed || foreign.Actor != agent || foreign.Revision != 3 {
		t.Errorf("another actor's key = %+v, %v; want its own commit", foreign, err)
	}

	// An expired key runs again.
	e.clock.advance(24*time.Hour + time.Second)

	later := op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"6s","duration":"1s","text":"renewed"}`, track))

	renewed, err := send(ui, 3, later)
	if err != nil || renewed.Replayed || renewed.Revision != 4 {
		t.Errorf("expired key = %+v, %v; want a fresh commit", renewed, err)
	}

	// A failed batch stores nothing: the retry with the same key runs again.
	key = "retry-key-0002"

	bad := op("item.move", `{"item_id":"itm_0000000999","start":"1s"}`)
	if _, err := send(ui, 4, bad); code(err) != command.CodeNotFound {
		t.Fatalf("bad batch = %v", err)
	}

	if _, err := send(ui, 4, bad); code(err) != command.CodeNotFound {
		t.Errorf("retry of a failed batch = %v, want it to run again and fail again", err)
	}
}

// SDD-005 section 18 check 8: commits to one project are strictly ordered and
// so are the events they publish.
func TestConcurrentCommitsKeepRevisionOrder(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	hot, hotTrack := e.withTrack()
	cold, coldTrack := e.withTrack()

	const perProject = 8

	var wg sync.WaitGroup

	for i := range perProject {
		for _, p := range [][2]string{{hot, hotTrack}, {cold, coldTrack}} {
			wg.Go(func() {
				// Each goroutine plans against the revision it read, like a real client: a
				// stale base without a conflict still applies, so every commit lands.
				text := fmt.Sprintf("t%d", i)
				for {
					h, err := e.store.Projects().Head(context.Background(), p[0])
					if err != nil {
						t.Error(err)
						return
					}

					_, err = e.edit(agent, p[0], h.Revision, op("text.add",
						fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":%q}`, p[1], i*2, text)))
					if err == nil {
						return
					}

					if code(err) != command.CodeOverlap && code(err) != command.CodeRevisionConflict {
						t.Error(err)
						return
					}
					// Overlap: another goroutine took that second first; try the next ones.
					i += perProject
				}
			})
		}
	}

	wg.Wait()

	for _, p := range []string{hot, cold} {
		h := e.head(p)
		if h.Revision != perProject+1 {
			t.Errorf("%s at r%d, want r%d", p, h.Revision, perProject+1)
		}

		rows, _, err := e.store.Operations().Page(context.Background(), p, service.HistoryQuery{Limit: 100})
		if err != nil || len(rows) != perProject+1 {
			t.Fatalf("%s has %d log rows, %v", p, len(rows), err)
		}

		for i, r := range rows {
			if r.Revision != int64(perProject+1-i) {
				t.Errorf("%s: row %d is revision %d, want contiguous", p, i, r.Revision)
			}
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	last := map[string]int64{}

	for _, ev := range e.events {
		if ev.Type == "batch.committed" && ev.Revision <= last[ev.ProjectID] {
			t.Errorf("event r%d of %s after r%d: out of order", ev.Revision, ev.ProjectID, last[ev.ProjectID])
		}

		last[ev.ProjectID] = ev.Revision
	}
}

// The new lint findings a batch introduced come back with the totals.
func TestEditReportsNewLintFindings(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, track := e.withTrack()

	res := e.mustEdit(
		ui,
		project,
		1,
		op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"hi"}`, track)),
	)

	if res.Lint.Errors != 0 || res.Lint.Warnings == 0 {
		t.Errorf("totals = %+v", res.Lint)
	}

	if res.Lint.LintScope != "fast" {
		t.Errorf("lint_scope = %q, want fast (VV-082)", res.Lint.LintScope)
	}

	codes := map[string]bool{}
	for _, f := range res.Findings {
		codes[f.Code] = true
	}

	if !codes["L_SILENT_EXPORT"] {
		t.Errorf("findings = %+v, want L_SILENT_EXPORT among them", res.Findings)
	}

	// The same finding is not new on the next edit.
	next := e.mustEdit(ui, project, 2, op("text.set", fmt.Sprintf(`{"item_id":%q,"text":"hello"}`, res.Created[0])))
	for _, f := range next.Findings {
		if f.Code == "L_SILENT_EXPORT" {
			t.Errorf("an old finding was reported as new: %+v", f)
		}
	}

	full, err := e.svc.Lint(e.as(agent), service.LintRequest{ProjectID: project})
	if err != nil || full.Warnings == 0 || !strings.HasPrefix(full.Text, "lint r3 "+project) {
		t.Errorf("lint = %+v, %v", full, err)
	}

	if _, err := e.svc.Lint(
		e.as(agent),
		service.LintRequest{ProjectID: project, Severity: "nope"},
	); code(
		err,
	) != command.CodeValidation {
		t.Errorf("bad severity = %v", err)
	}

	onlyErrors, err := e.svc.Lint(e.as(agent), service.LintRequest{ProjectID: project, Severity: "error"})
	if err != nil || len(onlyErrors.Findings) != 0 || onlyErrors.Warnings != full.Warnings {
		t.Errorf("errors only = %+v, %v; totals still cover every finding", onlyErrors, err)
	}
}

// SDD-007 section 9.1, T-24: a base more than 1,000 revisions behind is
// refused from the head alone, whatever the size of the log, and says what the
// current revision is.
func TestSecurity_T24_RevisionLookbackBounded(t *testing.T) {
	e := newEnv(t)
	project, track := e.withTrack()

	first := e.mustEdit(ui, project, e.head(project).Revision, op(
		"text.add",
		fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"mover"}`, track),
	))
	item := first.Created[0]

	for i := range 1002 {
		e.mustEdit(ui, project, e.head(project).Revision, op(
			"item.move", fmt.Sprintf(`{"item_id":%q,"start":"%ds"}`, item, 1+i%2),
		))
	}

	start := time.Now()
	_, err := e.edit(agent, project, 0, op(
		"text.add",
		fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"late"}`, track),
	))
	took := time.Since(start)

	se, ok := errors.AsType[*service.Error](err)
	if !ok || se.Code != command.CodeRevisionConflict || se.Details["current_revision"] != e.head(project).Revision {
		t.Fatalf("a base 1,000 behind = %v", err)
	}

	if _, has := se.Details["diff"]; has {
		t.Error("the refusal read the log: it carries a diff")
	}

	if took > 250*time.Millisecond {
		t.Errorf("the refusal took %v; it must not depend on the log", took)
	}
}

// VV-050: a batch that changes nothing commits no revision and no undo step.
func TestEditThatChangesNothingCommitsNothing(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, _ := e.withTrack()
	before := e.head(project).Revision
	events := len(e.events)
	same := op("project.rename", `{"name":"p"}`)

	res, err := e.edit(ui, project, before, same)
	if err != nil {
		t.Fatal(err)
	}

	if res.Applied || res.Revision != before || len(res.Diff) != 0 || len(res.Effective) != 1 {
		t.Errorf("no-op result = %+v", res)
	}

	if h := e.head(project); h.Revision != before || len(e.events) != events {
		t.Errorf("a no-op committed or published: head %+v, events %d to %d", h, events, len(e.events))
	}

	dry, err := e.svc.Edit(e.as(ui), service.EditRequest{
		ProjectID: project, BaseRevision: before, DryRun: true, Operations: []command.Operation{same},
	})
	if err != nil || dry.Applied || !dry.DryRun || dry.Revision != before {
		t.Errorf("no-op dry run = %+v, %v", dry, err)
	}
}

// The created ids of a batch are in operation order (SDD-003), not in the order
// of the diff, which sorts them by id.
func TestEditListsCreatedIDsInOperationOrder(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project, _ := e.withTrack()
	base := e.head(project).Revision

	res, err := e.edit(ui, project, base,
		op("track.add", `{"kind":"visual","name":"Z","id":"trk_zzzzzzzzzz"}`),
		op("track.add", `{"kind":"visual","name":"A","id":"trk_aaaaaaaaaa"}`),
		op("track.add", `{"kind":"audio","name":"M","id":"trk_mmmmmmmmmm"}`),
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"trk_zzzzzzzzzz", "trk_aaaaaaaaaa", "trk_mmmmmmmmmm"}
	if !slices.Equal(res.Created, want) {
		t.Errorf("created = %v, want %v", res.Created, want)
	}
}

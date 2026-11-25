package servicetest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/service"
)

// The agent flow of a recording (SDD-025 section 19): prepare, mark, propose; a person starts and decides.

func prepareReq(project string) service.RecordingPrepareRequest {
	return service.RecordingPrepareRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1", Name: "Main"}, FPS: 30,
		Microphone: "default", Events: true, Notes: "Say the hook\x1b[31m first.\n\n\n\nThen the demo.",
	}
}

func (r *recEnv) stopReady(t *testing.T, id string) {
	t.Helper()

	if _, err := r.svc.RecordingStop(personCtx(), service.RecordingRef{ID: id}); err != nil {
		t.Fatal(err)
	}

	r.waitState(t, id, "ready")
}

// readyTake is a recording that is ready and not added.
func (r *recEnv) readyTake(t *testing.T) (project, id string) {
	t.Helper()

	project = r.newProject("p")
	rec := r.start(t, project)
	r.stopReady(t, rec.ID)

	return project, rec.ID
}

// placedTake is a recording a person added, placed on the timeline of its project.
func (r *recEnv) placedTake(t *testing.T) (project, id string) {
	t.Helper()

	project, id = r.readyTake(t)

	if _, err := r.svc.RecordingConfirm(personCtx(), service.RecordingRef{ID: id}); err != nil {
		t.Fatal(err)
	}

	var err error

	pollUntil(10*time.Second, func() bool {
		_, err = r.svc.RecordingPlace(r.as(agent), service.RecordingPlaceRequest{ID: id})

		return err == nil || code(err) != "E_ASSET_NOT_READY"
	})

	if err != nil {
		t.Fatalf("RecordingPlace: %v", err)
	}

	return project, id
}

func (r *recEnv) docJSON(t *testing.T, project string) string {
	t.Helper()

	b, err := json.Marshal(r.document2(t, project))
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

func TestRecordingPrepareStartsNothingAndAPersonStartsIt(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	rec, err := r.svc.RecordingPrepare(r.as(agent), prepareReq(project))
	if err != nil {
		t.Fatalf("RecordingPrepare: %v", err)
	}

	if rec.State != "prepared" || rec.Plan == nil || rec.Plan.FPS != 30 || rec.Plan.Microphone != "default" ||
		!rec.Plan.Events || strings.ContainsRune(rec.Plan.Notes, 0x1b) || strings.Contains(rec.Plan.Notes, "\n\n\n") {
		t.Fatalf("prepared = %+v", rec)
	}

	if len(rec.Tracks) != 2 || rec.Source.Name != "Main" || !strings.Contains(rec.Text, "a person presses record") {
		t.Errorf("tracks %v, source %+v, text %q", rec.Tracks, rec.Source, rec.Text)
	}

	if n := r.cap.starts.Load(); n != 0 {
		t.Fatalf("preparing started the helper %d times", n)
	}

	if open, oerr := r.store.Recordings().Open(context.Background()); oerr != nil || len(open) != 0 {
		t.Fatalf("a prepared recording is not open: %v %v", open, oerr)
	}

	// an agent cannot start it, with its id or in any other way
	for _, req := range []service.RecordingStartRequest{
		{RecordingID: rec.ID},
		{ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"}},
	} {
		if _, serr := r.svc.RecordingStart(r.as(agent), req); code(serr) != "E_PRIVILEGED_SESSION" {
			t.Errorf("an agent starts %+v: %v", req, serr)
		}
	}

	if r.cap.starts.Load() != 0 {
		t.Fatal("a refused start reached the helper")
	}

	// a person gives the id and something else: ambiguous
	if _, serr := r.svc.RecordingStart(personCtx(), service.RecordingStartRequest{
		RecordingID: rec.ID, FPS: 60,
	}); code(serr) != "E_VALIDATION" {
		t.Errorf("an id with another member: %v", serr)
	}

	// another recording is not blocked by a plan that waits
	other := r.start(t, project)
	r.stopReady(t, other.ID)

	started, err := r.svc.RecordingStart(personCtx(), service.RecordingStartRequest{RecordingID: rec.ID})
	if err != nil {
		t.Fatalf("a person starts the prepared recording: %v", err)
	}

	t.Cleanup(func() { r.cap.last().end(capturehost.Lost{Err: capturehost.ErrLost}) })

	if started.ID != rec.ID || started.State != "recording" || started.Plan != nil || started.Source.Name != "Main" {
		t.Fatalf("started = %+v", started)
	}

	if r.cap.starts.Load() != 2 {
		t.Fatalf("starts = %d, want 2 (the other recording and this one)", r.cap.starts.Load())
	}

	if _, serr := r.svc.RecordingStart(
		personCtx(),
		service.RecordingStartRequest{RecordingID: rec.ID},
	); code(
		serr,
	) != "E_INVALID_STATE" &&
		code(serr) != "E_QUEUE_FULL" {
		t.Errorf("the same plan started twice: %v", serr)
	}
}

func TestRecordingPrepareValidatesAndIsLimited(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	cases := map[string]func(*service.RecordingPrepareRequest){
		"source.kind": func(q *service.RecordingPrepareRequest) { q.Source.Kind = "tab" },
		"fps":         func(q *service.RecordingPrepareRequest) { q.FPS = 29 },
		"script_id":   func(q *service.RecordingPrepareRequest) { q.ScriptID = "nope" },
		"notes":       func(q *service.RecordingPrepareRequest) { q.Notes = strings.Repeat("x", 2001) },
		"project_id":  func(q *service.RecordingPrepareRequest) { q.ProjectID = "prj_zzzzzzzzzz" },
	}

	for field, mutate := range cases {
		q := prepareReq(project)
		mutate(&q)

		_, err := r.svc.RecordingPrepare(r.as(agent), q)
		if field == "project_id" {
			if code(err) != "E_NOT_FOUND" {
				t.Errorf("an unknown project: %v", err)
			}

			continue
		}

		if code(err) != "E_VALIDATION" || serviceError(t, err).Path != field {
			t.Errorf("%s: %v", field, err)
		}
	}

	// a script id that looks right but is no script of the project
	q := prepareReq(project)
	q.ScriptID = "pit_0000000000"

	if _, err := r.svc.RecordingPrepare(r.as(agent), q); code(err) != "E_NOT_FOUND" {
		t.Errorf("a script that does not exist: %v", err)
	}

	for range 20 {
		if _, err := r.svc.RecordingPrepare(r.as(agent), prepareReq(project)); err != nil {
			t.Fatal(err)
		}
	}

	_, err := r.svc.RecordingPrepare(r.as(agent), prepareReq(project))
	if code(err) != "E_LIMIT_EXCEEDED" || serviceError(t, err).Details["limit"] != "prepared_recordings" {
		t.Fatalf("the 21st: %v", err)
	}

	// an agent without the media scope cannot prepare
	noMedia := service.WithCaller(
		context.Background(),
		service.Caller{Actor: agent, Scopes: []string{service.ScopeRead}},
	)
	if _, err := r.svc.RecordingPrepare(noMedia, prepareReq(project)); code(err) != "E_FORBIDDEN" &&
		code(err) != "E_INSUFFICIENT_SCOPE" {
		t.Errorf("without media: %v", err)
	}

	if r.cap.starts.Load() != 0 {
		t.Fatal("the helper was started")
	}
}

func TestRecordingCancelOfAPreparedRecordingByAnAgent(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	rec, err := r.svc.RecordingPrepare(r.as(agent), prepareReq(project))
	if err != nil {
		t.Fatal(err)
	}

	got, err := r.svc.RecordingCancel(r.as(agent), service.RecordingRef{ID: rec.ID})
	if err != nil || got.State != "cancelled" || got.Plan != nil {
		t.Fatalf("cancel = %+v %v", got, err)
	}

	if _, err := r.svc.RecordingStart(
		personCtx(),
		service.RecordingStartRequest{RecordingID: rec.ID},
	); code(
		err,
	) != "E_INVALID_STATE" {
		t.Errorf("start of a cancelled plan: %v", err)
	}
}

func TestRecordingMarks(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	plan, err := r.svc.RecordingPrepare(r.as(agent), prepareReq(project))
	if err != nil {
		t.Fatal(err)
	}

	mark := func(id, kind string, at, end int64, note string) (*service.Recording, error) {
		return r.svc.RecordingMark(r.as(agent), service.RecordingMarkRequest{
			ID: id, Kind: kind, AtMS: at, EndMS: end, Note: note,
		})
	}

	if _, err := mark(plan.ID, "good", 10, 0, ""); code(err) != "E_INVALID_STATE" {
		t.Errorf("a mark on a plan: %v", err)
	}

	rec := r.start(t, project)

	got, err := mark(rec.ID, "mistake", 1500, 0, "stumbled\x1b[2J here")
	if err != nil || len(got.Marks) != 1 || got.Marks[0].Kind != "mistake" || got.Marks[0].Actor != agent ||
		strings.ContainsRune(got.Marks[0].Note, 0x1b) {
		t.Fatalf("a mark while recording: %+v %v", got, err)
	}

	r.stopReady(t, rec.ID)

	got, err = mark(rec.ID, "good", 100, 900, "clean take")
	if err != nil || len(got.Marks) != 2 || got.Marks[1].N != 2 || got.Marks[1].EndMS != 900 {
		t.Fatalf("a mark on a ready take: %+v %v", got, err)
	}

	for name, req := range map[string]service.RecordingMarkRequest{
		"kind":   {ID: rec.ID, Kind: "great", AtMS: 1},
		"at_ms":  {ID: rec.ID, Kind: "bad", AtMS: 999999},
		"end_ms": {ID: rec.ID, Kind: "bad", AtMS: 500, EndMS: 400},
	} {
		if _, err := r.svc.RecordingMark(
			r.as(agent),
			req,
		); code(err) != "E_VALIDATION" ||
			serviceError(t, err).Path != name {
			t.Errorf("%s: %v", name, err)
		}
	}

	// a mark changes no timeline and no file: the read shows it with the recording
	read, err := r.svc.RecordingGet(r.as(agent), service.RecordingRef{ID: rec.ID})
	if err != nil || len(read.Marks) != 2 || !strings.Contains(read.Text, "2 marks") {
		t.Errorf("read = %+v %v", read, err)
	}

	for i := 2; i < 500; i++ {
		if _, err := mark(rec.ID, "bad", int64(i), 0, ""); err != nil {
			t.Fatalf("mark %d: %v", i, err)
		}
	}

	if _, err := mark(rec.ID, "bad", 1, 0, ""); code(err) != "E_LIMIT_EXCEEDED" ||
		serviceError(t, err).Details["limit"] != "recording_marks" {
		t.Errorf("the 501st mark: %v", err)
	}

	// set aside by a person, the recording is not found by an agent, so it takes no mark either
	if _, err := r.svc.RecordingDismiss(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := mark(rec.ID, "good", 1, 0, ""); code(err) != "E_RECORDING_NOT_FOUND" {
		t.Errorf("a mark on a dismissed recording: %v", err)
	}
}

func ranges(from ...int64) []service.RecordingCutRange {
	var out []service.RecordingCutRange

	for i := 0; i+1 < len(from); i += 2 {
		out = append(
			out,
			service.RecordingCutRange{FromMS: from[i], ToMS: from[i+1], Reason: fmt.Sprintf("reason %d", i/2)},
		)
	}

	return out
}

// T-166: a proposal writes the row of the recording and nothing else; no revision moves.
func TestSecurity_T166_AProposalChangesNoTimeline(t *testing.T) {
	r := newRecEnv(t)
	project, id := r.placedTake(t)

	before, revBefore := r.docJSON(t, project), r.revision(t, project)

	got, err := r.svc.RecordingProposeCuts(r.as(agent), service.RecordingProposeCutsRequest{
		ID: id, Ranges: ranges(200, 400, 1000, 1200),
	})
	if err != nil || len(got.Proposals) != 1 {
		t.Fatalf("propose = %+v %v", got, err)
	}

	p := got.Proposals[0]
	if !strings.HasPrefix(p.ID, "prp_") || p.State != "pending" || p.Actor != agent || len(p.Ranges) != 2 ||
		!strings.Contains(got.Text, "Proposal "+p.ID) {
		t.Fatalf("proposal = %+v, text %q", p, got.Text)
	}

	if r.revision(t, project) != revBefore || r.docJSON(t, project) != before {
		t.Fatal("a proposal changed the timeline")
	}
}

func TestRecordingProposeCutsRulesAndLimits(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	running := r.start(t, project)
	if _, err := r.svc.RecordingProposeCuts(r.as(agent), service.RecordingProposeCutsRequest{
		ID: running.ID, Ranges: ranges(0, 500),
	}); code(err) != "E_INVALID_STATE" {
		t.Errorf("a proposal for a recording that runs: %v", err)
	}

	r.stopReady(t, running.ID)

	bad := map[string][]service.RecordingCutRange{
		"ranges":            nil,
		"ranges[0].to_ms":   ranges(100, 150),
		"ranges[0].from_ms": ranges(1900, 2500),
		"ranges[1].from_ms": ranges(100, 600, 500, 900),
		"ranges[0].reason":  {{FromMS: 100, ToMS: 600, Reason: "  "}},
	}

	for field, rs := range bad {
		_, err := r.svc.RecordingProposeCuts(
			r.as(agent),
			service.RecordingProposeCutsRequest{ID: running.ID, Ranges: rs},
		)
		if code(err) != "E_VALIDATION" || serviceError(t, err).Path != field {
			t.Errorf("%s: %v", field, err)
		}
	}

	for range 5 {
		if _, err := r.svc.RecordingProposeCuts(r.as(agent), service.RecordingProposeCutsRequest{
			ID: running.ID, Ranges: ranges(100, 600),
		}); err != nil {
			t.Fatal(err)
		}
	}

	_, err := r.svc.RecordingProposeCuts(
		r.as(agent),
		service.RecordingProposeCutsRequest{ID: running.ID, Ranges: ranges(100, 600)},
	)
	if code(err) != "E_LIMIT_EXCEEDED" || serviceError(t, err).Details["limit"] != "recording_proposals" {
		t.Errorf("the sixth pending proposal: %v", err)
	}
}

func TestRecordingProposalDecideCutsOneBatchAndOneUndoRestores(t *testing.T) {
	r := newRecEnv(t)
	project, id := r.placedTake(t)

	prop, err := r.svc.RecordingProposeCuts(r.as(agent), service.RecordingProposeCutsRequest{
		ID: id, Ranges: ranges(200, 400, 1000, 1200),
	})
	if err != nil {
		t.Fatal(err)
	}

	pid := prop.Proposals[0].ID

	// an agent never decides, whatever it names
	for _, req := range []service.RecordingProposalDecideRequest{
		{ID: id, ProposalID: pid, Accept: []bool{true, true}},
		{ID: "rec_zzzzzzzzzz", ProposalID: "prp_zzzzzzzzzz"},
	} {
		if _, derr := r.svc.RecordingProposalDecide(r.as(agent), req); code(derr) != "E_PRIVILEGED_SESSION" {
			t.Errorf("an agent decides %+v: %v", req, derr)
		}
	}

	before, revBefore := r.docJSON(t, project), r.revision(t, project)
	itemsBefore := r.document2(t, project).Items

	if _, derr := r.svc.RecordingProposalDecide(personCtx(), service.RecordingProposalDecideRequest{
		ID: id, ProposalID: pid, Accept: []bool{true},
	}); code(derr) != "E_VALIDATION" {
		t.Errorf("one decision for two ranges: %v", derr)
	}

	if _, derr := r.svc.RecordingProposalDecide(personCtx(), service.RecordingProposalDecideRequest{
		ID: id, ProposalID: "prp_zzzzzzzzzz", Accept: []bool{true, true},
	}); code(derr) != "E_NOT_FOUND" {
		t.Errorf("an unknown proposal: %v", derr)
	}

	if r.revision(t, project) != revBefore {
		t.Fatal("a refused decision moved the revision")
	}

	res, err := r.svc.RecordingProposalDecide(personCtx(), service.RecordingProposalDecideRequest{
		ID: id, ProposalID: pid, Accept: []bool{true, false},
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if res.Edit == nil || !res.Edit.Applied || res.Edit.Revision != revBefore+1 {
		t.Fatalf("edit = %+v, want one batch at revision %d", res.Edit, revBefore+1)
	}

	got := res.Recording.Proposals[0]
	if got.State != "accepted" || got.Ranges[0].Decision != "accepted" || got.Ranges[1].Decision != "rejected" ||
		got.Revision != revBefore+1 || got.DecidedBy == "" {
		t.Fatalf("decided = %+v", got)
	}

	if r.revision(t, project) != revBefore+1 || r.docJSON(t, project) == before {
		t.Fatal("the accepted range did not change the timeline in one revision")
	}

	// the accepted range (200 to 400 ms, 6 frames at 30 fps) left the picture: the clip is that much shorter
	itemsAfter := r.document2(t, project).Items

	total := func(items map[string]timeline.Item) (n int) {
		for _, it := range items {
			if it.Type == timeline.ItemClip {
				n += it.Duration
			}
		}

		return n
	}

	if d := total(itemsBefore) - total(itemsAfter); d < 5 || d > 7 {
		t.Errorf("the cut removed %d frames, want 6", d)
	}

	// one undo restores it
	if _, err := r.svc.Undo(r.as(ui), service.UndoRequest{ProjectID: project}); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if after := r.docJSON(t, project); after != before {
		t.Errorf("one undo did not restore the timeline:\nbefore %s\nafter  %s", before, after)
	}

	// decided once
	if _, err := r.svc.RecordingProposalDecide(personCtx(), service.RecordingProposalDecideRequest{
		ID: id, ProposalID: pid, Accept: []bool{true, true},
	}); code(err) != "E_INVALID_STATE" {
		t.Errorf("a second decision: %v", err)
	}
}

func TestRecordingProposalRejectedTouchesNoTimeline(t *testing.T) {
	r := newRecEnv(t)
	project, id := r.placedTake(t)

	prop, err := r.svc.RecordingProposeCuts(
		r.as(agent),
		service.RecordingProposeCutsRequest{ID: id, Ranges: ranges(200, 400)},
	)
	if err != nil {
		t.Fatal(err)
	}

	rev := r.revision(t, project)

	res, err := r.svc.RecordingProposalDecide(personCtx(), service.RecordingProposalDecideRequest{
		ID: id, ProposalID: prop.Proposals[0].ID, Accept: []bool{false},
	})
	if err != nil || res.Edit != nil || res.Recording.Proposals[0].State != "rejected" ||
		r.revision(t, project) != rev {
		t.Fatalf("reject = %+v %v", res, err)
	}
}

func TestRecordingProposalNeedsTheTakeOnTheTimeline(t *testing.T) {
	r := newRecEnv(t)
	_, id := r.readyTake(t)

	prop, err := r.svc.RecordingProposeCuts(
		r.as(agent),
		service.RecordingProposeCutsRequest{ID: id, Ranges: ranges(200, 400)},
	)
	if err != nil {
		t.Fatal(err)
	}

	// not added to the project yet
	_, err = r.svc.RecordingProposalDecide(personCtx(), service.RecordingProposalDecideRequest{
		ID: id, ProposalID: prop.Proposals[0].ID, Accept: []bool{true},
	})
	if code(err) != "E_INVALID_STATE" {
		t.Errorf("a cut of a take that is not in the project: %v", err)
	}

	// still pending: the failure did not decide it
	read, err := r.svc.RecordingGet(r.as(agent), service.RecordingRef{ID: id})
	if err != nil || read.Proposals[0].State != "pending" {
		t.Errorf("after a failed decision: %+v %v", read, err)
	}
}

// agentCallers are the callers that are not a person, in every way a surface can name one; project is the one a
// project-limited key is limited to.
func (r *recEnv) agentCallers(project string) map[string]context.Context {
	return map[string]context.Context{
		"a project-limited key": service.WithCaller(context.Background(), service.Caller{
			Actor: "key:key_0000000002", Scopes: service.AllScopes, ProjectID: project,
		}),
		"a hosted chat session": service.WithCaller(context.Background(), service.Caller{
			Actor: "local:claude-code", Scopes: service.AllScopes, ChatSessionID: "cht_0000000001",
		}),
		"the local token": service.WithCaller(
			context.Background(),
			service.Caller{Actor: "local:cli", Scopes: service.AllScopes},
		),
		"an agent": r.as(agent),
		"an api key": service.WithCaller(
			context.Background(), service.Caller{Actor: "key:key_0000000001", Scopes: service.AllScopes}),
		"an mcp client": service.WithCaller(
			context.Background(),
			service.Caller{Actor: "local:mcp", Scopes: service.AllScopes},
		),
		"an operator": service.WithCaller(
			context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes, Operator: true}),
	}
}

// T-165: whatever an agent does with a recording, in whatever order, the helper never starts.
func TestSecurity_T165_NoSequenceOfAgentOperationsStartsTheHelper(t *testing.T) {
	r := newRecEnv(t)
	project, taken := r.placedTake(t)
	startsBefore := r.cap.starts.Load()

	for name, ctx := range r.agentCallers(project) {
		prepared, err := r.svc.RecordingPrepare(ctx, prepareReq(project))
		if err != nil {
			t.Fatalf("%s prepares: %v", name, err)
		}

		steps := []func() error{
			func() error {
				_, e := r.svc.RecordingMark(ctx, service.RecordingMarkRequest{ID: taken, Kind: "good", AtMS: 5})
				return e
			},
			func() error {
				_, e := r.svc.RecordingProposeCuts(
					ctx,
					service.RecordingProposeCutsRequest{ID: taken, Ranges: ranges(100, 500)},
				)
				return e
			},
			func() error { _, e := r.svc.RecordingGet(ctx, service.RecordingRef{ID: prepared.ID}); return e },
			func() error {
				_, e := r.svc.RecordingList(ctx, service.RecordingListRequest{ProjectID: project})
				return e
			},
			// the operations an agent may use on a plan: stop and pause find nothing running and start nothing
			func() error { _, e := r.svc.RecordingStop(ctx, service.RecordingRef{ID: prepared.ID}); return e },
			func() error { _, e := r.svc.RecordingPause(ctx, service.RecordingRef{ID: prepared.ID}); return e },
			// every way of starting or resuming is refused
			func() error {
				_, e := r.svc.RecordingStart(ctx, service.RecordingStartRequest{RecordingID: prepared.ID})
				return e
			},
			func() error { _, e := r.svc.RecordingResume(ctx, service.RecordingRef{ID: prepared.ID}); return e },
			func() error { _, e := r.svc.RecordingCancel(ctx, service.RecordingRef{ID: prepared.ID}); return e },
		}

		for i, step := range steps {
			_ = step()

			if n := r.cap.starts.Load(); n != startsBefore {
				t.Fatalf("%s, step %d: the helper was started (%d starts)", name, i, n)
			}
		}
	}
}

// T-165: the five paths that start or resume a capture all answer E_PRIVILEGED_SESSION to a caller that is not a
// person, with the project, the row and the disk left alone.
func TestSecurity_T165_EveryStartOrResumePathNeedsAPerson(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	plan, err := r.svc.RecordingPrepare(r.as(agent), prepareReq(project))
	if err != nil {
		t.Fatal(err)
	}

	paths := map[string]func(context.Context) error{
		"RecordingStart": func(c context.Context) error {
			_, e := r.svc.RecordingStart(c, service.RecordingStartRequest{
				ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"},
			})
			return e
		},
		"RecordingStart of a plan": func(c context.Context) error {
			_, e := r.svc.RecordingStart(c, service.RecordingStartRequest{RecordingID: plan.ID})
			return e
		},
		"RecordingResume": func(c context.Context) error {
			_, e := r.svc.RecordingResume(c, service.RecordingRef{ID: plan.ID})
			return e
		},
		"RecordingUploadBegin": func(c context.Context) error {
			_, e := r.svc.RecordingUploadBegin(c, service.RecordingUploadBeginRequest{ProjectID: project})
			return e
		},
		"RecordingUploadChunk": func(c context.Context) error {
			_, e := r.svc.RecordingUploadChunk(
				c,
				service.RecordingUploadChunkRequest{ID: plan.ID, Track: "screen", Body: strings.NewReader("x")},
			)
			return e
		},
		"RecordingUploadFinish": func(c context.Context) error {
			_, e := r.svc.RecordingUploadFinish(c, service.RecordingUploadFinishRequest{ID: plan.ID})
			return e
		},
	}

	rowsBefore, err := r.store.Recordings().ListByProject(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}

	for name, call := range paths {
		for who, ctx := range r.agentCallers(project) {
			if e := call(ctx); code(e) != "E_PRIVILEGED_SESSION" {
				t.Errorf("%s by %s: %v", name, who, e)
			}
		}
	}

	rowsAfter, err := r.store.Recordings().ListByProject(context.Background(), project)
	if err != nil || len(rowsAfter) != len(rowsBefore) || rowsAfter[0].Plan == "" {
		t.Errorf("a refused path changed the rows: %d -> %d, %v", len(rowsBefore), len(rowsAfter), err)
	}

	if r.cap.starts.Load() != 0 {
		t.Fatal("a refused path started the helper")
	}
}

// T-165: no rule can be written for a capture, and starting one never enters the approval queue.
func TestSecurity_T165_NoApprovalRuleCanBeWrittenForACaptureAndStartNeverQueues(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	for _, kind := range []string{"capture", "recording.start", "recording:start", "RecordingStart", "recording.resume"} {
		_, err := r.svc.ApprovalRuleSet(personCtx(), service.ApprovalRuleSetRequest{Kind: kind})
		if code(err) != "E_VALIDATION" || serviceError(t, err).Path != "kind" {
			t.Errorf("a rule for %q: %v", kind, err)
		}
	}

	rec := r.start(t, project)
	r.stopReady(t, rec.ID)

	list, err := r.svc.ApprovalList(personCtx(), service.ApprovalListRequest{State: "pending"})
	if err != nil || len(list.Requests) != 0 {
		t.Errorf("a person's start used the approval queue: %+v %v", list, err)
	}
}

// T-166: an agent never decides a proposal, on any caller, and a refused decision leaves it pending.
func TestSecurity_T166_AnAgentNeverDecidesAProposal(t *testing.T) {
	r := newRecEnv(t)
	project, id := r.placedTake(t)

	prop, err := r.svc.RecordingProposeCuts(
		r.as(agent),
		service.RecordingProposeCutsRequest{ID: id, Ranges: ranges(200, 400)},
	)
	if err != nil {
		t.Fatal(err)
	}

	rev := r.revision(t, project)

	for who, ctx := range r.agentCallers(project) {
		_, derr := r.svc.RecordingProposalDecide(ctx, service.RecordingProposalDecideRequest{
			ID: id, ProposalID: prop.Proposals[0].ID, Accept: []bool{true},
		})
		if code(derr) != "E_PRIVILEGED_SESSION" {
			t.Errorf("%s decides: %v", who, derr)
		}
	}

	read, err := r.svc.RecordingGet(r.as(agent), service.RecordingRef{ID: id})
	if err != nil || read.Proposals[0].State != "pending" || r.revision(t, project) != rev {
		t.Fatalf("after the refusals: %+v %v", read, err)
	}
}

// T-167: the service gives the shell the count of live helpers the host keeps, whatever the rows say.
func TestSecurity_T167_TheServiceReportsTheHostIndicatorNotTheRows(t *testing.T) {
	ind := &capturehost.Indicator{}
	e := newMediaEnv(t, mediaCfg{with: func(_ *env, o *service.Options) {
		o.Capture = service.NewLiveCapturer(&capturehost.Live{Indicator: ind})
	}})

	if a, ok := service.CaptureActivityOf(e.svc); !ok || a.Active {
		t.Fatalf("idle: %+v %v", a, ok)
	}

	ind.Enter()

	rows, err := e.store.Recordings().Open(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("the test has no recording row: %v %v", rows, err)
	}

	if a, ok := service.CaptureActivityOf(e.svc); !ok || !a.Active || a.Helpers != 1 || a.SinceMS == 0 {
		t.Errorf("a helper is alive and no row says so: %+v %v", a, ok)
	}

	ind.Leave()

	if a, _ := service.CaptureActivityOf(e.svc); a.Active {
		t.Errorf("the helper exited: %+v", a)
	}

	// a server with no capture says nothing is on
	none := newRecEnv(t)
	if a, ok := service.CaptureActivityOf(none.svc); !ok || a.Active {
		t.Errorf("a fake capturer: %+v %v", a, ok)
	}
}

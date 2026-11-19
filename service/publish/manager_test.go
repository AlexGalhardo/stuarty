package publish

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
)

var t0 = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)

type rig struct {
	t   *testing.T
	env *fakeEnv
	gw  *fakeGateway
	m   *Manager
}

func newRig(t *testing.T) *rig {
	t.Helper()

	env := newFakeEnv(t0)
	gw := newFakeGateway()
	m := NewManager(env, gw)
	env.m = m

	return &rig{t: t, env: env, gw: gw, m: m}
}

func (r *rig) create(targets ...TargetInput) *Publication {
	r.t.Helper()

	p, err := r.m.Create(context.Background(), CreateInput{
		ProjectID: "prj_1", Title: "My video", Text: "Watch it", Export: "job_exp", Targets: targets,
	})
	if err != nil {
		r.t.Fatal(err)
	}

	return p
}

func tin(at, zone string) TargetInput {
	return TargetInput{ConnectionID: "con_1", At: at, Zone: zone}
}

// approve does what ApprovalDecide does to the target: it runs the hook in a transaction.
func (r *rig) approve(p *Publication, targetID string) {
	r.t.Helper()

	cur, err := r.env.store.Get(context.Background(), p.ID)
	if err != nil {
		r.t.Fatal(err)
	}

	err = r.env.Write(context.Background(), func(ctx context.Context, tx Tx) error {
		return r.m.Approved(ctx, tx, targetID, cur.Target(targetID).ApprovalID)
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) target(p *Publication, id string) Target {
	r.t.Helper()

	cur, err := r.env.store.Get(context.Background(), p.ID)
	if err != nil {
		r.t.Fatal(err)
	}

	t := cur.Target(id)
	if t == nil {
		r.t.Fatalf("no target %s", id)
	}

	return *t
}

func wantInvalid(t *testing.T, err error, kind, path string) {
	t.Helper()

	var pe *Error
	if !errors.As(err, &pe) || pe.Kind != kind || (path != "" && !strings.HasPrefix(pe.Path, path)) {
		t.Fatalf("error = %v (%+v), want %s at %q", err, pe, kind, path)
	}
}

func TestPublicationCreate(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "America/Sao_Paulo"), TargetInput{ConnectionID: "con_2"})

	if len(p.Targets) != 2 || p.Title != "My video" || p.ProjectID != "prj_1" {
		t.Fatalf("publication = %+v", p)
	}

	first := p.Targets[0]
	if first.State != core.TargetDraft || first.Network != "fake" || first.ScheduledAt == nil ||
		!first.ScheduledAt.Equal(time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)) ||
		first.Zone != "America/Sao_Paulo" || first.LocalTime != "2026-11-21 09:00" {
		t.Errorf("first target = %+v", first)
	}

	if second := p.Targets[1]; second.State != core.TargetDraft || second.ScheduledAt != nil || second.Zone != "UTC" {
		t.Errorf("second target = %+v", second)
	}

	if len(r.env.events) != 1 || len(r.env.asks) != 0 {
		t.Errorf("events = %d, asks = %d", len(r.env.events), len(r.env.asks))
	}

	got, err := r.m.Get(context.Background(), p.ID)
	if err != nil || got.Targets[0].LocalTime != "2026-11-21 09:00" {
		t.Errorf("Get = %+v, %v", got, err)
	}

	if _, err := r.m.Get(context.Background(), "pub_nothing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get of nothing = %v", err)
	}
}

func TestPublicationCreateSaysWhatItDidToATime(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	p := r.create(tin("2027-03-14T02:30", "America/New_York"), tin("2026-11-01T01:30", "America/New_York"))

	if len(p.Notes) != 2 || !strings.Contains(p.Notes[0], "does not exist") ||
		!strings.Contains(p.Notes[1], "happens twice") {
		t.Fatalf("notes = %q", p.Notes)
	}

	if !p.Targets[0].ScheduledAt.Equal(time.Date(2027, 3, 14, 7, 30, 0, 0, time.UTC)) ||
		!p.Targets[1].ScheduledAt.Equal(time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)) {
		t.Errorf("instants = %v, %v", p.Targets[0].ScheduledAt, p.Targets[1].ScheduledAt)
	}
}

func TestPublicationCreateRefusals(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	late := 2000
	long := strings.Repeat("a", MaxTitleRunes+1)

	for _, tc := range []struct {
		name string
		in   CreateInput
		path string
		kind string
	}{
		{"no title", CreateInput{ProjectID: "p", Export: "e"}, "title", KindValidation},
		{"long title", CreateInput{ProjectID: "p", Title: long, Export: "e"}, "title", KindValidation},
		{"control in the title", CreateInput{ProjectID: "p", Title: "a\x07b", Export: "e"}, "title", KindValidation},
		{"newline in the title", CreateInput{ProjectID: "p", Title: "a\nb", Export: "e"}, "title", KindValidation},
		{"zero-width in the text", CreateInput{ProjectID: "p", Title: "a", Text: "a" + string(rune(0x200b)) + "b", Export: "e"}, "text", KindValidation},
		{"no export", CreateInput{ProjectID: "p", Title: "a"}, "export", KindValidation},
		{"no connection", CreateInput{ProjectID: "p", Title: "a", Export: "e", Targets: []TargetInput{{}}}, "targets[0].connection_id", KindValidation},
		{"bad zone", CreateInput{ProjectID: "p", Title: "a", Export: "e", Targets: []TargetInput{tin("2026-11-21T09:00", "Mars/Base")}}, "at", KindValidation},
		{"bad time", CreateInput{ProjectID: "p", Title: "a", Export: "e", Targets: []TargetInput{tin("tomorrow", "UTC")}}, "at", KindValidation},
		{"late window", CreateInput{ProjectID: "p", Title: "a", Export: "e", Targets: []TargetInput{{ConnectionID: "c", LateWindowMinutes: &late}}}, "late_window_minutes", KindValidation},
		{"too many tags", CreateInput{ProjectID: "p", Title: "a", Export: "e", Targets: []TargetInput{{ConnectionID: "c", Fields: Fields{Tags: make([]string, MaxTags+1)}}}}, "targets[0].fields.tags", KindValidation},
		{"too many targets", CreateInput{ProjectID: "p", Title: "a", Export: "e", Targets: make([]TargetInput, MaxTargets+1)}, "targets", KindValidation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t)
			_, err := r.m.Create(ctx, tc.in)
			wantInvalid(t, err, tc.kind, tc.path)

			if len(r.env.store.pubs) != 0 {
				t.Error("a refused create left a publication")
			}
		})
	}

	r := newRig(t)
	if _, err := r.m.Create(ctx, CreateInput{
		ProjectID: "p", Title: "a", Export: "e", Targets: []TargetInput{{ConnectionID: "con_missing"}},
	}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("a connection that does not exist = %v", err)
	}
}

func TestPublicationScheduleAsksAndWaits(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "America/Sao_Paulo"))
	id := p.Targets[0].ID

	got, err := r.m.Schedule(context.Background(), ScheduleInput{ID: p.ID, TargetID: id})
	if err != nil {
		t.Fatal(err)
	}

	tg := got.Targets[0]
	if tg.State != core.TargetWaitingApproval || tg.ApprovalID == "" || tg.Attempts != 0 {
		t.Fatalf("target = %+v", tg)
	}

	if len(r.env.asks) != 1 {
		t.Fatalf("asks = %d", len(r.env.asks))
	}

	a := r.env.asks[0]
	for _, want := range []string{
		`Publish "My video"`, "Fake network", `"My channel"`, "con_1", "visible to: private",
		"2026-11-21 09:00 America/Sao_Paulo", "2026-11-21T12:00:00Z", "video.mp4", "1234 bytes", `text: "Watch it"`,
	} {
		if !strings.Contains(a.Summary, want) {
			t.Errorf("the summary lacks %q: %s", want, a.Summary)
		}
	}

	if len(a.Summary) > 500 {
		t.Errorf("the summary is %d characters", len(a.Summary))
	}

	if a.ApprovalID != tg.ApprovalID || a.TargetID != id || a.PublicationID != p.ID || a.ProjectID != "prj_1" ||
		a.ConnectionID != "con_1" {
		t.Errorf("ask = %+v", a)
	}

	// The post as the person is shown it is kept, with its digest.
	cur := r.target(p, id)
	if len(cur.Snapshot) == 0 || len(cur.PayloadSHA256) != 64 {
		t.Errorf("snapshot = %s, digest = %q", cur.Snapshot, cur.PayloadSHA256)
	}
}

func TestPublicationApprovalMovesTheTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for _, tc := range []struct {
		name string
		at   string
		move time.Duration // how long after the creation the person approves
		want core.TargetState
	}{
		{"before its time", "2026-11-20T12:00", 0, core.TargetScheduled},
		{"at its time", "2026-11-20T12:00", 3 * time.Hour, core.TargetApproved},
		{"within the grace", "2026-11-20T12:00", 3*time.Hour + 30*time.Second, core.TargetApproved},
		{"after its window", "2026-11-20T12:00", 3*time.Hour + 2*time.Minute, core.TargetMissed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t)
			p := r.create(tin(tc.at, "UTC"))
			id := p.Targets[0].ID

			if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: id}); err != nil {
				t.Fatal(err)
			}

			r.env.set(t0.Add(tc.move))
			r.approve(p, id)

			if got := r.target(p, id).State; got != tc.want {
				t.Errorf("state = %s, want %s", got, tc.want)
			}

			if tc.want == core.TargetMissed && len(r.env.released) == 0 {
				t.Error("a missed target kept its reserved cost")
			}
		})
	}
}

func TestPublicationApprovalOfAReplacedRequestDoesNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "UTC"))
	id := p.Targets[0].ID

	if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: id}); err != nil {
		t.Fatal(err)
	}

	old := r.target(p, id).ApprovalID

	// The person moves it to another day: a new request replaces the old one.
	if _, err := r.m.Schedule(
		ctx,
		ScheduleInput{ID: p.ID, TargetID: id, At: "2026-11-22T09:00", Zone: "UTC"},
	); err != nil {
		t.Fatal(err)
	}

	cur := r.target(p, id)
	if cur.ApprovalID == old || cur.State != core.TargetWaitingApproval {
		t.Fatalf("target = %+v", cur)
	}

	if len(r.env.withdrawn) != 1 || r.env.withdrawn[0] != old {
		t.Errorf("withdrawn = %v, want the old request %s", r.env.withdrawn, old)
	}

	// The old request, approved late by some means, moves nothing.
	err := r.env.Write(ctx, func(ctx context.Context, tx Tx) error { return r.m.Approved(ctx, tx, id, old) })
	if err != nil || r.target(p, id).State != core.TargetWaitingApproval {
		t.Errorf("state after the old approval = %s, %v", r.target(p, id).State, err)
	}

	r.approve(p, id)

	if cur := r.target(p, id); cur.State != core.TargetScheduled || cur.ScheduledAt.Day() != 22 {
		t.Errorf("target = %+v", cur)
	}
}

func TestPublicationEditAfterApprovalAsksAgain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "UTC"), tin("2026-11-21T10:00", "UTC"))

	for _, tg := range p.Targets {
		if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: tg.ID}); err != nil {
			t.Fatal(err)
		}

		r.approve(p, tg.ID)
	}

	a, b := p.Targets[0].ID, p.Targets[1].ID
	if r.target(p, a).State != core.TargetScheduled || r.target(p, b).State != core.TargetScheduled {
		t.Fatal("both targets should be scheduled")
	}

	oldA := r.target(p, a).ApprovalID
	oldDigest := r.target(p, a).PayloadSHA256
	asks := len(r.env.asks)

	// A change of the text of one target returns that target only.
	title := "A different title"
	got, err := r.m.Update(ctx, UpdateInput{ID: p.ID, Targets: []TargetInput{{ID: a, Fields: Fields{Title: title}}}})
	if err != nil {
		t.Fatal(err)
	}

	ta, tb := got.Target(a), got.Target(b)
	if ta.State != core.TargetWaitingApproval || ta.ApprovalID == oldA || tb.State != core.TargetScheduled {
		t.Fatalf("a = %+v, b = %+v", ta, tb)
	}

	if len(r.env.asks) != asks+1 || !strings.Contains(r.env.asks[asks].Summary, title) {
		t.Errorf("the new request does not show the new title: %v", r.env.asks[asks:])
	}

	if r.target(p, a).PayloadSHA256 == oldDigest {
		t.Error("the snapshot was not taken again")
	}

	if !contains(r.env.withdrawn, oldA) || !contains(r.env.released, a) {
		t.Errorf("withdrawn = %v, released = %v", r.env.withdrawn, r.env.released)
	}

	// A change of the shared text returns every waiting target.
	text := "New caption"
	if got, err = r.m.Update(ctx, UpdateInput{ID: p.ID, Text: &text}); err != nil {
		t.Fatal(err)
	}

	if got.Target(a).State != core.TargetWaitingApproval || got.Target(b).State != core.TargetWaitingApproval {
		t.Errorf("states = %s, %s", got.Target(a).State, got.Target(b).State)
	}

	// An update that changes nothing asks nothing.
	r.approve(p, a)

	n := len(r.env.asks)
	if got, err = r.m.Update(
		ctx,
		UpdateInput{ID: p.ID, Text: &text, Targets: []TargetInput{{ID: a, Fields: Fields{Title: title}}}},
	); err != nil {
		t.Fatal(err)
	}

	if len(r.env.asks) != n || got.Target(a).State != core.TargetScheduled {
		t.Errorf("asks = %d (was %d), state = %s", len(r.env.asks), n, got.Target(a).State)
	}

	// The time is part of what was approved.
	if got, err = r.m.Update(
		ctx,
		UpdateInput{ID: p.ID, Targets: []TargetInput{{ID: a, At: "2026-11-23T09:00", Zone: "UTC"}}},
	); err != nil {
		t.Fatal(err)
	}

	if got.Target(a).State != core.TargetWaitingApproval || got.Target(a).ScheduledAt.Day() != 23 {
		t.Errorf("target = %+v", got.Target(a))
	}
}

func contains(list []string, s string) bool { return slices.Contains(list, s) }

func TestPublicationEditOfADraftAsksNothing(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "UTC"))
	title := "Edited"

	got, err := r.m.Update(context.Background(), UpdateInput{ID: p.ID, Title: &title, Targets: []TargetInput{
		{ID: p.Targets[0].ID, Fields: Fields{Tags: []string{"a", "b"}}},
		{ConnectionID: "con_3"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	if got.Title != title || len(got.Targets) != 2 || got.Targets[0].State != core.TargetDraft ||
		len(got.Targets[0].Fields.Tags) != 2 || got.Targets[1].ConnectionID != "con_3" || len(r.env.asks) != 0 {
		t.Errorf("publication = %+v, asks = %d", got, len(r.env.asks))
	}

	// A target is removed while it is a draft.
	got, err = r.m.Update(
		context.Background(),
		UpdateInput{ID: p.ID, Targets: []TargetInput{{ID: p.Targets[0].ID, Remove: true}}},
	)
	if err != nil || len(got.Targets) != 1 {
		t.Errorf("after the removal: %+v, %v", got, err)
	}
}

func TestPublicationEditRefusals(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "UTC"))
	id := p.Targets[0].ID

	// A target that is being sent, or went out, does not change.
	for _, st := range []core.TargetState{core.TargetPublishing, core.TargetPublished, core.TargetCancelled} {
		cur, _ := r.env.store.Get(ctx, p.ID)
		cur.Target(id).State = st
		_ = r.env.store.Save(ctx, &cur)

		_, err := r.m.Update(ctx, UpdateInput{ID: p.ID, Targets: []TargetInput{{ID: id, Fields: Fields{Title: "x"}}}})
		wantInvalid(t, err, KindInvalidState, "")

		if st == core.TargetPublishing {
			title := "shared"
			_, err = r.m.Update(ctx, UpdateInput{ID: p.ID, Title: &title})
			wantInvalid(t, err, KindInvalidState, "")
		}
	}

	empty := ""
	if _, err := r.m.Update(ctx, UpdateInput{ID: p.ID, Export: &empty}); err == nil {
		t.Error("a publication without its video was accepted")
	}

	if _, err := r.m.Update(ctx, UpdateInput{ID: "pub_nothing"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update of nothing = %v", err)
	}

	if _, err := r.m.Update(ctx, UpdateInput{ID: p.ID, Targets: []TargetInput{{ID: "tgt_nothing"}}}); err == nil {
		t.Error("a target that is not of the publication was accepted")
	}
}

func TestPublicationScheduleRefusals(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	p := r.create(tin("2026-11-19T09:00", "UTC"), TargetInput{ConnectionID: "con_1"})

	// The time is already past.
	_, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID})
	wantInvalid(t, err, KindValidation, "at")

	// No time at all.
	_, err = r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[1].ID})
	wantInvalid(t, err, KindValidation, "at")

	// Without a target, the targets that have a time are scheduled; none here is in the future.
	_, err = r.m.Schedule(ctx, ScheduleInput{ID: p.ID})
	wantInvalid(t, err, KindValidation, "at")

	// A refusal changes nothing.
	if st := r.target(p, p.Targets[0].ID).State; st != core.TargetDraft || len(r.env.asks) != 0 {
		t.Errorf("state = %s, asks = %d", st, len(r.env.asks))
	}

	// The issues of the network come back whole.
	r.gw.issues = []core.Issue{{Path: "title", Code: "too_long", Message: "the title is too long"}}

	_, err = r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[1].ID, At: "2026-11-21T09:00", Zone: "UTC"})

	var pe *Error
	if !errors.As(err, &pe) || len(pe.Issues) != 1 || pe.Path != "title" {
		t.Errorf("error = %v", err)
	}

	if len(r.env.asks) != 0 {
		t.Error("a post with issues was asked about")
	}
}

func TestPublicationScheduleWithoutATargetTakesTheDrafts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "UTC"), tin("2026-11-22T09:00", "UTC"), TargetInput{ConnectionID: "con_1"})

	got, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID})
	if err != nil {
		t.Fatal(err)
	}

	if got.Targets[0].State != core.TargetWaitingApproval || got.Targets[1].State != core.TargetWaitingApproval ||
		got.Targets[2].State != core.TargetDraft || len(r.env.asks) != 2 {
		t.Errorf(
			"states = %s, %s, %s; asks = %d",
			got.Targets[0].State,
			got.Targets[1].State,
			got.Targets[2].State,
			len(r.env.asks),
		)
	}
}

func TestPublicationDeclinedBackToDraft(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "UTC"))
	id := p.Targets[0].ID

	if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: id}); err != nil {
		t.Fatal(err)
	}

	ap := r.target(p, id).ApprovalID

	err := r.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		return r.m.Declined(ctx, tx, id, ap, CodeExpired, "no one answered")
	})
	if err != nil {
		t.Fatal(err)
	}

	cur := r.target(p, id)
	if cur.State != core.TargetDraft || cur.Error == nil || cur.Error.Code != CodeExpired || cur.ApprovalID != "" ||
		cur.Error.Hint == "" {
		t.Errorf("target = %+v, error = %+v", cur, cur.Error)
	}

	// It can be asked again, and the error goes.
	if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: id}); err != nil {
		t.Fatal(err)
	}

	if cur := r.target(p, id); cur.State != core.TargetWaitingApproval || cur.Error != nil {
		t.Errorf("target = %+v", cur)
	}
}

func TestPublicationSendNowAndRetry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "UTC"))
	id := p.Targets[0].ID

	got, err := r.m.SendNow(ctx, TargetRef{ID: p.ID, TargetID: id})
	if err != nil {
		t.Fatal(err)
	}

	tg := got.Targets[0]
	if tg.State != core.TargetWaitingApproval || !tg.SendNow || !tg.ScheduledAt.Equal(t0) {
		t.Fatalf("target = %+v", tg)
	}

	// The person answers two hours later: a send-now request is still good, a scheduled one would have been missed.
	r.env.set(t0.Add(2 * time.Hour))
	r.approve(p, id)

	if st := r.target(p, id).State; st != core.TargetApproved {
		t.Errorf("state = %s, want approved", st)
	}

	// Retry is for a failed target only.
	_, err = r.m.Retry(ctx, TargetRef{ID: p.ID, TargetID: id})
	wantInvalid(t, err, KindInvalidState, "")

	cur, _ := r.env.store.Get(ctx, p.ID)
	cur.Target(id).State = core.TargetFailed
	cur.Target(id).Error = &TargetError{Code: "quota", Message: "x"}
	_ = r.env.store.Save(ctx, &cur)

	got, err = r.m.Retry(ctx, TargetRef{ID: p.ID, TargetID: id})
	if err != nil || got.Targets[0].State != core.TargetWaitingApproval || got.Targets[0].Error != nil {
		t.Errorf("after the retry: %+v, %v", got.Targets[0], err)
	}

	// Giving it a time clears send_now.
	got, err = r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: id, At: "2026-11-25T09:00", Zone: "UTC"})
	if err != nil || got.Targets[0].SendNow {
		t.Errorf("after the schedule: %+v, %v", got.Targets[0], err)
	}

	if _, err := r.m.SendNow(ctx, TargetRef{ID: p.ID}); err == nil {
		t.Error("send-now without a target was accepted")
	}
}

func TestPublicationCancel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "UTC"), tin("2026-11-22T09:00", "UTC"), tin("2026-11-23T09:00", "UTC"))

	for _, tg := range p.Targets[:2] {
		if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: tg.ID}); err != nil {
			t.Fatal(err)
		}
	}

	first := r.target(p, p.Targets[0].ID).ApprovalID

	got, err := r.m.Cancel(ctx, TargetRef{ID: p.ID, TargetID: p.Targets[0].ID})
	if err != nil || got.Targets[0].State != core.TargetCancelled {
		t.Fatalf("cancel one: %+v, %v", got.Targets[0], err)
	}

	if !contains(r.env.withdrawn, first) || !contains(r.env.released, p.Targets[0].ID) {
		t.Errorf("withdrawn = %v, released = %v", r.env.withdrawn, r.env.released)
	}

	// Every one that has not gone out.
	got, err = r.m.Cancel(ctx, TargetRef{ID: p.ID})
	if err != nil {
		t.Fatal(err)
	}

	for _, tg := range got.Targets {
		if tg.State != core.TargetCancelled {
			t.Errorf("target %s is %s", tg.ID, tg.State)
		}
	}

	// A cancelled target is not scheduled again.
	_, err = r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID})
	wantInvalid(t, err, KindInvalidState, "")

	// One that is being sent, or went out, is not cancelled.
	cur, _ := r.env.store.Get(ctx, p.ID)
	cur.Target(p.Targets[2].ID).State = core.TargetPublishing
	_ = r.env.store.Save(ctx, &cur)

	_, err = r.m.Cancel(ctx, TargetRef{ID: p.ID, TargetID: p.Targets[2].ID})
	wantInvalid(t, err, KindInvalidState, "")

	cur.Target(p.Targets[2].ID).State = core.TargetPublished
	_ = r.env.store.Save(ctx, &cur)

	_, err = r.m.Cancel(ctx, TargetRef{ID: p.ID, TargetID: p.Targets[2].ID})
	wantInvalid(t, err, KindInvalidState, "")

	if _, err := r.m.Cancel(ctx, TargetRef{ID: p.ID, TargetID: "tgt_nothing"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cancel of nothing = %v", err)
	}
}

func TestPublicationList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	a := r.create(tin("2026-11-21T09:00", "UTC"))
	b := r.create(tin("2026-12-05T09:00", "UTC"))

	if _, err := r.m.Schedule(ctx, ScheduleInput{ID: a.ID, TargetID: a.Targets[0].ID}); err != nil {
		t.Fatal(err)
	}

	list, total, err := r.m.List(ctx, Filter{ProjectID: "prj_1"})
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("all: %d, %v", total, err)
	}

	from, to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	list, _, err = r.m.List(ctx, Filter{From: &from, To: &to})
	if err != nil || len(list) != 1 || list[0].ID != a.ID {
		t.Errorf("november: %v, %v", list, err)
	}

	list, _, _ = r.m.List(ctx, Filter{State: core.TargetWaitingApproval})
	if len(list) != 1 || list[0].ID != a.ID {
		t.Errorf("waiting: %v", list)
	}

	list, _, _ = r.m.List(ctx, Filter{State: core.TargetDraft})
	if len(list) != 1 || list[0].ID != b.ID {
		t.Errorf("draft: %v", list)
	}

	list, _, _ = r.m.List(ctx, Filter{ProjectID: "prj_other"})
	if len(list) != 0 {
		t.Errorf("other project: %v", list)
	}

	if _, _, err := r.m.List(ctx, Filter{State: "bogus"}); err == nil {
		t.Error("a state that is not one was accepted")
	}
}

func TestApprovalByARuleSchedulesAtOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	r.env.auto = true

	p := r.create(tin("2026-11-21T09:00", "UTC"))

	got, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID})
	if err != nil {
		t.Fatal(err)
	}

	if got.Targets[0].State != core.TargetScheduled {
		t.Errorf("state = %s, want scheduled", got.Targets[0].State)
	}
}

func TestApprovalRefusedLeavesTheTargetAsItWas(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	p := r.create(tin("2026-11-21T09:00", "UTC"))
	r.env.askErr = errors.New("over the ceiling")

	if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID}); err == nil {
		t.Fatal("the schedule was accepted")
	}

	if cur := r.target(
		p,
		p.Targets[0].ID,
	); cur.State != core.TargetDraft || cur.ApprovalID != "" ||
		cur.Snapshot != nil {
		t.Errorf("target = %+v", cur)
	}
}

func TestApprovalSummaryIsCleaned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)

	p, err := r.m.Create(ctx, CreateInput{
		ProjectID: "prj_1", Title: strings.Repeat("T", 150), Text: "line one\nline two " + strings.Repeat("x", 300),
		Export: "e", Targets: []TargetInput{tin("2026-11-21T09:00", "UTC")},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID}); err != nil {
		t.Fatal(err)
	}

	s := r.env.asks[0].Summary
	if len(s) > 500 || strings.ContainsAny(s, "\n\r\t") {
		t.Errorf("summary (%d): %q", len(s), s)
	}
}

func TestCostOfATargetIsAskedAndReleased(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := newRig(t)
	r.gw.cost = 250000

	p := r.create(tin("2026-11-21T09:00", "UTC"))
	id := p.Targets[0].ID

	if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: id}); err != nil {
		t.Fatal(err)
	}

	if a := r.env.asks[0]; a.CostMicros != 250000 || a.Currency != "USD" {
		t.Errorf("ask = %+v", a)
	}

	// Every request starts by giving back what the former one reserved.
	r.approve(p, id)

	released := len(r.env.released)

	if _, err := r.m.Update(ctx, UpdateInput{ID: p.ID, Title: new("Another")}); err != nil {
		t.Fatal(err)
	}

	if len(r.env.released) <= released || !contains(r.env.released, id) {
		t.Errorf("released = %v", r.env.released)
	}

	// Cancelling gives it back too.
	released = len(r.env.released)
	if _, err := r.m.Cancel(ctx, TargetRef{ID: p.ID, TargetID: id}); err != nil || len(r.env.released) <= released {
		t.Errorf("released = %v, %v", r.env.released, err)
	}
}

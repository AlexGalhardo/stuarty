package publish

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
)

//go:fix inline

func TestCalendarDayNoteAndThemeAreKeptClearedAndPerOwner(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	ctx := context.Background()

	d, err := r.m.DaySet(
		ctx,
		DayInput{Date: "2026-12-25", Zone: "America/Sao_Paulo", Note: new("long video day"), Theme: new("amber")},
	)
	if err != nil || d.Note != "long video day" || d.Theme != "amber" || d.Zone != "America/Sao_Paulo" {
		t.Fatalf("set: %+v, %v", d, err)
	}

	// A member left out stays: the theme alone does not wipe the note.
	d, err = r.m.DaySet(ctx, DayInput{Date: "2026-12-25", Theme: new("teal")})
	if err != nil || d.Note != "long video day" || d.Theme != "teal" || d.Zone != "America/Sao_Paulo" {
		t.Fatalf("theme only: %+v, %v", d, err)
	}

	// Another user of a network workspace has a day of their own.
	r.env.as(Actor{ID: "usr:b", OwnerUserID: "usr_b"})

	if got, _, _ := r.env.store.Day(ctx, "usr_b", "2026-12-25"); got.Note != "" {
		t.Errorf("a day of another owner: %+v", got)
	}

	if _, err := r.m.DaySet(ctx, DayInput{Date: "2026-12-25", Note: new("mine")}); err != nil {
		t.Fatal(err)
	}

	mine, _, _ := r.env.store.Day(ctx, "usr_b", "2026-12-25")
	first, _, _ := r.env.store.Day(ctx, "", "2026-12-25")

	if mine.Note != "mine" || first.Note != "long video day" {
		t.Errorf("owners mixed: %+v and %+v", mine, first)
	}

	// Clearing both leaves no row.
	r.env.as(Actor{})

	d, err = r.m.DaySet(ctx, DayInput{Date: "2026-12-25", Note: new(""), Theme: new("")})
	if err != nil || !d.Empty() {
		t.Fatalf("clear: %+v, %v", d, err)
	}

	if _, ok, _ := r.env.store.Day(ctx, "", "2026-12-25"); ok {
		t.Error("an empty day kept its row")
	}

	for name, in := range map[string]DayInput{
		"date":  {Date: "25/12/2026"},
		"theme": {Date: "2026-12-25", Theme: new("#fff")},
		"note":  {Date: "2026-12-25", Note: new(strings.Repeat("a", core.MaxNoteRunes+1))},
		"zone":  {Date: "2026-12-25", Zone: "Local"},
	} {
		if _, err := r.m.DaySet(ctx, in); err == nil {
			t.Errorf("%s was accepted", name)
		} else {
			wantInvalid(t, err, KindValidation, name)
		}
	}
}

func TestCalendarQuietDayIsAPersonsAndKeepsTheNote(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	ctx := context.Background()

	if _, err := r.m.DaySet(ctx, DayInput{Date: "2026-12-25", Note: new("holiday")}); err != nil {
		t.Fatal(err)
	}

	for _, who := range []Actor{{ID: "local:token"}, {ID: "key:agent", OwnerUserID: "usr_a"}} {
		r.env.as(who)

		_, err := r.m.QuietSet(ctx, "2026-12-25", "UTC", true)
		wantInvalid(t, err, KindPersonOnly, "")

		var pe *Error

		if errors.As(err, &pe) && pe.Hint == "" {
			t.Error("the refusal has no hint")
		}
	}

	if d, _, _ := r.env.store.Day(ctx, "", "2026-12-25"); d.Quiet {
		t.Fatal("a caller that is not a person marked a quiet day")
	}

	r.env.as(Actor{ID: "user:ui", Person: true})

	d, err := r.m.QuietSet(ctx, "2026-12-25", "UTC", true)
	if err != nil || !d.Quiet || d.Note != "holiday" {
		t.Fatalf("a person marks it: %+v, %v", d, err)
	}

	// An agent cannot remove it either.
	r.env.as(Actor{ID: "key:agent"})

	if _, err := r.m.QuietSet(ctx, "2026-12-25", "UTC", false); err == nil {
		t.Fatal("an agent removed a quiet day")
	}

	if d, _, _ := r.env.store.Day(ctx, "", "2026-12-25"); !d.Quiet {
		t.Fatal("the mark is gone")
	}

	r.env.as(Actor{ID: "user:ui", Person: true})

	d, err = r.m.QuietSet(ctx, "2026-12-25", "", false)
	if err != nil || d.Quiet || d.Note != "holiday" {
		t.Fatalf("a person clears it: %+v, %v", d, err)
	}

	// With nothing but the mark, clearing the mark deletes the row.
	if _, err := r.m.QuietSet(ctx, "2026-12-26", "", true); err != nil {
		t.Fatal(err)
	}

	if _, err := r.m.QuietSet(ctx, "2026-12-26", "", false); err != nil {
		t.Fatal(err)
	}

	if _, ok, _ := r.env.store.Day(ctx, "", "2026-12-26"); ok {
		t.Error("a day with nothing kept its row")
	}
}

func TestCalendarSlotsAreSavedChangedRemovedAndBounded(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	ctx := context.Background()

	s, err := r.m.SlotSave(
		ctx,
		SlotInput{Network: "instagram", Weekday: "tue", Time: "18:00", Zone: "America/Sao_Paulo"},
	)
	if err != nil || !strings.HasPrefix(s.ID, "slt_") {
		t.Fatalf("create: %+v, %v", s, err)
	}

	s2, err := r.m.SlotSave(ctx, SlotInput{ID: s.ID, Network: "instagram", Weekday: "wed", Time: "09:30", Zone: "UTC"})
	if err != nil || s2.ID != s.ID || s2.Weekday != "wed" {
		t.Fatalf("change: %+v, %v", s2, err)
	}

	if all, _ := r.env.store.Slots(ctx, ""); len(all) != 1 || all[0].Time != "09:30" {
		t.Fatalf("slots %+v", all)
	}

	// A slot of someone else is not found, not changed and not removed.
	r.env.as(Actor{ID: "usr:b", OwnerUserID: "usr_b"})

	if _, err := r.m.SlotSave(
		ctx,
		SlotInput{ID: s.ID, Network: "x", Weekday: "mon", Time: "10:00"},
	); !errors.Is(
		err,
		ErrNotFound,
	) {
		t.Errorf("another owner changed a slot: %v", err)
	}

	if err := r.m.SlotDelete(ctx, s.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another owner removed a slot: %v", err)
	}

	r.env.as(Actor{})

	if _, err := r.m.SlotSave(ctx, SlotInput{Network: "x", Weekday: "funday", Time: "10:00"}); err == nil {
		t.Error("a bad weekday was accepted")
	} else {
		wantInvalid(t, err, KindValidation, "")
	}

	if err := r.m.SlotDelete(ctx, s.ID); err != nil {
		t.Fatal(err)
	}

	if err := r.m.SlotDelete(ctx, s.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed twice: %v", err)
	}

	for i := range core.MaxSlots {
		if _, err := r.m.SlotSave(
			ctx,
			SlotInput{Network: "x", Weekday: "mon", Time: fmt.Sprintf("%02d:00", i%24)},
		); err != nil {
			t.Fatalf("slot %d: %v", i, err)
		}
	}

	_, err = r.m.SlotSave(ctx, SlotInput{Network: "x", Weekday: "mon", Time: "10:00"})
	wantInvalid(t, err, KindValidation, "")
}

func TestCalendarPlanHasDaysSlotsFilledByATargetAndOnlyTheAskedPublications(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	ctx := context.Background()

	// "fake" is the network of the fake gateway; Tuesday 2026-12-01 18:00 in Lisbon is 18:00 UTC.
	slot, err := r.m.SlotSave(ctx, SlotInput{Network: "fake", Weekday: "tue", Time: "18:00", Zone: "Europe/Lisbon"})
	if err != nil {
		t.Fatal(err)
	}

	p := r.create(tin("2026-12-01T18:15", "Europe/Lisbon"))

	if _, err := r.m.DaySet(ctx, DayInput{Date: "2026-12-01", Note: new("series"), Theme: new("blue")}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.m.DaySet(ctx, DayInput{Date: "2026-12-20", Note: new("outside")}); err != nil {
		t.Fatal(err)
	}

	from, to := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC)

	plan, err := r.m.Plan(ctx, PlanInput{From: from, To: to, Zone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Days) != 1 || plan.Days[0].Date != "2026-12-01" || plan.Days[0].Note != "series" {
		t.Errorf("days %+v", plan.Days)
	}

	if len(plan.Rules) != 1 || plan.Rules[0].ID != slot.ID {
		t.Errorf("rules %+v", plan.Rules)
	}

	if len(plan.Slots) != 1 || plan.Slots[0].Date != "2026-12-01" || plan.Slots[0].Filled == nil ||
		plan.Slots[0].Filled.PublicationID != p.ID || plan.Slots[0].Filled.TargetID != p.Targets[0].ID {
		t.Fatalf("slots %+v", plan.Slots)
	}

	if len(plan.Publications) != 0 {
		t.Errorf("the publications were not asked for: %d", len(plan.Publications))
	}

	plan, err = r.m.Plan(ctx, PlanInput{From: from, To: to, Zone: "UTC", Publications: true})
	if err != nil || len(plan.Publications) != 1 || plan.Total != 1 {
		t.Fatalf("with publications: %+v, %v", plan, err)
	}

	// A cancelled target does not fill the slot.
	if _, err := r.m.Cancel(ctx, TargetRef{ID: p.ID, TargetID: p.Targets[0].ID}); err != nil {
		t.Fatal(err)
	}

	plan, _ = r.m.Plan(ctx, PlanInput{From: from, To: to, Zone: "UTC"})
	if plan.Slots[0].Filled != nil {
		t.Errorf("a cancelled target fills a slot: %+v", plan.Slots[0])
	}

	if _, err := r.m.Plan(ctx, PlanInput{From: to, To: from}); err == nil {
		t.Error("a range that ends before it starts")
	}

	if _, err := r.m.Plan(ctx, PlanInput{From: from, To: to, Zone: "Mars/Olympus"}); err == nil {
		t.Error("an unknown zone")
	}
}

// The day of the view is a local date: the plan of the 25th in Sao Paulo holds the day row and the slot of that date.
func TestCalendarPlanOfOneLocalDay(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	ctx := context.Background()

	if _, err := r.m.SlotSave(
		ctx,
		SlotInput{Network: "fake", Weekday: "fri", Time: "22:00", Zone: "America/Sao_Paulo"},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := r.m.DaySet(
		ctx,
		DayInput{Date: "2026-12-25", Zone: "America/Sao_Paulo", Theme: new("red")},
	); err != nil {
		t.Fatal(err)
	}

	from, _ := core.ParseBound("2026-12-25", "America/Sao_Paulo")
	to, _ := core.ParseBound("2026-12-26", "America/Sao_Paulo")

	plan, err := r.m.Plan(ctx, PlanInput{From: from, To: to, Zone: "America/Sao_Paulo"})
	if err != nil || len(plan.Days) != 1 || len(plan.Slots) != 1 || plan.Slots[0].At.Hour() != 1 {
		t.Fatalf("%+v, %v", plan, err)
	}
}

func TestCalendarDuplicateMakesANewDraftAndLeavesTheOriginal(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	ctx := context.Background()
	r.env.auto = false

	p := r.create(
		tin("2026-11-25T09:00", "America/Sao_Paulo"),
		TargetInput{ConnectionID: "con_2", Fields: Fields{Title: "Other", Tags: []string{"a"}}},
	)

	for _, tg := range p.Targets {
		if _, err := r.m.Schedule(
			ctx,
			ScheduleInput{ID: p.ID, TargetID: tg.ID, At: "2026-11-25T09:00", Zone: "America/Sao_Paulo"},
		); err != nil {
			t.Fatal(err)
		}
	}

	r.approve(p, p.Targets[0].ID)

	before, _ := r.env.store.Get(ctx, p.ID)
	asks := len(r.env.asks)

	r.env.as(Actor{ID: "key:agent", OwnerUserID: "usr_a"})

	cp, err := r.m.Duplicate(ctx, DuplicateInput{ID: p.ID, At: "2026-12-02T18:00", Zone: "America/Sao_Paulo"})
	if err != nil {
		t.Fatal(err)
	}

	if cp.ID == p.ID || cp.Title != p.Title || cp.Text != p.Text || cp.Export != p.Export ||
		cp.ProjectID != p.ProjectID ||
		len(cp.Targets) != 2 ||
		cp.OwnerUserID != "usr_a" ||
		cp.CreatedBy != "key:agent" {
		t.Fatalf("copy %+v", cp)
	}

	for i, tg := range cp.Targets {
		if tg.ID == p.Targets[i].ID || tg.State != core.TargetDraft || tg.ApprovalID != "" || tg.Attempts != 0 ||
			tg.ScheduledAt == nil || tg.ScheduledAt.Format(time.RFC3339) != "2026-12-02T21:00:00Z" ||
			tg.Zone != "America/Sao_Paulo" || tg.ConnectionID != p.Targets[i].ConnectionID || tg.Network != p.Targets[i].Network {
			t.Errorf("target %d = %+v", i, tg)
		}

		if tg.SendNow || len(tg.Snapshot) != 0 || tg.PayloadSHA256 != "" || tg.RemoteID != "" || tg.Error != nil {
			t.Errorf("target %d carries the old post: %+v", i, tg)
		}
	}

	if cp.Targets[1].Fields.Title != "Other" || len(cp.Targets[1].Fields.Tags) != 1 {
		t.Errorf("fields %+v", cp.Targets[1].Fields)
	}

	// No request was made, and the original is exactly as it was.
	if len(r.env.asks) != asks {
		t.Errorf("a copy asked for approval: %d asks", len(r.env.asks)-asks)
	}

	after, _ := r.env.store.Get(ctx, p.ID)
	if fmt.Sprintf("%+v", before) != fmt.Sprintf("%+v", after) {
		t.Errorf("the original changed:\n%+v\n%+v", before, after)
	}

	// One target only.
	one, err := r.m.Duplicate(ctx, DuplicateInput{ID: p.ID, TargetID: p.Targets[1].ID, At: "2026-12-03T10:00"})
	if err != nil || len(one.Targets) != 1 || one.Targets[0].ConnectionID != "con_2" ||
		one.Targets[0].Zone != "America/Sao_Paulo" {
		t.Fatalf("one target: %+v, %v", one, err)
	}

	if _, err := r.m.Duplicate(ctx, DuplicateInput{ID: p.ID}); err == nil {
		t.Error("a copy with no time")
	}

	if _, err := r.m.Duplicate(
		ctx,
		DuplicateInput{ID: p.ID, TargetID: "tgt_nope", At: "2026-12-03T10:00"},
	); !errors.Is(
		err,
		ErrNotFound,
	) {
		t.Errorf("an unknown target: %v", err)
	}

	if _, err := r.m.Duplicate(
		ctx,
		DuplicateInput{ID: "pub_nope", At: "2026-12-03T10:00"},
	); !errors.Is(
		err,
		ErrNotFound,
	) {
		t.Errorf("an unknown publication: %v", err)
	}
}

func TestCalendarExportHoldsTheScheduledTargetsAndNoId(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	ctx := context.Background()

	p := r.create(
		tin("2026-12-01T09:00", "UTC"),
		TargetInput{ConnectionID: "con_secret_2", At: "2026-12-02T09:00", Zone: "UTC"},
		TargetInput{ConnectionID: "con_3"},
	)

	if _, err := r.m.Cancel(ctx, TargetRef{ID: p.ID, TargetID: p.Targets[1].ID}); err != nil {
		t.Fatal(err)
	}

	from, to := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 8, 0, 0, 0, 0, time.UTC)

	body, err := r.m.Export(ctx, from, to, Filter{})
	if err != nil {
		t.Fatal(err)
	}

	if n := strings.Count(body, "BEGIN:VEVENT"); n != 1 {
		t.Fatalf("%d events, want the one scheduled and not cancelled:\n%s", n, body)
	}

	for _, id := range []string{p.ID, p.Targets[0].ID, "con_", "apr_", "job_exp"} {
		if strings.Contains(body, id) {
			t.Errorf("the file holds %q:\n%s", id, body)
		}
	}

	if !strings.Contains(body, "DTSTART:20261201T090000Z") || !strings.Contains(body, "SUMMARY:fake: My video") {
		t.Errorf("the event:\n%s", body)
	}

	if _, err := r.m.Export(ctx, to, from, Filter{}); err == nil {
		t.Error("a range that ends before it starts")
	}
}

// quietRig is a rig with 2026-12-25 marked as quiet by a person; the actors are the ones that may call.
type quietRig struct {
	*rig

	person, agent Actor
}

func newQuietRig(t *testing.T) *quietRig {
	t.Helper()

	q := &quietRig{rig: newRig(t), person: Actor{ID: "user:ui", Person: true}, agent: Actor{ID: "key:agent"}}

	q.env.as(q.person)

	if _, err := q.m.QuietSet(context.Background(), "2026-12-25", "America/Sao_Paulo", true); err != nil {
		t.Fatal(err)
	}

	return q
}

// wantQuiet checks the three answers of one path: refused for an agent, whatever it passes; asked to confirm for a
// person; and done for a person who confirmed. try(confirm) runs the path as the current actor.
func (q *quietRig) wantQuiet(name string, p *Publication, try func(confirm bool) error) {
	q.t.Helper()

	asks := len(q.env.asks)
	before, _ := q.env.store.Get(context.Background(), p.ID)

	q.env.as(q.agent)

	for _, confirm := range []bool{false, true} {
		err := try(confirm)
		wantInvalid(q.t, err, KindPersonOnly, "at")

		var pe *Error

		if errors.As(err, &pe) && (pe.Hint == "" || !strings.Contains(pe.Message, "2026-12-")) {
			q.t.Errorf("%s: the refusal does not say which day or what to do: %+v", name, pe)
		}
	}

	q.env.as(q.person)

	wantInvalid(q.t, try(false), KindInvalidState, "confirm_quiet_day")

	if len(q.env.asks) != asks {
		q.t.Fatalf("%s: a refused call asked for approval", name)
	}

	cur, _ := q.env.store.Get(context.Background(), p.ID)
	for i := range cur.Targets {
		tg := &cur.Targets[i]
		if tg.State != before.Targets[i].State || tg.ApprovalID != before.Targets[i].ApprovalID ||
			!ptrEqual(tg.ScheduledAt, before.Targets[i].ScheduledAt) {
			q.t.Fatalf("%s: a refused call changed the target: %+v", name, *tg)
		}
	}

	if err := try(true); err != nil {
		q.t.Fatalf("%s: the confirmed call: %v", name, err)
	}

	if len(q.env.asks) != asks+1 {
		q.t.Errorf("%s: the confirmed call made %d requests", name, len(q.env.asks)-asks)
	}
}

func TestCalendarQuietDayRefusesEveryPathThatAsksForApproval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 23:30 on the 25th in Sao Paulo is 02:30 on the 26th in UTC: the day of the post is the 25th.
	const at = "2026-12-25T23:30"

	t.Run("schedule", func(t *testing.T) {
		t.Parallel()

		q := newQuietRig(t)
		p := q.create(tin(at, "America/Sao_Paulo"))

		q.wantQuiet("schedule", p, func(confirm bool) error {
			_, err := q.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID, ConfirmQuietDay: confirm})

			return err
		})
	})

	t.Run("schedule with a new time", func(t *testing.T) {
		t.Parallel()

		q := newQuietRig(t)
		p := q.create(tin("2026-12-30T09:00", "America/Sao_Paulo"))

		q.wantQuiet("schedule at", p, func(confirm bool) error {
			_, err := q.m.Schedule(ctx, ScheduleInput{
				ID: p.ID, TargetID: p.Targets[0].ID, At: at, Zone: "America/Sao_Paulo", ConfirmQuietDay: confirm,
			})

			return err
		})
	})

	t.Run("send now on the quiet day", func(t *testing.T) {
		t.Parallel()

		q := newQuietRig(t)
		q.env.set(time.Date(2026, 12, 25, 15, 0, 0, 0, time.UTC))

		p := q.create(TargetInput{ConnectionID: "con_1", Zone: "America/Sao_Paulo"})

		q.wantQuiet("send now", p, func(confirm bool) error {
			_, err := q.m.SendNow(ctx, TargetRef{ID: p.ID, TargetID: p.Targets[0].ID, ConfirmQuietDay: confirm})

			return err
		})
	})

	t.Run("retry", func(t *testing.T) {
		t.Parallel()

		q := newQuietRig(t)
		q.env.set(time.Date(2026, 12, 25, 15, 0, 0, 0, time.UTC))

		p := q.create(TargetInput{ConnectionID: "con_1", Zone: "America/Sao_Paulo"})
		failTarget(t, q.rig, p)

		q.wantQuiet("retry", p, func(confirm bool) error {
			_, err := q.m.Retry(ctx, TargetRef{ID: p.ID, TargetID: p.Targets[0].ID, ConfirmQuietDay: confirm})

			return err
		})
	})

	t.Run("an edit that asks again", func(t *testing.T) {
		t.Parallel()

		q := newQuietRig(t)

		// The target waits on a day that was not quiet; the day is then marked, and the edit asks again.
		q.env.as(q.person)

		p := q.create(tin("2026-12-30T09:00", "America/Sao_Paulo"))
		if _, err := q.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID}); err != nil {
			t.Fatal(err)
		}

		if _, err := q.m.QuietSet(ctx, "2026-12-30", "America/Sao_Paulo", true); err != nil {
			t.Fatal(err)
		}

		q.wantQuiet("update", p, func(confirm bool) error {
			_, err := q.m.Update(
				ctx,
				UpdateInput{ID: p.ID, Title: new(fmt.Sprintf("new title %v", confirm)), ConfirmQuietDay: confirm},
			)

			return err
		})
	})
}

// Marking a day is not cancelling: what a person approved before stays approved, and a day that is not quiet is
// not affected by the mark of the next one.
func TestCalendarQuietDayLeavesWhatWasApprovedAndTheOtherDays(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	ctx := context.Background()
	r.env.as(Actor{ID: "user:ui", Person: true})

	p := r.create(tin("2026-12-25T10:00", "UTC"))

	if _, err := r.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID}); err != nil {
		t.Fatal(err)
	}

	r.approve(p, p.Targets[0].ID)

	if _, err := r.m.QuietSet(ctx, "2026-12-25", "UTC", true); err != nil {
		t.Fatal(err)
	}

	if got := r.target(p, p.Targets[0].ID); got.State != core.TargetScheduled {
		t.Errorf("marking the day moved an approved post: %s", got.State)
	}

	// The next day, with no mark, is open to an agent.
	r.env.as(Actor{ID: "key:agent"})

	q := r.create(tin("2026-12-26T10:00", "UTC"))
	if _, err := r.m.Schedule(ctx, ScheduleInput{ID: q.ID, TargetID: q.Targets[0].ID}); err != nil {
		t.Errorf("an agent on a day that is not quiet: %v", err)
	}

	// Creating a draft on the quiet day is allowed: no rule stops a plan, only the request does.
	if _, err := r.m.Create(ctx, CreateInput{
		ProjectID: "prj_1", Title: "t", Export: "job_exp", Targets: []TargetInput{tin("2026-12-25T11:00", "UTC")},
	}); err != nil {
		t.Errorf("a draft on a quiet day: %v", err)
	}
}

// failTarget puts the first target of a publication in the failed state, as the scheduler would.
func failTarget(t *testing.T, r *rig, p *Publication) {
	t.Helper()

	ctx := context.Background()

	err := r.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		cur, err := tx.Store().Get(ctx, p.ID)
		if err != nil {
			return err
		}

		cur.Targets[0].State = core.TargetFailed

		return tx.Store().Save(ctx, &cur)
	})
	if err != nil {
		t.Fatal(err)
	}
}

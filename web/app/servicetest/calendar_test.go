package servicetest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
)

// The scopes of the day of the calendar (SDD-020 section 30A, SDD-007 section 5): read reads the plan and the export,
// edit writes a note, a slot and a copy, and a key limited to one project reads and writes nothing of the days of the
// whole calendar.
func TestCalendarOperationsHoldTheirScopesAndTheProjectLimit(t *testing.T) {
	c := newCalendar(t)
	p := c.create(c.target(c.at(time.Hour)))
	other := c.newProject("another")

	reader := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:r", Scopes: []string{service.ScopeRead}},
	)
	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:l", Scopes: service.AllScopes, ProjectID: other,
	})
	own := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:o", Scopes: service.AllScopes, ProjectID: c.project,
	})

	day := c.clock.now().Add(time.Hour).UTC().Format("2006-01-02")
	next := c.clock.now().Add(25 * time.Hour).UTC().Format("2006-01-02")

	plan := service.CalendarPlanRequest{From: day, To: next}
	export := service.CalendarExportRequest{From: day, To: next}
	note := "a note"

	// read: the plan and the export, nothing else.
	if _, err := c.svc.CalendarPlan(reader, plan); err != nil {
		t.Errorf("a read key plans: %v", err)
	}

	if _, err := c.svc.CalendarExport(reader, export); err != nil {
		t.Errorf("a read key exports: %v", err)
	}

	for name, run := range map[string]func() error{
		"day": func() error {
			_, err := c.svc.CalendarDaySet(reader, service.CalendarDaySetRequest{Date: day, Note: &note})

			return err
		},
		"slot": func() error {
			_, err := c.svc.CalendarSlotSave(reader, service.CalendarSlotSaveRequest{Network: "fake", Weekday: "tue", Time: "09:00"})

			return err
		},
		"remove": func() error {
			_, err := c.svc.CalendarSlotDelete(reader, service.CalendarSlotDeleteRequest{ID: "slt_0000000001"})

			return err
		},
		"duplicate": func() error {
			_, err := c.svc.PublicationDuplicate(reader, service.PublicationDuplicateRequest{ID: p.ID, At: "2026-12-02T18:00"})

			return err
		},
	} {
		if got := code(run()); got != service.CodeForbidden {
			t.Errorf("a read key: %s answered %q, want E_FORBIDDEN", name, got)
		}
	}

	// A key limited to another project cannot reach the plan, the days or the slots of the whole calendar, or
	// copy what it cannot see.
	for name, run := range map[string]func() error{
		"plan": func() error { _, err := c.svc.CalendarPlan(limited, plan); return err },
		"day": func() error {
			_, err := c.svc.CalendarDaySet(limited, service.CalendarDaySetRequest{Date: day, Note: &note})

			return err
		},
		"slot": func() error {
			_, err := c.svc.CalendarSlotSave(limited, service.CalendarSlotSaveRequest{Network: "fake", Weekday: "tue", Time: "09:00"})

			return err
		},
		"duplicate": func() error {
			_, err := c.svc.PublicationDuplicate(limited, service.PublicationDuplicateRequest{ID: p.ID, At: "2026-12-02T18:00"})

			return err
		},
	} {
		if got := code(run()); got != service.CodeForbidden && got != "E_NOT_FOUND" {
			t.Errorf("a key of another project: %s answered %q", name, got)
		}
	}

	// Its export holds nothing of the other project, and the key of the project holds its own post.
	out, err := c.svc.CalendarExport(limited, export)
	if err != nil || strings.Contains(out.Body, "BEGIN:VEVENT") {
		t.Errorf("the export of another project = %v, %v", out, err)
	}

	mine, err := c.svc.CalendarExport(own, export)
	if err != nil || strings.Count(mine.Body, "BEGIN:VEVENT") != 1 {
		t.Errorf("the export of the project = %v, %v", mine, err)
	}
}

// Marking a quiet day is a person's: the local token, a key and an agent get E_PRIVILEGED_SESSION whatever the scope
// they hold, and the day is as it was; a person marks it and anyone who reads sees it (SDD-020 section 30A.1).
func TestCalendarQuietDayIsMarkedByAPersonAndSeenByEveryone(t *testing.T) {
	c := newCalendar(t)
	day := c.clock.now().Add(48 * time.Hour).UTC().Format("2006-01-02")
	next := c.clock.now().Add(72 * time.Hour).UTC().Format("2006-01-02")
	mark := service.CalendarQuietSetRequest{Date: day, Zone: "UTC", Quiet: true}
	// A key of the local workspace holds every scope and is still not a person.
	aKey := service.WithCaller(context.Background(), service.Caller{Actor: "key:k", Scopes: service.AllScopes})
	who := map[string]context.Context{"the local token": c.as(token), "a key": aKey}

	for name, who := range who {
		if _, err := c.svc.CalendarQuietSet(who, mark); code(err) != service.CodePrivilegedSession {
			t.Errorf("%s marked a quiet day: %v", name, err)
		}
	}

	plan, err := c.svc.CalendarPlan(c.as(token), service.CalendarPlanRequest{From: day, To: next})
	if err != nil || len(plan.Days) != 0 {
		t.Fatalf("a refused mark left a day: %+v, %v", plan, err)
	}

	got, err := c.svc.CalendarQuietSet(c.asPerson(), mark)
	if err != nil || !got.Quiet {
		t.Fatalf("a person marks it: %+v, %v", got, err)
	}

	for name, who := range who {
		plan, err := c.svc.CalendarPlan(who, service.CalendarPlanRequest{From: day, To: next})
		if err != nil || len(plan.Days) != 1 || !plan.Days[0].Quiet {
			t.Errorf("%s reads %+v, %v", name, plan, err)
		}

		if _, err := c.svc.CalendarQuietSet(
			who,
			service.CalendarQuietSetRequest{Date: day, Zone: "UTC"},
		); code(
			err,
		) != service.CodePrivilegedSession {
			t.Errorf("%s cleared a quiet day: %v", name, err)
		}
	}

	if cleared, err := c.svc.CalendarQuietSet(
		c.asPerson(),
		service.CalendarQuietSetRequest{Date: day, Zone: "UTC"},
	); err != nil ||
		cleared.Quiet {
		t.Errorf("a person clears it: %+v, %v", cleared, err)
	}
}

// A person who schedules on a quiet day is asked to confirm, and only the confirmed call schedules; the same call
// from the local token is refused outright (D-146, SDD-020 section 30A.1).
func TestCalendarSchedulingOnAQuietDayNeedsAPersonWhoConfirms(t *testing.T) {
	c := newCalendar(t)
	at := c.clock.now().Add(48 * time.Hour)
	p := c.create(c.target(at.Format("2006-01-02T15:04")))

	if _, err := c.svc.CalendarQuietSet(c.asPerson(), service.CalendarQuietSetRequest{
		Date: at.Format("2006-01-02"), Zone: "UTC", Quiet: true,
	}); err != nil {
		t.Fatal(err)
	}

	req := service.PublicationScheduleRequest{ID: p.ID, TargetID: p.Targets[0].ID}

	if _, err := c.svc.PublicationSchedule(c.as(token), req); code(err) != service.CodePrivilegedSession {
		t.Errorf("the local token scheduled on a quiet day: %v", err)
	}

	var se *service.Error

	if _, err := c.svc.PublicationSchedule(
		c.asPerson(),
		req,
	); !errors.As(err, &se) || se.Code != "E_INVALID_STATE" ||
		se.Path != "confirm_quiet_day" {
		t.Fatalf("a person without the confirmation: %v", err)
	}

	if got := c.get(p).Target(p.Targets[0].ID); got.State != "draft" {
		t.Fatalf("a refused call moved the target to %s", got.State)
	}

	req.ConfirmQuietDay = true

	done, err := c.svc.PublicationSchedule(c.asPerson(), req)
	if err != nil || done.Target(p.Targets[0].ID).State != "waiting_approval" {
		t.Fatalf("the confirmed call = %+v, %v", done, err)
	}
}

// Two users of a network workspace have calendars of their own: a note, a quiet day and a slot of one are not read,
// written or removed by the other.
func TestCalendarDaysAndSlotsBelongToTheirOwner(t *testing.T) {
	c := newCalendar(t)
	one := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:one", Scopes: service.AllScopes, OwnerUserID: "usr_0000000001"},
	)
	two := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:two", Scopes: service.AllScopes, OwnerUserID: "usr_0000000002"},
	)
	day := c.clock.now().Add(48 * time.Hour).UTC().Format("2006-01-02")
	next := c.clock.now().Add(72 * time.Hour).UTC().Format("2006-01-02")
	note := "mine"

	if _, err := c.svc.CalendarDaySet(one, service.CalendarDaySetRequest{Date: day, Note: &note}); err != nil {
		t.Fatal(err)
	}

	slot, err := c.svc.CalendarSlotSave(
		one,
		service.CalendarSlotSaveRequest{Network: "fake", Weekday: "mon", Time: "09:00"},
	)
	if err != nil {
		t.Fatal(err)
	}

	theirs, err := c.svc.CalendarPlan(two, service.CalendarPlanRequest{From: day, To: next})
	if err != nil || len(theirs.Days) != 0 || len(theirs.Rules) != 0 {
		t.Errorf("the other user reads %+v, %v", theirs, err)
	}

	if _, err := c.svc.CalendarSlotDelete(
		two,
		service.CalendarSlotDeleteRequest{ID: slot.ID},
	); code(
		err,
	) != "E_NOT_FOUND" {
		t.Errorf("the other user removed a slot: %v", err)
	}

	mine, err := c.svc.CalendarPlan(one, service.CalendarPlanRequest{From: day, To: next})
	if err != nil || len(mine.Days) != 1 || len(mine.Rules) != 1 {
		t.Errorf("the owner reads %+v, %v", mine, err)
	}
}

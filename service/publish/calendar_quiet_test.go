package publish

import (
	"context"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
)

// A quiet day is the interval of its local date in the zone it was planned in: a post whose instant is inside it is
// refused whatever zone the post was written in, and one just outside it is not.
func TestCalendarQuietDayIsTheIntervalOfItsDateInItsZone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := newQuietRig(t) // 2026-12-25 in America/Sao_Paulo is quiet: 03:00Z to 03:00Z the next day.

	for name, c := range map[string]struct {
		at, zone string
		quiet    bool
	}{
		"inside, written in the zone of the day":          {"2026-12-25T10:00", "America/Sao_Paulo", true},
		"inside, written in UTC on the next UTC date":     {"2026-12-26T02:30", "UTC", true},
		"inside, written in Lisbon":                       {"2026-12-25T12:00", "Europe/Lisbon", true},
		"just before, the same date string in UTC":        {"2026-12-25T02:00", "UTC", false},
		"just after, the next day in the zone of the day": {"2026-12-26T00:30", "America/Sao_Paulo", false},
	} {
		q.env.as(q.agent)

		p := q.create(tin(c.at, c.zone))
		_, err := q.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID})

		if c.quiet {
			wantInvalid(t, err, KindPersonOnly, "at")
		} else if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Today is a day too: the job publish.upload has no time of its own and goes at the moment of the approval, so the
// guard of a caller that is not a person reads the clock.
func TestCalendarQuietNowRefusesOnlyWhoIsNotAPerson(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := newQuietRig(t)

	q.env.set(time.Date(2026, 12, 25, 15, 0, 0, 0, time.UTC))

	q.env.as(q.agent)
	wantInvalid(t, q.m.QuietNow(ctx, ""), KindPersonOnly, "at")

	q.env.as(q.person)

	if err := q.m.QuietNow(ctx, ""); err != nil {
		t.Errorf("a person is not stopped: %v", err)
	}

	// The next morning in Sao Paulo, no longer quiet.
	q.env.set(time.Date(2026, 12, 26, 4, 0, 0, 0, time.UTC))
	q.env.as(q.agent)

	if err := q.m.QuietNow(ctx, ""); err != nil {
		t.Errorf("the day after: %v", err)
	}

	// Another owner's quiet day is not this owner's.
	q.env.set(time.Date(2026, 12, 25, 15, 0, 0, 0, time.UTC))

	if err := q.m.QuietNow(ctx, "usr_b"); err != nil {
		t.Errorf("a day of another owner: %v", err)
	}
}

func TestDayContainsIsTheIntervalOfADayInAZone(t *testing.T) {
	t.Parallel()

	in := time.Date(2026, 12, 26, 2, 59, 0, 0, time.UTC)
	out := time.Date(2026, 12, 26, 3, 0, 0, 0, time.UTC)

	if !core.DayContains("2026-12-25", "America/Sao_Paulo", in) ||
		core.DayContains("2026-12-25", "America/Sao_Paulo", out) {
		t.Error("the last minute of the day in Sao Paulo is inside, the next is not")
	}

	if core.DayContains("2026-12-25", "Mars/Olympus", in) {
		t.Error("an unknown zone holds no instant")
	}
}

// The zone of a quiet day says where its interval starts, so changing it moves the day: an agent that may write the
// note of the day cannot move the zone of a day a person marked quiet, and then schedule in the hours it freed.
func TestCalendarQuietDayZoneIsMovedOnlyByAPerson(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := newQuietRig(t) // 2026-12-25 in America/Sao_Paulo is quiet: 03:00Z to 03:00Z the next day.

	q.env.as(q.agent)

	_, err := q.m.DaySet(ctx, DayInput{Date: "2026-12-25", Zone: "Pacific/Kiritimati", Note: new("moved")})
	wantInvalid(t, err, KindPersonOnly, "zone")

	if d, _, _ := q.env.store.Day(ctx, "", "2026-12-25"); d.Zone != "America/Sao_Paulo" || d.Note != "" || !d.Quiet {
		t.Fatalf("a refused call changed the day: %+v", d)
	}

	// 20:00 in Sao Paulo is 23:00Z, inside the quiet day, and stays refused.
	p := q.create(tin("2026-12-25T20:00", "America/Sao_Paulo"))
	_, err = q.m.Schedule(ctx, ScheduleInput{ID: p.ID, TargetID: p.Targets[0].ID})
	wantInvalid(t, err, KindPersonOnly, "at")

	// The note of the quiet day, in its own zone or with none, is still the agent's to write.
	for _, zone := range []string{"", "America/Sao_Paulo"} {
		if _, err := q.m.DaySet(ctx, DayInput{Date: "2026-12-25", Zone: zone, Note: new("holiday")}); err != nil {
			t.Errorf("an agent writes the note with the zone %q: %v", zone, err)
		}
	}

	// A person moves it.
	q.env.as(q.person)

	d, err := q.m.DaySet(ctx, DayInput{Date: "2026-12-25", Zone: "Europe/Lisbon"})
	if err != nil || d.Zone != "Europe/Lisbon" || !d.Quiet {
		t.Fatalf("a person moves the zone of the day: %+v, %v", d, err)
	}
}

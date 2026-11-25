package store

import (
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// The days of the calendar are read by owner and by range of dates, a day replaces itself, and a day with nothing
// in it is a deleted row (SDD-005 section 5.15.2).
func TestCalendarDaysAreKeptByOwnerAndDate(t *testing.T) {
	s := testStore(t)
	now := time.UnixMilli(1000).UTC()
	repo := s.Publications()

	put := func(owner, date, note string, quiet bool) {
		t.Helper()

		if err := repo.PutDay(
			ctx,
			owner,
			pub.Day{Date: date, Zone: "UTC", Note: note, Theme: "blue", Quiet: quiet, UpdatedAt: now},
		); err != nil {
			t.Fatal(err)
		}
	}

	put("", "2026-12-24", "a", false)
	put("", "2026-12-25", "b", true)
	put("", "2026-12-26", "c", false)
	put("usr_b", "2026-12-25", "theirs", false)
	put("", "2026-12-25", "b again", true)

	got, err := repo.Days(ctx, "", "2026-12-25", "2026-12-26")
	if err != nil || len(got) != 2 || got[0].Note != "b again" || !got[0].Quiet || got[1].Date != "2026-12-26" {
		t.Fatalf("days %+v, %v", got, err)
	}

	if theirs, _ := repo.Days(
		ctx,
		"usr_b",
		"2026-01-01",
		"2026-12-31",
	); len(theirs) != 1 ||
		theirs[0].Note != "theirs" {
		t.Errorf("another owner reads %+v", theirs)
	}

	if err := repo.PutDay(ctx, "", pub.Day{Date: "2026-12-25", Zone: "UTC"}); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := repo.Day(ctx, "", "2026-12-25"); ok || err != nil {
		t.Errorf("an empty day kept its row: %v, %v", ok, err)
	}
}

func TestCalendarSlotsAreKeptByOwner(t *testing.T) {
	s := testStore(t)
	now := time.UnixMilli(1000).UTC()
	repo := s.Publications()

	rule := core.Rule{
		ID:           "slt_1",
		Network:      "instagram",
		Weekday:      "tue",
		Time:         "18:00",
		Zone:         "UTC",
		ConnectionID: "con_1",
	}
	if err := repo.PutSlot(ctx, "", rule, now); err != nil {
		t.Fatal(err)
	}

	rule.Time = "19:00"
	if err := repo.PutSlot(ctx, "", rule, now); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Slots(ctx, "")
	if err != nil || len(got) != 1 || got[0] != rule {
		t.Fatalf("slots %+v, %v", got, err)
	}

	// Another owner can neither take the id nor remove the slot.
	if err := repo.PutSlot(
		ctx,
		"usr_b",
		core.Rule{ID: "slt_1", Network: "x", Weekday: "mon", Time: "10:00", Zone: "UTC"},
		now,
	); err == nil {
		t.Error("another owner replaced a slot")
	}

	if ok, err := repo.DeleteSlot(ctx, "usr_b", "slt_1"); ok || err != nil {
		t.Errorf("another owner removed a slot: %v, %v", ok, err)
	}

	if ok, err := repo.DeleteSlot(ctx, "", "slt_1"); !ok || err != nil {
		t.Errorf("remove: %v, %v", ok, err)
	}

	if got, _ := repo.Slots(ctx, ""); len(got) != 0 {
		t.Errorf("slots left: %+v", got)
	}
}

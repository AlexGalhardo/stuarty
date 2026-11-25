package publish

import (
	"context"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
)

// Day is what the calendar keeps about one local date (SDD-020 section 30A.1, SDD-005 section 5.15.2): a note, a
// theme and a quiet mark. A date with none of them has no row and reads as the empty Day.
type Day struct {
	// Date is the local date, 2026-12-25; Zone the IANA zone the day was planned in.
	Date  string `json:"date"`
	Zone  string `json:"zone"`
	Note  string `json:"note,omitempty"  untrusted:"true"`
	Theme string `json:"theme,omitempty"`
	// Quiet is the rule "no posts today": only a person sets or removes it.
	Quiet     bool      `json:"quiet,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Empty says whether the day holds nothing worth a row.
func (d Day) Empty() bool { return d.Note == "" && d.Theme == "" && !d.Quiet }

// CalendarStore is where the days and the slots live. A user of a network workspace reads and writes only the rows of
// their own owner; in local mode the owner is the empty text.
type CalendarStore interface {
	// Days returns the days with a row between two local dates, both inclusive, in the order of the date.
	Days(ctx context.Context, owner, fromDate, toDate string) ([]Day, error)
	// Day returns one day; ok is false when it has no row.
	Day(ctx context.Context, owner, date string) (d Day, ok bool, err error)
	// PutDay writes a day, or deletes its row when the day is empty.
	PutDay(ctx context.Context, owner string, d Day) error
	// Slots returns the slots of the owner, in the order of their id.
	Slots(ctx context.Context, owner string) ([]core.Rule, error)
	// PutSlot inserts or replaces a slot of the owner; the slot of another owner with that id is not touched.
	PutSlot(ctx context.Context, owner string, r core.Rule, now time.Time) error
	// DeleteSlot removes a slot of the owner; false when there is none.
	DeleteSlot(ctx context.Context, owner, id string) (bool, error)
}

// SlotFill names the target that fills an occurrence of a slot.
type SlotFill struct {
	PublicationID string `json:"publication_id"`
	TargetID      string `json:"target_id"`
}

// SlotView is an occurrence of a slot with the target that fills it, if any.
type SlotView struct {
	core.Occurrence

	Filled *SlotFill `json:"filled,omitempty"`
}

// Plan is the answer for a range of the calendar (SDD-020 section 30A.4).
type Plan struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	Zone string    `json:"zone"`
	// Days holds the days of the range that have a note, a theme or a mark.
	Days []Day `json:"days"`
	// Rules are the recurring slots; Slots their occurrences in the range.
	Rules []core.Rule `json:"rules"`
	Slots []SlotView  `json:"slots"`
	// Publications are those of the range, when asked, with the total before the page.
	Publications []Publication `json:"publications"`
	Total        int           `json:"total"`
}

package publish

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The day of the calendar (SDD-020 section 30A, author decision D-146): a note and a theme, a quiet mark, and the
// recurring slots that are expanded to instants when a range is read. Nothing here reads a clock or a store.

const (
	// MaxNoteRunes is the longest note of a day.
	MaxNoteRunes = 2000
	// MaxSlots is how many recurring slots a workspace holds.
	MaxSlots = 50
	// FillWindow is how long after the start of a slot a target still fills it.
	FillWindow = 60 * time.Minute
	// MaxExpandRange is the longest range slots are expanded for; a calendar asks for a month, a week or a day.
	MaxExpandRange = 400 * 24 * time.Hour
)

// ErrCalendar is a day, a note, a theme or a slot that cannot be accepted.
var ErrCalendar = errors.New("publish: invalid calendar entry")

// Themes are the names a day may take; the app maps each to a design token (the design system, "Calendar themes").
var Themes = []string{"blue", "green", "amber", "red", "teal", "gray"}

func calendarErr(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrCalendar}, args...)...)
}

// CheckTheme accepts an empty theme (none) or one of Themes.
func CheckTheme(theme string) error {
	if theme == "" || slices.Contains(Themes, theme) {
		return nil
	}

	return calendarErr("the theme %q is not one of %s", theme, strings.Join(Themes, ", "))
}

// CheckNote accepts plain text of at most MaxNoteRunes with line breaks and tabs and no other control character.
func CheckNote(note string) error {
	if !utf8.ValidString(note) {
		return calendarErr("the note is not valid text")
	}

	if utf8.RuneCountInString(note) > MaxNoteRunes {
		return calendarErr("the note has more than %d characters", MaxNoteRunes)
	}

	for _, r := range note {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			return calendarErr("the note holds a control character")
		}
	}

	return nil
}

// ParseDate reads a local date such as 2026-12-25 and returns it in that form; anything else is an error.
func ParseDate(s string) (string, error) {
	t, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	if err != nil {
		return "", calendarErr("%q is not a date such as 2026-12-25", s)
	}

	return t.Format(time.DateOnly), nil
}

// LocalDate is the local date of an instant in a zone: the day a target belongs to (section 30A.1).
func LocalDate(at time.Time, zone string) (string, error) {
	loc, err := LoadZone(zone)
	if err != nil {
		return "", err
	}

	return at.In(loc).Format(time.DateOnly), nil
}

// DayContains says whether an instant falls on a local date in a zone: the day is the interval from the midnight that
// starts that date in that zone to the next one, so a day planned in Sao Paulo holds 23:30 there, which is the next
// day in UTC.
func DayContains(date, zone string, at time.Time) bool {
	loc, err := LoadZone(zone)
	if err != nil {
		return false
	}

	return at.In(loc).Format(time.DateOnly) == date
}

// QuietVerdict is what the quiet-day rule says about scheduling on a day.
type QuietVerdict int

const (
	// QuietClear says the day is not quiet, or a person confirmed.
	QuietClear QuietVerdict = iota
	// QuietNeedsPerson says the day is quiet and the caller is not a person.
	QuietNeedsPerson
	// QuietNeedsConfirm says the day is quiet and a person did not confirm yet.
	QuietNeedsConfirm
)

// QuietCheck is the rule of a quiet day (D-146): a caller that is not a person never schedules on it, whatever it
// passes; a person does when the call carries the confirmation.
func QuietCheck(quiet, person, confirmed bool) QuietVerdict {
	switch {
	case !quiet:
		return QuietClear
	case !person:
		return QuietNeedsPerson
	case !confirmed:
		return QuietNeedsConfirm
	default:
		return QuietClear
	}
}

// Weekdays are the names a slot takes, in the order of the week from Monday.
var Weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

var (
	timeOfDay   = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	networkName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

// Rule is a recurring slot: every week, on a weekday, at a local time in a zone, on a network (and optionally on
// one account of it). It is a reminder and holds no content.
type Rule struct {
	ID           string `json:"id"`
	Network      string `json:"network"`
	Weekday      string `json:"weekday"`
	Time         string `json:"time"`
	Zone         string `json:"zone"`
	ConnectionID string `json:"connection_id,omitempty"`
}

// CheckRule validates a slot and returns it with the zone filled in (UTC when empty).
func CheckRule(r Rule) (Rule, error) {
	if !networkName.MatchString(r.Network) {
		return r, calendarErr("the network %q is not a network name such as instagram", r.Network)
	}

	if !slices.Contains(Weekdays, r.Weekday) {
		return r, calendarErr("the weekday %q is not one of %s", r.Weekday, strings.Join(Weekdays, ", "))
	}

	if !timeOfDay.MatchString(r.Time) {
		return r, calendarErr("the time %q is not a time such as 18:00", r.Time)
	}

	if r.Zone == "" {
		r.Zone = "UTC"
	}

	if _, err := LoadZone(r.Zone); err != nil {
		return r, calendarErr("%s", strings.TrimPrefix(err.Error(), "publish: invalid time: "))
	}

	if len(r.ConnectionID) > 64 || strings.ContainsAny(r.ConnectionID, " \t\r\n") {
		return r, calendarErr("the connection id is not an id")
	}

	return r, nil
}

// Occurrence is a slot on one day of a range.
type Occurrence struct {
	SlotID       string `json:"slot_id"`
	Network      string `json:"network"`
	ConnectionID string `json:"connection_id,omitempty"`
	// Date and Time are the wall time of the slot, in Zone; At is the instant, in UTC.
	Date       string     `json:"date"`
	Time       string     `json:"time"`
	Zone       string     `json:"zone"`
	At         time.Time  `json:"at"`
	Adjustment Adjustment `json:"adjustment,omitempty"`
}

// ExpandSlots lists the occurrences of the slots whose instant is in [from, to), in the order of the instant. It
// walks the local dates of each slot's own zone, so a weekly 18:00 stays at 18:00 across a change of the clock; a
// wall time the clock jumps over is the first instant after the jump (Adjustment gap), and one that happens twice
// the first of them (ambiguous), as LocalInstant says.
func ExpandSlots(rules []Rule, from, to time.Time) ([]Occurrence, error) {
	if !to.After(from) {
		return nil, nil
	}

	if to.Sub(from) > MaxExpandRange {
		return nil, calendarErr("a range of more than %d days", int(MaxExpandRange.Hours()/24))
	}

	var out []Occurrence

	for _, r := range rules {
		loc, err := LoadZone(r.Zone)
		if err != nil {
			return nil, err
		}

		// The local dates one day either side of the range cover every wall time whose instant is in it.
		first := from.In(loc)
		day := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
		last := to.In(loc)
		end := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)

		for ; !day.After(end); day = day.AddDate(0, 0, 1) {
			if weekdayName(day.Weekday()) != r.Weekday {
				continue
			}

			date := day.Format(time.DateOnly)

			at, adj, err := LocalInstant(date+"T"+r.Time, loc)
			if err != nil {
				return nil, err
			}

			if at.Before(from) || !at.Before(to) {
				continue
			}

			out = append(out, Occurrence{
				SlotID: r.ID, Network: r.Network, ConnectionID: r.ConnectionID, Date: date, Time: r.Time,
				Zone: r.Zone, At: at, Adjustment: adj,
			})
		}
	}

	slices.SortStableFunc(out, func(a, b Occurrence) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}

		return strings.Compare(a.SlotID, b.SlotID)
	})

	return out, nil
}

func weekdayName(d time.Weekday) string {
	return Weekdays[(int(d)+6)%7]
}

// Placed is a target as the fill of a slot reads it: where it goes and when.
type Placed struct {
	Key          string
	Network      string
	ConnectionID string
	At           time.Time
}

// FillSlots says which target fills each occurrence: the first, by time, of the same network (and of the named
// account) whose instant is in [at, at + FillWindow). A target fills at most one occurrence. The answer has one
// entry per occurrence, the key of the target or the empty text for an empty slot.
func FillSlots(occ []Occurrence, placed []Placed) []string {
	sorted := slices.SortedStableFunc(slices.Values(placed), func(a, b Placed) int { return a.At.Compare(b.At) })
	used := make([]bool, len(sorted))
	out := make([]string, len(occ))

	for i := range occ {
		o := &occ[i]

		for j := range sorted {
			p := &sorted[j]
			if used[j] || p.Network != o.Network || (o.ConnectionID != "" && p.ConnectionID != o.ConnectionID) {
				continue
			}

			if p.At.Before(o.At) || !p.At.Before(o.At.Add(FillWindow)) {
				continue
			}

			used[j], out[i] = true, p.Key

			break
		}
	}

	return out
}

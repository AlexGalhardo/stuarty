package publish

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDayNoteThemeAndDate(t *testing.T) {
	t.Parallel()

	if err := CheckTheme("amber"); err != nil {
		t.Errorf("amber: %v", err)
	}

	if err := CheckTheme(""); err != nil {
		t.Errorf("no theme: %v", err)
	}

	if err := CheckTheme("#ff0000"); !errors.Is(err, ErrCalendar) {
		t.Errorf("a colour is not a theme: %v", err)
	}

	if err := CheckNote("long video day\nsecond line\tindented"); err != nil {
		t.Errorf("a plain note: %v", err)
	}

	if err := CheckNote(strings.Repeat("a", MaxNoteRunes+1)); !errors.Is(err, ErrCalendar) {
		t.Errorf("a long note: %v", err)
	}

	if err := CheckNote(strings.Repeat("é", MaxNoteRunes)); err != nil {
		t.Errorf("the limit counts characters, not bytes: %v", err)
	}

	if err := CheckNote("bell\x07"); !errors.Is(err, ErrCalendar) {
		t.Errorf("a control character: %v", err)
	}

	for _, bad := range []string{"", "2026-13-01", "25/12/2026", "2026-12-25T10:00"} {
		if _, err := ParseDate(bad); !errors.Is(err, ErrCalendar) {
			t.Errorf("ParseDate(%q) = %v", bad, err)
		}
	}

	if got, err := ParseDate(" 2026-12-25 "); err != nil || got != "2026-12-25" {
		t.Errorf("ParseDate = %q, %v", got, err)
	}
}

// A post at 23:30 in Sao Paulo belongs to the 25th there, though it is the 26th in UTC.
func TestDayOfAnInstantIsItsLocalDateInItsZone(t *testing.T) {
	t.Parallel()

	instant := at(t, "2026-12-26T02:30:00Z")

	for zone, want := range map[string]string{"America/Sao_Paulo": "2026-12-25", "UTC": "2026-12-26", "": "2026-12-26"} {
		got, err := LocalDate(instant, zone)
		if err != nil || got != want {
			t.Errorf("LocalDate in %q = %q, %v; want %q", zone, got, err, want)
		}
	}

	if _, err := LocalDate(instant, "Mars/Olympus"); err == nil {
		t.Error("an unknown zone is an error")
	}
}

func TestDayQuietRule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		quiet, person, confirmed bool
		want                     QuietVerdict
	}{
		{false, false, false, QuietClear},
		{false, true, false, QuietClear},
		{true, false, false, QuietNeedsPerson},
		// An agent that passes the confirmation is still refused: the flag is a person's.
		{true, false, true, QuietNeedsPerson},
		{true, true, false, QuietNeedsConfirm},
		{true, true, true, QuietClear},
	}

	for _, c := range cases {
		if got := QuietCheck(c.quiet, c.person, c.confirmed); got != c.want {
			t.Errorf("QuietCheck(%+v) = %v, want %v", c, got, c.want)
		}
	}
}

func TestSlotsAreChecked(t *testing.T) {
	t.Parallel()

	ok := Rule{Network: "instagram", Weekday: "tue", Time: "18:00", Zone: "America/Sao_Paulo"}
	if _, err := CheckRule(ok); err != nil {
		t.Fatalf("a good slot: %v", err)
	}

	if r, err := CheckRule(Rule{Network: "x", Weekday: "mon", Time: "09:05"}); err != nil || r.Zone != "UTC" {
		t.Errorf("an empty zone is UTC: %+v, %v", r, err)
	}

	for name, bad := range map[string]Rule{
		"network": {Network: "Insta gram", Weekday: "tue", Time: "18:00"},
		"weekday": {Network: "x", Weekday: "tuesday", Time: "18:00"},
		"time":    {Network: "x", Weekday: "tue", Time: "24:00"},
		"short":   {Network: "x", Weekday: "tue", Time: "9:00"},
		"zone":    {Network: "x", Weekday: "tue", Time: "18:00", Zone: "Local"},
		"conn":    {Network: "x", Weekday: "tue", Time: "18:00", ConnectionID: "a b"},
	} {
		if _, err := CheckRule(bad); !errors.Is(err, ErrCalendar) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Every Tuesday 18:00 in Lisbon keeps its wall time across the change of the clock of 2026-03-29, so the instant
// moves by an hour.
func TestSlotsExpandAcrossTheChangeToSummerTime(t *testing.T) {
	t.Parallel()

	rules := []Rule{{ID: "slt_1", Network: "instagram", Weekday: "tue", Time: "18:00", Zone: "Europe/Lisbon"}}

	got, err := ExpandSlots(rules, at(t, "2026-03-23T00:00:00Z"), at(t, "2026-04-07T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"2026-03-24T18:00:00Z", "2026-03-31T17:00:00Z"}

	if len(got) != len(want) {
		t.Fatalf("%d occurrences, want %d: %+v", len(got), len(want), got)
	}

	for i, o := range got {
		if o.At.Format(time.RFC3339) != want[i] || o.Time != "18:00" || o.Adjustment != AdjustNone {
			t.Errorf("occurrence %d = %+v, want %s at 18:00 local", i, o, want[i])
		}
	}

	if got[0].Date != "2026-03-24" || got[1].Date != "2026-03-31" {
		t.Errorf("dates %s, %s", got[0].Date, got[1].Date)
	}
}

func TestSlotsExpandWhenTheWallTimeDoesNotExistOrHappensTwice(t *testing.T) {
	t.Parallel()

	gap := []Rule{{ID: "slt_g", Network: "x", Weekday: "sun", Time: "01:30", Zone: "Europe/Lisbon"}}

	// 2026-03-29: the clock jumps from 01:00 to 02:00 in Lisbon; 01:30 does not exist.
	got, err := ExpandSlots(gap, at(t, "2026-03-29T00:00:00Z"), at(t, "2026-03-30T00:00:00Z"))
	if err != nil || len(got) != 1 {
		t.Fatalf("gap: %+v, %v", got, err)
	}

	// The wall time 01:30 read as if it were UTC, which is 02:30 in Lisbon after the jump.
	if got[0].Adjustment != AdjustGap || got[0].At.Format(time.RFC3339) != "2026-03-29T01:30:00Z" {
		t.Errorf("gap occurrence = %+v", got[0])
	}

	twice := []Rule{{ID: "slt_a", Network: "x", Weekday: "sun", Time: "01:30", Zone: "Europe/Lisbon"}}

	// 2026-10-25: the clock goes back from 02:00 to 01:00; 01:30 happens twice and the first is 00:30 UTC.
	got, err = ExpandSlots(twice, at(t, "2026-10-25T00:00:00Z"), at(t, "2026-10-26T00:00:00Z"))
	if err != nil || len(got) != 1 {
		t.Fatalf("ambiguous: %+v, %v", got, err)
	}

	if got[0].Adjustment != AdjustAmbiguous || got[0].At.Format(time.RFC3339) != "2026-10-25T00:30:00Z" {
		t.Errorf("ambiguous occurrence = %+v", got[0])
	}
}

func TestSlotsExpandAcrossTheChangeBackAndInAZoneWithNone(t *testing.T) {
	t.Parallel()

	rules := []Rule{
		{ID: "slt_l", Network: "x", Weekday: "mon", Time: "09:00", Zone: "Europe/Lisbon"},
		{ID: "slt_s", Network: "x", Weekday: "mon", Time: "09:00", Zone: "America/Sao_Paulo"},
	}

	got, err := ExpandSlots(rules, at(t, "2026-10-19T00:00:00Z"), at(t, "2026-11-03T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}

	lisbon := map[string]string{}
	saoPaulo := map[string]string{}

	for _, o := range got {
		if o.SlotID == "slt_l" {
			lisbon[o.Date] = o.At.Format("15:04")
		} else {
			saoPaulo[o.Date] = o.At.Format("15:04")
		}
	}

	// Lisbon is on summer time (UTC+1) until the 25th, then on UTC; Sao Paulo has no change.
	if lisbon["2026-10-19"] != "08:00" || lisbon["2026-10-26"] != "09:00" || lisbon["2026-11-02"] != "09:00" {
		t.Errorf("Lisbon instants %v", lisbon)
	}

	if saoPaulo["2026-10-19"] != "12:00" || saoPaulo["2026-10-26"] != "12:00" {
		t.Errorf("Sao Paulo instants %v", saoPaulo)
	}
}

// The range is read in instants: a slot late in the evening in a western zone is on the next UTC day, and it is
// listed for the range that holds its instant, not the one that holds its date.
func TestSlotsExpandByInstantAndDayRange(t *testing.T) {
	t.Parallel()

	rules := []Rule{{ID: "slt_1", Network: "x", Weekday: "fri", Time: "22:00", Zone: "America/Sao_Paulo"}}

	// Friday 2026-12-25 22:00 in Sao Paulo is Saturday 01:00 UTC.
	dayOfFriday := func(zone string) (time.Time, time.Time) {
		from, err := ParseBound("2026-12-25", zone)
		if err != nil {
			t.Fatal(err)
		}

		to, err := ParseBound("2026-12-26", zone)
		if err != nil {
			t.Fatal(err)
		}

		return from, to
	}

	from, to := dayOfFriday("America/Sao_Paulo")

	got, err := ExpandSlots(rules, from, to)
	if err != nil || len(got) != 1 || got[0].Date != "2026-12-25" {
		t.Fatalf("Sao Paulo day: %+v, %v", got, err)
	}

	from, to = dayOfFriday("UTC")

	got, err = ExpandSlots(rules, from, to)
	if err != nil || len(got) != 0 {
		t.Errorf("UTC day of the 25th holds no instant of it: %+v, %v", got, err)
	}

	if _, err := ExpandSlots(rules, from, from.Add(MaxExpandRange+time.Hour)); !errors.Is(err, ErrCalendar) {
		t.Errorf("a range of years: %v", err)
	}

	if got, err := ExpandSlots(rules, to, from); err != nil || got != nil {
		t.Errorf("an empty range: %+v, %v", got, err)
	}
}

func TestSlotsAreFilledByTheFirstMatchingTarget(t *testing.T) {
	t.Parallel()

	rules := []Rule{
		{ID: "slt_a", Network: "instagram", Weekday: "tue", Time: "18:00", Zone: "UTC"},
		{ID: "slt_b", Network: "youtube", Weekday: "tue", Time: "18:00", Zone: "UTC", ConnectionID: "con_yt2"},
	}

	occ, err := ExpandSlots(rules, at(t, "2026-12-01T00:00:00Z"), at(t, "2026-12-02T00:00:00Z"))
	if err != nil || len(occ) != 2 {
		t.Fatalf("%+v, %v", occ, err)
	}

	placed := []Placed{
		{Key: "late", Network: "instagram", At: at(t, "2026-12-01T19:30:00Z")},
		{Key: "inside", Network: "instagram", At: at(t, "2026-12-01T18:59:00Z")},
		{Key: "other-account", Network: "youtube", ConnectionID: "con_yt1", At: at(t, "2026-12-01T18:00:00Z")},
	}

	got := FillSlots(occ, placed)
	if len(got) != 2 {
		t.Fatalf("%v", got)
	}

	byNetwork := map[string]string{}
	for i, o := range occ {
		byNetwork[o.Network] = got[i]
	}

	if byNetwork["instagram"] != "inside" {
		t.Errorf("the target inside the hour fills the slot, got %q", byNetwork["instagram"])
	}

	if byNetwork["youtube"] != "" {
		t.Errorf("a slot of one account is not filled by another, got %q", byNetwork["youtube"])
	}

	// One target fills one slot.
	twice := []Occurrence{occ[0], occ[0]}
	if got := FillSlots(twice, placed[1:2]); got[0] != "inside" || got[1] != "" {
		t.Errorf("one target, two slots: %v", got)
	}
}

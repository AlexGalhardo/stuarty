package publish

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
)

// The operations of the day of the calendar (SDD-020 section 30A): notes and themes, quiet days, recurring slots, the
// plan of a range, duplicate and the .ics export. The rules are in core/publish; this file reads and writes through
// the store and applies who may do what.

const (
	hintPersonMarksQuiet = "a person marks or clears a quiet day in the Publish page of the Stuarty app; the local " +
		"token, API keys and the agent surfaces cannot"
	hintQuietDay = "that day is marked as quiet: a person schedules on it in the Publish page of the Stuarty app and " +
		"confirms there; choose another day"
	hintQuietConfirm = "ask the person whether to schedule on the quiet day, and repeat the call with confirm_quiet_day true"
	// maxExportPublications bounds one export: a calendar of a creator is far from it.
	maxExportPublications = 5000
)

// calendarError turns what core/publish refuses into a validation error at a path.
func calendarError(path string, err error) error {
	if errors.Is(err, core.ErrCalendar) || errors.Is(err, core.ErrTime) {
		msg := strings.TrimPrefix(
			strings.TrimPrefix(err.Error(), "publish: invalid calendar entry: "),
			"publish: invalid time: ",
		)

		return invalid(path, "%s", msg)
	}

	return err
}

func (m *Manager) owner(ctx context.Context) string { return m.env.Actor(ctx).OwnerUserID }

type quietConfirmKey struct{}

// withQuietConfirm carries the person's confirmation of a post on a quiet day to ask, the one place every path that
// asks for approval goes through.
func withQuietConfirm(ctx context.Context, confirmed bool) context.Context {
	return context.WithValue(ctx, quietConfirmKey{}, confirmed)
}

// quietDayAt returns the date of the quiet day an instant falls on, or "". A day is the interval of its local date in
// the zone it was planned in, so the rows within two days of the UTC date of the instant are read and each is asked.
func quietDayAt(ctx context.Context, s Store, owner string, at time.Time) (string, error) {
	utc := at.UTC()
	from, to := utc.AddDate(0, 0, -2).Format(time.DateOnly), utc.AddDate(0, 0, 2).Format(time.DateOnly)

	days, err := s.Days(ctx, owner, from, to)
	if err != nil {
		return "", err
	}

	for i := range days {
		if days[i].Quiet && core.DayContains(days[i].Date, days[i].Zone, at) {
			return days[i].Date, nil
		}
	}

	return "", nil
}

// checkQuiet is the rule of a quiet day (SDD-020 section 30A.1, T-180): a target whose time falls on a day marked as
// quiet is not asked for approval by a caller that is not a person, and a person must confirm. It runs inside the
// transaction of the change, so a mark made a moment ago is seen.
func (m *Manager) checkQuiet(ctx context.Context, tx Tx, p *Publication, t *Target) error {
	date, err := quietDayAt(ctx, tx.Store(), p.OwnerUserID, *t.ScheduledAt)
	if err != nil {
		return err
	}

	confirmed, _ := ctx.Value(quietConfirmKey{}).(bool)

	switch core.QuietCheck(date != "", m.env.Actor(ctx).Person, confirmed) {
	case core.QuietNeedsPerson:
		return quietRefusal(date)
	case core.QuietNeedsConfirm:
		return &Error{
			Kind: KindInvalidState, Path: "confirm_quiet_day", Hint: hintQuietConfirm,
			Message: date + " is marked as a quiet day (no posts).",
		}
	case core.QuietClear:
	}

	return nil
}

func quietRefusal(date string) *Error {
	return &Error{
		Kind: KindPersonOnly, Path: "at", Hint: hintQuietDay,
		Message: date + " is a quiet day (no posts): only a person schedules on it.",
	}
}

// QuietNow refuses a caller that is not a person when the moment is on a quiet day. It guards the one path that
// sends at the moment of the approval and has no time of its own, the job publish.upload (PublishNow): a person's own
// is not stopped, since the person reads the request and decides it (SDD-020 section 30A.1).
func (m *Manager) QuietNow(ctx context.Context, owner string) error {
	if m.env.Actor(ctx).Person {
		return nil
	}

	date, err := quietDayAt(ctx, m.env.Store(), owner, m.env.Now())
	if err != nil || date == "" {
		return err
	}

	return quietRefusal(date)
}

// DayInput changes the note and the theme of a day; a nil member is left as it is and an empty one clears it.
type DayInput struct {
	Date  string  `json:"date"`
	Zone  string  `json:"zone,omitempty"`
	Note  *string `json:"note,omitempty"`
	Theme *string `json:"theme,omitempty"`
}

func (m *Manager) putDay(
	ctx context.Context, date, zone string, change func(d *Day) error,
) (*Day, error) {
	date, err := core.ParseDate(date)
	if err != nil {
		return nil, calendarError("date", err)
	}

	if zone != "" {
		if _, zerr := core.LoadZone(zone); zerr != nil {
			return nil, calendarError("zone", zerr)
		}
	}

	owner := m.owner(ctx)

	var out Day

	err = m.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		d, _, err := tx.Store().Day(ctx, owner, date)
		if err != nil {
			return err
		}

		d.Date = date

		if zone != "" {
			d.Zone = zone
		}

		if d.Zone == "" {
			d.Zone = "UTC"
		}

		if err := change(&d); err != nil {
			return err
		}

		d.UpdatedAt = m.env.Now()
		out = d

		return tx.Store().PutDay(ctx, owner, d)
	})
	if err != nil {
		return nil, err
	}

	return &out, nil
}

// DaySet writes the note and the theme of a day. A day left with no note, no theme and no mark has no row.
func (m *Manager) DaySet(ctx context.Context, in DayInput) (*Day, error) {
	if in.Note != nil {
		if err := core.CheckNote(*in.Note); err != nil {
			return nil, calendarError("note", err)
		}
	}

	if in.Theme != nil {
		if err := core.CheckTheme(*in.Theme); err != nil {
			return nil, calendarError("theme", err)
		}
	}

	return m.putDay(ctx, in.Date, in.Zone, func(d *Day) error {
		if in.Note != nil {
			d.Note = *in.Note
		}

		if in.Theme != nil {
			d.Theme = *in.Theme
		}

		return nil
	})
}

// QuietSet marks a day as quiet or clears the mark. Only a person does (D-146): every other caller is refused and the
// day is as it was. The note and the theme of the day are kept.
func (m *Manager) QuietSet(ctx context.Context, date, zone string, quiet bool) (*Day, error) {
	if !m.env.Actor(ctx).Person {
		return nil, &Error{
			Kind: KindPersonOnly, Path: "date", Message: "only a person marks or clears a quiet day.",
			Hint: hintPersonMarksQuiet,
		}
	}

	return m.putDay(ctx, date, zone, func(d *Day) error {
		d.Quiet = quiet

		return nil
	})
}

// SlotInput is a recurring slot to save; an empty ID makes a new one.
type SlotInput struct {
	ID           string `json:"id,omitempty"`
	Network      string `json:"network"`
	Weekday      string `json:"weekday"`
	Time         string `json:"time"`
	Zone         string `json:"zone,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
}

// SlotSave creates a slot or replaces one of the caller's. A slot is a reminder: it opens no connection and sends
// nothing.
func (m *Manager) SlotSave(ctx context.Context, in SlotInput) (*core.Rule, error) {
	r, err := core.CheckRule(core.Rule{
		ID:           in.ID,
		Network:      in.Network,
		Weekday:      in.Weekday,
		Time:         in.Time,
		Zone:         in.Zone,
		ConnectionID: in.ConnectionID,
	})
	if err != nil {
		return nil, calendarError("", err)
	}

	owner := m.owner(ctx)

	err = m.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		all, err := tx.Store().Slots(ctx, owner)
		if err != nil {
			return err
		}

		exists := slices.ContainsFunc(all, func(s core.Rule) bool { return s.ID == r.ID })

		switch {
		case r.ID == "":
			if len(all) >= core.MaxSlots {
				return invalid("", "at most %d slots.", core.MaxSlots).hint("remove a slot you no longer use")
			}

			r.ID = m.env.NewID("slt")
		case !exists:
			return ErrNotFound
		}

		return tx.Store().PutSlot(ctx, owner, r, m.env.Now())
	})
	if err != nil {
		return nil, err
	}

	return &r, nil
}

// SlotDelete removes a slot of the caller's.
func (m *Manager) SlotDelete(ctx context.Context, id string) error {
	owner := m.owner(ctx)

	return m.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		ok, err := tx.Store().DeleteSlot(ctx, owner, id)
		if err != nil {
			return err
		}

		if !ok {
			return ErrNotFound
		}

		return nil
	})
}

// PlanInput asks for a range: From inclusive and To exclusive, read in Zone by the service. Filter selects the
// publications (the project of a limited key, the owner of a user, the page) and is used for the fill of the slots
// whether or not the answer carries the publications.
type PlanInput struct {
	From, To     time.Time
	Zone         string
	Filter       Filter
	Publications bool
}

// Plan reads the days, the slots and, when asked, the publications of a range.
func (m *Manager) Plan(ctx context.Context, in PlanInput) (*Plan, error) {
	loc, err := core.LoadZone(in.Zone)
	if err != nil {
		return nil, calendarError("zone", err)
	}

	if !in.To.After(in.From) {
		return nil, invalid("to", "must be after from.")
	}

	owner := m.owner(ctx)
	store := m.env.Store()

	rules, err := store.Slots(ctx, owner)
	if err != nil {
		return nil, err
	}

	occ, err := core.ExpandSlots(rules, in.From, in.To)
	if err != nil {
		return nil, calendarError("to", err)
	}

	zone := in.Zone
	if zone == "" {
		zone = "UTC"
	}

	// Every local date the range touches, the last instant of the range included.
	fromDate := in.From.In(loc).Format(time.DateOnly)
	toDate := in.To.Add(-time.Millisecond).In(loc).Format(time.DateOnly)

	days, err := store.Days(ctx, owner, fromDate, toDate)
	if err != nil {
		return nil, err
	}

	plan := &Plan{
		From: in.From.UTC(), To: in.To.UTC(), Zone: zone, Days: days, Rules: rules, Publications: []Publication{},
	}

	if plan.Days == nil {
		plan.Days = []Day{}
	}

	if plan.Rules == nil {
		plan.Rules = []core.Rule{}
	}

	if len(occ) > 0 {
		if err := m.fill(ctx, in, occ, plan); err != nil {
			return nil, err
		}
	} else {
		plan.Slots = []SlotView{}
	}

	if in.Publications {
		f := in.Filter
		f.From, f.To = &in.From, &in.To

		list, total, err := m.List(ctx, f)
		if err != nil {
			return nil, err
		}

		plan.Publications, plan.Total = list, total
	}

	return plan, nil
}

// fill says which target fills each occurrence, from every publication of the range.
func (m *Manager) fill(ctx context.Context, in PlanInput, occ []core.Occurrence, plan *Plan) error {
	f := in.Filter
	f.From, f.To, f.Offset, f.Limit = &in.From, &in.To, 0, maxListLimit

	pubs, _, err := m.env.Store().List(ctx, f)
	if err != nil {
		return err
	}

	var (
		placed []core.Placed
		where  = map[string]SlotFill{}
	)

	for i := range pubs {
		for j := range pubs[i].Targets {
			t := &pubs[i].Targets[j]
			if t.ScheduledAt == nil || t.State == core.TargetCancelled {
				continue
			}

			placed = append(
				placed,
				core.Placed{Key: t.ID, Network: t.Network, ConnectionID: t.ConnectionID, At: *t.ScheduledAt},
			)
			where[t.ID] = SlotFill{PublicationID: pubs[i].ID, TargetID: t.ID}
		}
	}

	filled := core.FillSlots(occ, placed)
	plan.Slots = make([]SlotView, len(occ))

	for i := range occ {
		plan.Slots[i] = SlotView{Occurrence: occ[i]}

		if key := filled[i]; key != "" {
			w := where[key]
			plan.Slots[i].Filled = &w
		}
	}

	return nil
}

// Export writes the targets of a range as an iCalendar file (SDD-020 section 30A.5): the ones that have a time and
// are not cancelled, with UTC times. It reads only, and carries no id, address or credential.
func (m *Manager) Export(ctx context.Context, from, to time.Time, f Filter) (string, error) {
	if !to.After(from) {
		return "", invalid("to", "must be after from.")
	}

	f.From, f.To, f.Offset, f.Limit = &from, &to, 0, maxListLimit

	var events []core.Event

	for seen := 0; seen < maxExportPublications; {
		list, total, err := m.env.Store().List(ctx, f)
		if err != nil {
			return "", err
		}

		for i := range list {
			events = append(events, eventsOf(&list[i], from, to)...)
		}

		seen += len(list)
		f.Offset += len(list)

		if len(list) == 0 || seen >= total {
			break
		}
	}

	return core.WriteICS(events, m.env.Now()), nil
}

func eventsOf(p *Publication, from, to time.Time) []core.Event {
	var out []core.Event

	for i := range p.Targets {
		t := &p.Targets[i]
		if t.ScheduledAt == nil || t.State == core.TargetCancelled || t.ScheduledAt.Before(from) ||
			!t.ScheduledAt.Before(to) {
			continue
		}

		title, text := t.Fields.Title, t.Fields.Description
		if title == "" {
			title = p.Title
		}

		if text == "" {
			text = p.Text
		}

		out = append(out, core.Event{
			ID: t.ID, At: *t.ScheduledAt, Network: t.Network, Title: title, Text: text, State: string(t.State),
			Confirmed: t.State.Approved() || t.State == core.TargetPublishing || t.State == core.TargetPublished ||
				t.State == core.TargetSentAsDraft,
		})
	}

	return out
}

// DuplicateInput copies a publication to another day or hour.
type DuplicateInput struct {
	ID string `json:"id"`
	// TargetID names the one target to copy; empty copies every target that is not cancelled.
	TargetID string `json:"target_id,omitempty"`
	// At is the new time: an instant, or a local date and time read in Zone; Zone defaults to the zone of the target.
	At   string `json:"at"`
	Zone string `json:"zone,omitempty"`
}

// Duplicate makes a new publication with the same title, text, files and targets, each a draft at the new time with
// no approval, no request and none of the result (SDD-020 section 30A.3). The original is not changed, and scheduling
// the copy asks for approval like any other.
func (m *Manager) Duplicate(ctx context.Context, in DuplicateInput) (*Publication, error) {
	if strings.TrimSpace(in.At) == "" {
		return nil, invalid("at", "give the new time of the copy.").hint("for example 2026-12-02T18:00 with a zone")
	}

	src, err := m.env.Store().Get(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	var from []Target

	for i := range src.Targets {
		t := src.Targets[i]

		switch {
		case in.TargetID != "" && t.ID != in.TargetID:
		case in.TargetID == "" && t.State == core.TargetCancelled:
		default:
			from = append(from, t)
		}
	}

	if len(from) == 0 {
		if in.TargetID != "" {
			return nil, ErrNotFound
		}

		return nil, state("every post of this publication was cancelled: there is nothing to copy.")
	}

	now, actor := m.env.Now(), m.env.Actor(ctx)
	p := &Publication{
		ID: m.env.NewID("pub"), ProjectID: src.ProjectID, Title: src.Title, Text: src.Text, Export: src.Export,
		Thumbnail: src.Thumbnail, Post: src.Post, CreatedAt: now, UpdatedAt: now, CreatedBy: actor.ID,
		OwnerUserID: actor.OwnerUserID,
	}

	for i := range from {
		o := &from[i]
		t := &Target{
			ID:            m.env.NewID("tgt"),
			PublicationID: p.ID,
			ConnectionID:  o.ConnectionID,
			Network:       o.Network,
			Fields: cloneFields(
				o.Fields,
			),
			State:             core.TargetDraft,
			Zone:              o.Zone,
			LateWindowMinutes: o.LateWindowMinutes,
			Position:          i,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		if t.Zone == "" {
			t.Zone = "UTC"
		}

		note, err := m.applyTime(t, &TargetInput{At: in.At, Zone: in.Zone})
		if err != nil {
			return nil, err
		}

		if note != "" {
			p.Notes = append(p.Notes, note)
		}

		p.Targets = append(p.Targets, *t)
	}

	err = m.env.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Store().Insert(ctx, p); err != nil {
			return err
		}

		tx.Emit(view(*p))

		return nil
	})
	if err != nil {
		return nil, err
	}

	out := view(*p)

	return &out, nil
}

func cloneFields(f Fields) Fields {
	f.Tags = slices.Clone(f.Tags)

	if f.MadeForKids != nil {
		v := *f.MadeForKids
		f.MadeForKids = &v
	}

	return f
}

package service

import (
	"context"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// The day of the publishing calendar (SDD-020 section 30A, author decision D-146): the plan of a range, the note and
// theme of a day, quiet days, recurring slots, duplicate and the .ics export. The rules are in core/publish and in the
// manager of service/publish; these are the operations, with their callers and scopes.

// CalendarPlanRequest asks for a range of the calendar: From inclusive and To exclusive, each a date (the start of
// that day in Zone), a local time read in Zone, or an instant.
type CalendarPlanRequest struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Zone      string `json:"zone,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	// Publications asks for the publications of the range too, paged by Limit and Offset as PublicationList.
	Publications bool `json:"publications,omitempty"`
	Limit        int  `json:"limit,omitempty"`
	Offset       int  `json:"offset,omitempty"`
}

// CalendarPlanResult is the days, the slots and, when asked, the publications of a range.
type CalendarPlanResult = pub.Plan

// CalendarDay is a day of the calendar: a note, a theme and a quiet mark.
type CalendarDay = pub.Day

// CalendarDaySetRequest writes the note and the theme of a day; a member left out stays, an empty one clears it.
type CalendarDaySetRequest = pub.DayInput

// CalendarQuietSetRequest marks a day as quiet or clears the mark.
type CalendarQuietSetRequest struct {
	Date  string `json:"date"`
	Zone  string `json:"zone,omitempty"`
	Quiet bool   `json:"quiet"`
}

// CalendarSlot is a recurring slot.
type CalendarSlot = corepub.Rule

// CalendarSlotSaveRequest creates a slot (no id) or replaces one.
type CalendarSlotSaveRequest = pub.SlotInput

// CalendarSlotDeleteRequest names the slot to remove.
type CalendarSlotDeleteRequest struct {
	ID string `json:"id"`
}

// CalendarSlotDeleteResult says which slot was removed.
type CalendarSlotDeleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// PublicationDuplicateRequest copies a publication to another day or hour, as drafts.
type PublicationDuplicateRequest = pub.DuplicateInput

// CalendarExportRequest asks for the targets of a range as an iCalendar file.
type CalendarExportRequest struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Zone      string `json:"zone,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// CalendarExportResult is the file: its name, its media type and its bytes as text.
type CalendarExportResult struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
}

// ICSContentType is the media type of the export.
const ICSContentType = "text/calendar; charset=utf-8"

// calendarRange reads the two ends of a range, both required.
func calendarRange(from, to, zone string) (time.Time, time.Time, error) {
	var out [2]time.Time

	for i, f := range []struct{ raw, path string }{{from, "from"}, {to, "to"}} {
		if f.raw == "" {
			return time.Time{}, time.Time{}, validationAt(f.path, "is required: a date such as 2026-12-01.")
		}

		v, err := corepub.ParseBound(f.raw, zone)
		if err != nil {
			return time.Time{}, time.Time{}, validationAt(
				f.path, "must be a date such as 2026-12-01, or a time such as 2026-12-01T00:00:00Z.")
		}

		out[i] = v
	}

	if !out[1].After(out[0]) {
		return time.Time{}, time.Time{}, validationAt("to", "must be after from.")
	}

	return out[0], out[1], nil
}

// calendarFilter selects the publications a caller may see, as PublicationList does: the project of a key, the owner
// of a user.
func calendarFilter(c Caller, projectID string) pub.Filter {
	f := pub.Filter{ProjectID: projectID}

	if c.ProjectID != "" {
		f.ProjectID = c.ProjectID
	}

	if c.OwnerUserID != "" && !hasScope(c, ScopeAdmin) {
		f.OwnerUserID = c.OwnerUserID
	}

	return f
}

// workspaceCalendar refuses a key limited to one project: the days and the slots belong to the whole calendar, not to
// a project, so such a key reads its publications with publication:list and the export, and nothing more.
func workspaceCalendar(c Caller) error {
	if c.ProjectID == "" {
		return nil
	}

	return newError(
		CodeForbidden,
		"the key is limited to one project and cannot read or write the days of the whole calendar.",
	).
		withHint("use a key without a project limit, or read the publications of the project with publication:list")
}

// CalendarPlan reads a range of the calendar.
func (s *svc) CalendarPlan(ctx context.Context, req CalendarPlanRequest) (*CalendarPlanResult, error) {
	c, err := s.authorize(ctx, "CalendarPlan", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := workspaceCalendar(c); err != nil {
		return nil, err
	}

	from, to, err := calendarRange(req.From, req.To, req.Zone)
	if err != nil {
		return nil, err
	}

	f := calendarFilter(c, req.ProjectID)
	f.Limit, f.Offset = req.Limit, req.Offset

	plan, perr := s.pubs.Plan(ctx, pub.PlanInput{
		From: from, To: to, Zone: req.Zone, Filter: f, Publications: req.Publications,
	})
	if perr != nil {
		return nil, callerError(c, publicationError(perr))
	}

	return plan, nil
}

// CalendarDaySet writes the note and the theme of a day.
func (s *svc) CalendarDaySet(ctx context.Context, req CalendarDaySetRequest) (*CalendarDay, error) {
	c, err := s.authorize(ctx, "CalendarDaySet", "")
	if err != nil {
		return nil, err
	}

	if err := workspaceCalendar(c); err != nil {
		return nil, err
	}

	d, derr := s.pubs.DaySet(ctx, req)
	if derr != nil {
		return nil, callerError(c, publicationError(derr))
	}

	return d, nil
}

// CalendarQuietSet marks a day as quiet or clears the mark: a person in the app only (D-146).
func (s *svc) CalendarQuietSet(ctx context.Context, req CalendarQuietSetRequest) (*CalendarDay, error) {
	c, err := s.authorize(ctx, "CalendarQuietSet", "")
	if err != nil {
		return nil, err
	}

	d, qerr := s.pubs.QuietSet(ctx, req.Date, req.Zone, req.Quiet)
	if qerr != nil {
		return nil, callerError(c, publicationError(qerr))
	}

	return d, nil
}

// CalendarSlotSave creates or replaces a recurring slot.
func (s *svc) CalendarSlotSave(ctx context.Context, req CalendarSlotSaveRequest) (*CalendarSlot, error) {
	c, err := s.authorize(ctx, "CalendarSlotSave", "")
	if err != nil {
		return nil, err
	}

	if err := workspaceCalendar(c); err != nil {
		return nil, err
	}

	r, serr := s.pubs.SlotSave(ctx, req)
	if serr != nil {
		return nil, callerError(c, publicationError(serr))
	}

	return r, nil
}

// CalendarSlotDelete removes a recurring slot.
func (s *svc) CalendarSlotDelete(
	ctx context.Context,
	req CalendarSlotDeleteRequest,
) (*CalendarSlotDeleteResult, error) {
	c, err := s.authorize(ctx, "CalendarSlotDelete", "")
	if err != nil {
		return nil, err
	}

	if err := workspaceCalendar(c); err != nil {
		return nil, err
	}

	if derr := s.pubs.SlotDelete(ctx, req.ID); derr != nil {
		return nil, callerError(c, publicationError(derr))
	}

	return &CalendarSlotDeleteResult{ID: req.ID, Deleted: true}, nil
}

// PublicationDuplicate copies a publication to another day or hour.
func (s *svc) PublicationDuplicate(ctx context.Context, req PublicationDuplicateRequest) (*Publication, error) {
	c, _, err := s.publicationOf(ctx, "PublicationDuplicate", req.ID)
	if err != nil {
		return nil, err
	}

	p, derr := s.pubs.Duplicate(ctx, req)
	if derr != nil {
		return nil, callerError(c, publicationError(derr))
	}

	return p, nil
}

// CalendarExport writes the targets of a range as an iCalendar file.
func (s *svc) CalendarExport(ctx context.Context, req CalendarExportRequest) (*CalendarExportResult, error) {
	c, err := s.authorize(ctx, "CalendarExport", req.ProjectID)
	if err != nil {
		return nil, err
	}

	from, to, err := calendarRange(req.From, req.To, req.Zone)
	if err != nil {
		return nil, err
	}

	body, eerr := s.pubs.Export(ctx, from, to, calendarFilter(c, req.ProjectID))
	if eerr != nil {
		return nil, callerError(c, publicationError(eerr))
	}

	return &CalendarExportResult{
		Filename:    "stuarty-calendar-" + from.UTC().Format("20060102") + ".ics",
		ContentType: ICSContentType,
		Body:        body,
	}, nil
}

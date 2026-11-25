package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The day of the publishing calendar (SDD-020 section 30A): handlers that parse, call one Service method and format.
// The quiet-day rule, who may mark a day and what the file holds are the Service's.

type calendarPlanIn struct {
	From         string `json:"from"                   validate:"required" doc:"The start of the range: a date such as 2026-12-01 (the start of that day in zone), a local time read in zone, or an instant."`
	To           string `json:"to"                     validate:"required" doc:"The end of the range, exclusive, in the same forms as from."`
	Zone         string `json:"zone,omitempty"                             doc:"The IANA zone dates and local times are read in; empty is UTC."`
	ProjectID    string `json:"project_id,omitempty"                       doc:"Only the publications of one project."`
	Publications bool   `json:"publications,omitempty"                     doc:"Also return the publications of the range, paged by limit and offset."`
	Limit        int    `json:"limit,omitempty"                            doc:"The page size of the publications, 1 to 500; the default is 100."`
	Offset       int    `json:"offset,omitempty"                           doc:"How many publications to skip."`
}

type calendarDayIn struct {
	Date  string  `json:"date"            validate:"required" doc:"The local date, 2026-12-25."`
	Zone  string  `json:"zone,omitempty"                      doc:"The IANA zone the day is planned in; empty keeps the one it has, or UTC."`
	Note  *string `json:"note,omitempty"                      doc:"The note of the day, at most 2000 characters; leave out to keep it, empty to clear it."`
	Theme *string `json:"theme,omitempty"                     doc:"The theme: blue, green, amber, red, teal or gray; leave out to keep it, empty to clear it."`
}

type calendarQuietIn struct {
	Date  string `json:"date"           validate:"required" doc:"The local date, 2026-12-25."`
	Zone  string `json:"zone,omitempty"                     doc:"The IANA zone the day is planned in; empty keeps the one it has, or UTC."`
	Quiet bool   `json:"quiet"                              doc:"true marks the day as quiet (no posts); false clears the mark."`
}

type calendarSlotIn struct {
	ID           string `json:"id,omitempty"            doc:"The slot to replace, slt_ and 10 characters; leave out to make a new one."`
	Network      string `json:"network"                 doc:"The network the slot reminds you of, for example instagram."               validate:"required"`
	Weekday      string `json:"weekday"                 doc:"mon, tue, wed, thu, fri, sat or sun."                                      validate:"required"`
	Time         string `json:"time"                    doc:"The local time, 24 hours, such as 18:00."                                  validate:"required"`
	Zone         string `json:"zone,omitempty"          doc:"The IANA zone of the time; empty is UTC."`
	ConnectionID string `json:"connection_id,omitempty" doc:"Only this account of the network; empty is any."`
}

type calendarSlotRefIn struct {
	ID string `json:"id" validate:"required" doc:"A slot id, slt_ and 10 characters."`
}

type publicationDuplicateIn struct {
	ID       string `json:"id"                  validate:"required" doc:"A publication id, pub_ and 10 characters."`
	TargetID string `json:"target_id,omitempty"                     doc:"The one target to copy; leave out to copy every target that is not cancelled."`
	At       string `json:"at"                  validate:"required" doc:"The new time: a date and time such as 2026-12-02T18:00 read in zone, or an instant with an offset or Z."`
	Zone     string `json:"zone,omitempty"                          doc:"The IANA zone the new time is read in; the default is the zone of each target."`
}

type calendarExportIn struct {
	From      string `json:"from"                 validate:"required" doc:"The start of the range: a date such as 2026-12-01, a local time read in zone, or an instant."`
	To        string `json:"to"                   validate:"required" doc:"The end of the range, exclusive."`
	Zone      string `json:"zone,omitempty"                           doc:"The IANA zone dates and local times are read in; empty is UTC."`
	ProjectID string `json:"project_id,omitempty"                     doc:"Only the publications of one project."`
}

func (h *apiHandler) calendarPlan(c *web.Ctx, in calendarPlanIn) (service.CalendarPlanResult, error) {
	res, err := h.Service.CalendarPlan(c.Context(), service.CalendarPlanRequest(in))

	return deref(res), err
}

func (h *apiHandler) calendarDaySet(c *web.Ctx, in calendarDayIn) (service.CalendarDay, error) {
	res, err := h.Service.CalendarDaySet(c.Context(), service.CalendarDaySetRequest(in))

	return deref(res), err
}

func (h *apiHandler) calendarQuietSet(c *web.Ctx, in calendarQuietIn) (service.CalendarDay, error) {
	res, err := h.Service.CalendarQuietSet(c.Context(), service.CalendarQuietSetRequest(in))

	return deref(res), err
}

func (h *apiHandler) calendarSlotSave(c *web.Ctx, in calendarSlotIn) (service.CalendarSlot, error) {
	res, err := h.Service.CalendarSlotSave(c.Context(), service.CalendarSlotSaveRequest(in))

	return deref(res), err
}

func (h *apiHandler) calendarSlotDelete(c *web.Ctx, in calendarSlotRefIn) (service.CalendarSlotDeleteResult, error) {
	res, err := h.Service.CalendarSlotDelete(c.Context(), service.CalendarSlotDeleteRequest(in))

	return deref(res), err
}

func (h *apiHandler) publicationsDuplicate(c *web.Ctx, in publicationDuplicateIn) (service.Publication, error) {
	res, err := h.Service.PublicationDuplicate(c.Context(), service.PublicationDuplicateRequest(in))

	return deref(res), err
}

// calendarExport is GET /calendar/export: the .ics file, an attachment. The body is the one the Service wrote, with
// the media type it named; nothing here adds to it.
func (h *apiHandler) calendarExport(c *web.Ctx, in calendarExportIn) (noIn, error) {
	res, err := h.Service.CalendarExport(c.Context(), service.CalendarExportRequest(in))
	if err != nil {
		return noIn{}, err
	}

	hd := c.Writer().Header()
	hd.Set("Content-Disposition", `attachment; filename="`+res.Filename+`"`)
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", "private, no-store")

	return noIn{}, c.Blob(200, res.ContentType, []byte(res.Body))
}

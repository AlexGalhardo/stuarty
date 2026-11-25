package remote

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
)

// PublicationDuplicate implements service.Service.
func (c *Client) PublicationDuplicate(
	ctx context.Context, req service.PublicationDuplicateRequest,
) (*service.Publication, error) {
	body := struct {
		TargetID string `json:"target_id,omitempty"`
		At       string `json:"at"`
		Zone     string `json:"zone,omitempty"`
	}{req.TargetID, req.At, req.Zone}

	return post[service.Publication](ctx, c, publicationPath(req.ID, "/duplicate"), body)
}

// CalendarPlan implements service.Service.
func (c *Client) CalendarPlan(
	ctx context.Context,
	req service.CalendarPlanRequest,
) (*service.CalendarPlanResult, error) {
	q := url.Values{}
	setIf(q, "from", req.From)
	setIf(q, "to", req.To)
	setIf(q, "zone", req.Zone)
	setIf(q, "project_id", req.ProjectID)
	setBool(q, "publications", req.Publications)

	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}

	if req.Offset > 0 {
		q.Set("offset", strconv.Itoa(req.Offset))
	}

	return get[service.CalendarPlanResult](ctx, c, "/calendar/plan", q)
}

// CalendarDaySet implements service.Service.
func (c *Client) CalendarDaySet(ctx context.Context, req service.CalendarDaySetRequest) (*service.CalendarDay, error) {
	body := struct {
		Zone  string  `json:"zone,omitempty"`
		Note  *string `json:"note,omitempty"`
		Theme *string `json:"theme,omitempty"`
	}{req.Zone, req.Note, req.Theme}

	return put[service.CalendarDay](ctx, c, "/calendar/days/"+segment(req.Date), body)
}

// CalendarQuietSet implements service.Service.
func (c *Client) CalendarQuietSet(
	ctx context.Context,
	req service.CalendarQuietSetRequest,
) (*service.CalendarDay, error) {
	body := struct {
		Zone  string `json:"zone,omitempty"`
		Quiet bool   `json:"quiet"`
	}{req.Zone, req.Quiet}

	return put[service.CalendarDay](ctx, c, "/calendar/quiet/"+segment(req.Date), body)
}

// CalendarSlotSave implements service.Service.
func (c *Client) CalendarSlotSave(
	ctx context.Context,
	req service.CalendarSlotSaveRequest,
) (*service.CalendarSlot, error) {
	return put[service.CalendarSlot](ctx, c, "/calendar/slots", req)
}

// CalendarSlotDelete implements service.Service.
func (c *Client) CalendarSlotDelete(
	ctx context.Context, req service.CalendarSlotDeleteRequest,
) (*service.CalendarSlotDeleteResult, error) {
	out := new(service.CalendarSlotDeleteResult)
	if err := c.do(ctx, http.MethodDelete, "/calendar/slots/"+segment(req.ID), nil, nil, out); err != nil {
		return nil, err
	}

	return out, nil
}

// CalendarExport implements service.Service.
func (c *Client) CalendarExport(
	ctx context.Context,
	req service.CalendarExportRequest,
) (*service.CalendarExportResult, error) {
	q := url.Values{}
	setIf(q, "from", req.From)
	setIf(q, "to", req.To)
	setIf(q, "zone", req.Zone)
	setIf(q, "project_id", req.ProjectID)

	raw := new(rawBody)
	if err := c.do(ctx, http.MethodGet, "/calendar/export", q, nil, raw); err != nil {
		return nil, err
	}

	_, params, _ := mime.ParseMediaType(raw.Disposition)

	return &service.CalendarExportResult{
		Filename: params["filename"], ContentType: raw.ContentType, Body: string(raw.Bytes),
	}, nil
}

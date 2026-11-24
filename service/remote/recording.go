package remote

import (
	"context"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
)

// RecordingPause implements service.Service.
func (c *Client) RecordingPause(ctx context.Context, req service.RecordingRef) (*service.Recording, error) {
	return post[service.Recording](ctx, c, "/recordings/"+segment(req.ID)+"/pause", struct{}{})
}

// RecordingResume implements service.Service.
func (c *Client) RecordingResume(ctx context.Context, req service.RecordingRef) (*service.Recording, error) {
	return post[service.Recording](ctx, c, "/recordings/"+segment(req.ID)+"/resume", struct{}{})
}

// RecordingEvents implements service.Service.
func (c *Client) RecordingEvents(
	ctx context.Context, req service.RecordingEventsRequest,
) (*service.RecordingEventsResult, error) {
	q := url.Values{}
	if req.FromMS != 0 {
		q.Set("from_ms", strconv.FormatInt(req.FromMS, 10))
	}

	if req.Kind != "" {
		q.Set("kind", req.Kind)
	}

	if req.Limit != 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}

	return get[service.RecordingEventsResult](ctx, c, "/recordings/"+segment(req.ID)+"/events", q)
}

// RecordingSources implements service.Service.
func (c *Client) RecordingSources(
	ctx context.Context, _ service.RecordingSourcesRequest,
) (*service.RecordingSourcesResult, error) {
	return get[service.RecordingSourcesResult](ctx, c, "/recording/sources", nil)
}

// RecordingStart implements service.Service.
func (c *Client) RecordingStart(ctx context.Context, req service.RecordingStartRequest) (*service.Recording, error) {
	return post[service.Recording](ctx, c, "/recordings", req)
}

// RecordingStop implements service.Service.
func (c *Client) RecordingStop(ctx context.Context, req service.RecordingRef) (*service.Recording, error) {
	return post[service.Recording](ctx, c, "/recordings/"+segment(req.ID)+"/stop", struct{}{})
}

// RecordingCancel implements service.Service.
func (c *Client) RecordingCancel(ctx context.Context, req service.RecordingRef) (*service.Recording, error) {
	return post[service.Recording](ctx, c, "/recordings/"+segment(req.ID)+"/cancel", struct{}{})
}

// RecordingGet implements service.Service.
func (c *Client) RecordingGet(ctx context.Context, req service.RecordingRef) (*service.Recording, error) {
	return get[service.Recording](ctx, c, "/recordings/"+segment(req.ID), nil)
}

// RecordingList implements service.Service.
func (c *Client) RecordingList(
	ctx context.Context, req service.RecordingListRequest,
) (*service.RecordingListResult, error) {
	return get[service.RecordingListResult](ctx, c, "/projects/"+segment(req.ProjectID)+"/recordings", nil)
}

// RecordingConfirm implements service.Service.
func (c *Client) RecordingConfirm(ctx context.Context, req service.RecordingRef) (*service.Recording, error) {
	return post[service.Recording](ctx, c, "/recordings/"+segment(req.ID)+"/confirm", struct{}{})
}

// RecordingDismiss implements service.Service.
func (c *Client) RecordingDismiss(ctx context.Context, req service.RecordingRef) (*service.Recording, error) {
	return post[service.Recording](ctx, c, "/recordings/"+segment(req.ID)+"/dismiss", struct{}{})
}

// RecordingPlace implements service.Service.
func (c *Client) RecordingPlace(ctx context.Context, req service.RecordingPlaceRequest) (*service.EditResult, error) {
	body := struct {
		TimelineID string `json:"timeline_id,omitempty"`
	}{req.TimelineID}

	return post[service.EditResult](ctx, c, "/recordings/"+segment(req.ID)+"/place", body)
}

package remote

import (
	"context"
	"net/http"
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
	if req.ProjectID == "" {
		return get[service.RecordingListResult](ctx, c, "/recordings", nil)
	}

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

// RecordingUploadBegin implements service.Service.
func (c *Client) RecordingUploadBegin(
	ctx context.Context, req service.RecordingUploadBeginRequest,
) (*service.Recording, error) {
	return post[service.Recording](ctx, c, "/recordings/upload", req)
}

// RecordingUploadChunk implements service.Service: the bytes of the chunk are the body, the rest is the path and the
// query.
func (c *Client) RecordingUploadChunk(
	ctx context.Context, req service.RecordingUploadChunkRequest,
) (*service.RecordingUploadChunkResult, error) {
	q := url.Values{}
	q.Set("sha256", req.SHA256)

	if req.ElapsedMS > 0 {
		q.Set("elapsed_ms", strconv.FormatInt(req.ElapsedMS, 10))
	}

	out := new(service.RecordingUploadChunkResult)
	path := "/recordings/" + segment(req.ID) + "/upload/" + segment(req.Track) + "/" + strconv.Itoa(req.Index)

	if err := c.do(ctx, http.MethodPut, path, q, fileBody{r: req.Body, size: req.Size}, out); err != nil {
		return nil, err
	}

	return out, nil
}

// RecordingUploadFinish implements service.Service.
func (c *Client) RecordingUploadFinish(
	ctx context.Context, req service.RecordingUploadFinishRequest,
) (*service.Recording, error) {
	body := struct {
		Tracks    []service.RecordingUploadCount `json:"tracks,omitempty"`
		ElapsedMS int64                          `json:"elapsed_ms,omitempty"`
	}{req.Tracks, req.ElapsedMS}

	return post[service.Recording](ctx, c, "/recordings/"+segment(req.ID)+"/finish", body)
}

// RecordingPrepare implements service.Service.
func (c *Client) RecordingPrepare(
	ctx context.Context, req service.RecordingPrepareRequest,
) (*service.Recording, error) {
	return post[service.Recording](ctx, c, "/recordings/prepare", req)
}

// RecordingMark implements service.Service.
func (c *Client) RecordingMark(ctx context.Context, req service.RecordingMarkRequest) (*service.Recording, error) {
	body := struct {
		Kind  string `json:"kind"`
		AtMS  int64  `json:"at_ms"`
		EndMS int64  `json:"end_ms,omitempty"`
		Note  string `json:"note,omitempty"`
	}{req.Kind, req.AtMS, req.EndMS, req.Note}

	return post[service.Recording](ctx, c, "/recordings/"+segment(req.ID)+"/marks", body)
}

// RecordingProposeCuts implements service.Service.
func (c *Client) RecordingProposeCuts(
	ctx context.Context, req service.RecordingProposeCutsRequest,
) (*service.Recording, error) {
	body := struct {
		Ranges []service.RecordingCutRange `json:"ranges"`
	}{req.Ranges}

	return post[service.Recording](ctx, c, "/recordings/"+segment(req.ID)+"/proposals", body)
}

// RecordingProposalDecide implements service.Service.
func (c *Client) RecordingProposalDecide(
	ctx context.Context, req service.RecordingProposalDecideRequest,
) (*service.RecordingProposalDecideResult, error) {
	body := struct {
		Accept     []bool `json:"accept"`
		TimelineID string `json:"timeline_id,omitempty"`
	}{req.Accept, req.TimelineID}

	return post[service.RecordingProposalDecideResult](
		ctx, c, "/recordings/"+segment(req.ID)+"/proposals/"+segment(req.ProposalID)+"/decide", body)
}

package remote

import (
	"context"
	"net/url"
	"strconv"

	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

func publicationPath(id, suffix string) string {
	return "/publications/" + segment(id) + suffix
}

// PublicationCreate implements service.Service.
func (c *Client) PublicationCreate(
	ctx context.Context, req service.PublicationCreateRequest,
) (*service.Publication, error) {
	return post[service.Publication](ctx, c, "/publications", req)
}

// PublicationGet implements service.Service.
func (c *Client) PublicationGet(ctx context.Context, req service.PublicationGetRequest) (*service.Publication, error) {
	return get[service.Publication](ctx, c, publicationPath(req.ID, ""), nil)
}

// PublicationList implements service.Service.
func (c *Client) PublicationList(
	ctx context.Context, req service.PublicationListRequest,
) (*service.PublicationListResult, error) {
	q := url.Values{}

	for k, v := range map[string]string{
		"project_id": req.ProjectID, "state": req.State, "network": req.Network, "from": req.From, "to": req.To,
		"zone": req.Zone,
	} {
		if v != "" {
			q.Set(k, v)
		}
	}

	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}

	if req.Offset > 0 {
		q.Set("offset", strconv.Itoa(req.Offset))
	}

	return get[service.PublicationListResult](ctx, c, "/publications", q)
}

// PublicationUpdate implements service.Service.
func (c *Client) PublicationUpdate(
	ctx context.Context, req service.PublicationUpdateRequest,
) (*service.Publication, error) {
	body := struct {
		Title     *string           `json:"title,omitempty"`
		Text      *string           `json:"text,omitempty"`
		Export    *string           `json:"export,omitempty"`
		Thumbnail *string           `json:"thumbnail,omitempty"`
		Targets   []pub.TargetInput `json:"targets,omitempty"`
	}{req.Title, req.Text, req.Export, req.Thumbnail, req.Targets}

	return put[service.Publication](ctx, c, publicationPath(req.ID, ""), body)
}

// PublicationSchedule implements service.Service.
func (c *Client) PublicationSchedule(
	ctx context.Context, req service.PublicationScheduleRequest,
) (*service.Publication, error) {
	body := struct {
		TargetID          string `json:"target_id,omitempty"`
		At                string `json:"at,omitempty"`
		Zone              string `json:"zone,omitempty"`
		LateWindowMinutes *int   `json:"late_window_minutes,omitempty"`
	}{req.TargetID, req.At, req.Zone, req.LateWindowMinutes}

	return post[service.Publication](ctx, c, publicationPath(req.ID, "/schedule"), body)
}

type targetBody struct {
	TargetID string `json:"target_id,omitempty"`
}

// PublicationSendNow implements service.Service.
func (c *Client) PublicationSendNow(
	ctx context.Context, req service.PublicationSendNowRequest,
) (*service.Publication, error) {
	return post[service.Publication](ctx, c, publicationPath(req.ID, "/send-now"), targetBody{req.TargetID})
}

// PublicationRetry implements service.Service.
func (c *Client) PublicationRetry(
	ctx context.Context, req service.PublicationRetryRequest,
) (*service.Publication, error) {
	return post[service.Publication](ctx, c, publicationPath(req.ID, "/retry"), targetBody{req.TargetID})
}

// PublicationCancel implements service.Service.
func (c *Client) PublicationCancel(
	ctx context.Context, req service.PublicationCancelRequest,
) (*service.Publication, error) {
	return post[service.Publication](ctx, c, publicationPath(req.ID, "/cancel"), targetBody{req.TargetID})
}

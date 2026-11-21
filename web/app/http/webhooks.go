package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type webhookRefIn struct {
	ID string `json:"id" validate:"required" doc:"A webhook id, whk_ and 10 characters."`
}

type webhookCreateIn struct {
	Name    string   `json:"name"              validate:"required" doc:"1 to 60 characters, shown to the person."`
	URL     string   `json:"url"               validate:"required" doc:"Where the events are sent: https, or http for localhost or a loopback address. A private, link-local or metadata address is refused."`
	Events  []string `json:"events"            validate:"required" doc:"One to five of publication.published, publication.failed, export.ready, approval.requested, job.failed; GET /webhook-events says what each carries."`
	Enabled *bool    `json:"enabled,omitempty"                     doc:"Whether the webhook receives events; default true."`
}

type webhookUpdateIn struct {
	ID      string    `json:"id"                validate:"required" doc:"A webhook id."`
	Name    *string   `json:"name,omitempty"                        doc:"1 to 60 characters."`
	URL     *string   `json:"url,omitempty"                         doc:"The new address; the same rules as at creation."`
	Events  *[]string `json:"events,omitempty"                      doc:"The new list of events."`
	Enabled *bool     `json:"enabled,omitempty"                     doc:"Turn the webhook on or off."`
}

type webhookDeliveriesIn struct {
	ID     string `json:"id"               validate:"required" doc:"A webhook id."`
	Limit  int    `json:"limit,omitempty"                      doc:"The most deliveries in the page, 1 to 100; default 50."`
	Offset int    `json:"offset,omitempty"                     doc:"How many deliveries to skip."`
}

type webhookRedeliverIn struct {
	ID         string `json:"id"          validate:"required" doc:"A webhook id."`
	DeliveryID string `json:"delivery_id" validate:"required" doc:"A delivery id, wdl_ and 10 characters."`
}

func (h *apiHandler) webhookEvents(c *web.Ctx, _ noIn) (service.WebhookEventListResult, error) {
	res, err := h.Service.WebhookEventList(c.Context(), service.WebhookEventListRequest{})

	return deref(res), err
}

func (h *apiHandler) webhooksList(c *web.Ctx, _ noIn) (service.WebhookListResult, error) {
	res, err := h.Service.WebhookList(c.Context(), service.WebhookListRequest{})

	return deref(res), err
}

func (h *apiHandler) webhooksGet(c *web.Ctx, in webhookRefIn) (service.Webhook, error) {
	res, err := h.Service.WebhookGet(c.Context(), service.WebhookRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) webhooksCreate(c *web.Ctx, in webhookCreateIn) (service.WebhookSecretResult, error) {
	res, err := h.Service.WebhookCreate(c.Context(), service.WebhookCreateRequest{
		Name: in.Name, URL: in.URL, Events: in.Events, Enabled: in.Enabled,
	})

	// The secret is in this answer once: nothing may keep it.
	c.SetHeader("Cache-Control", "no-store")

	return deref(res), err
}

func (h *apiHandler) webhooksUpdate(c *web.Ctx, in webhookUpdateIn) (service.Webhook, error) {
	res, err := h.Service.WebhookUpdate(c.Context(), service.WebhookUpdateRequest{
		ID: in.ID, Name: in.Name, URL: in.URL, Events: in.Events, Enabled: in.Enabled,
	})

	return deref(res), err
}

func (h *apiHandler) webhooksRemove(c *web.Ctx, in webhookRefIn) (service.Webhook, error) {
	res, err := h.Service.WebhookRemove(c.Context(), service.WebhookRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) webhooksRotate(c *web.Ctx, in webhookRefIn) (service.WebhookSecretResult, error) {
	res, err := h.Service.WebhookRotateSecret(c.Context(), service.WebhookRef{ID: in.ID})

	c.SetHeader("Cache-Control", "no-store")

	return deref(res), err
}

func (h *apiHandler) webhooksTest(c *web.Ctx, in webhookRefIn) (service.WebhookDelivery, error) {
	res, err := h.Service.WebhookTest(c.Context(), service.WebhookRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) webhooksDeliveries(c *web.Ctx, in webhookDeliveriesIn) (service.WebhookDeliveriesResult, error) {
	res, err := h.Service.WebhookDeliveryList(c.Context(), service.WebhookDeliveriesRequest{
		ID: in.ID, Limit: in.Limit, Offset: in.Offset,
	})

	return deref(res), err
}

func (h *apiHandler) webhooksRedeliver(c *web.Ctx, in webhookRedeliverIn) (service.WebhookDelivery, error) {
	res, err := h.Service.WebhookRedeliver(c.Context(), service.WebhookRedeliverRequest{
		ID: in.ID, DeliveryID: in.DeliveryID,
	})

	return deref(res), err
}

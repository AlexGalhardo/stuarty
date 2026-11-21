package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

func (s stub) WebhookEventList(
	context.Context,
	service.WebhookEventListRequest,
) (*service.WebhookEventListResult, error) {
	return reply[service.WebhookEventListResult](s.err)
}

func (s stub) WebhookList(context.Context, service.WebhookListRequest) (*service.WebhookListResult, error) {
	return reply[service.WebhookListResult](s.err)
}

func (s stub) WebhookGet(context.Context, service.WebhookRef) (*service.Webhook, error) {
	return reply[service.Webhook](s.err)
}

func (s stub) WebhookCreate(context.Context, service.WebhookCreateRequest) (*service.WebhookSecretResult, error) {
	return reply[service.WebhookSecretResult](s.err)
}

func (s stub) WebhookUpdate(context.Context, service.WebhookUpdateRequest) (*service.Webhook, error) {
	return reply[service.Webhook](s.err)
}

func (s stub) WebhookRemove(context.Context, service.WebhookRef) (*service.Webhook, error) {
	return reply[service.Webhook](s.err)
}

func (s stub) WebhookRotateSecret(context.Context, service.WebhookRef) (*service.WebhookSecretResult, error) {
	return reply[service.WebhookSecretResult](s.err)
}

func (s stub) WebhookTest(context.Context, service.WebhookRef) (*service.WebhookDelivery, error) {
	return reply[service.WebhookDelivery](s.err)
}

func (s stub) WebhookDeliveryList(
	context.Context,
	service.WebhookDeliveriesRequest,
) (*service.WebhookDeliveriesResult, error) {
	return reply[service.WebhookDeliveriesResult](s.err)
}

func (s stub) WebhookRedeliver(context.Context, service.WebhookRedeliverRequest) (*service.WebhookDelivery, error) {
	return reply[service.WebhookDelivery](s.err)
}

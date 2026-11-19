package cli

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
)

func (s stub) ProviderList(context.Context, service.ProviderListRequest) (*service.ProviderListResult, error) {
	return reply[service.ProviderListResult](s.err)
}

func (s stub) ProviderModels(context.Context, service.ProviderModelsRequest) (*service.ProviderModelsResult, error) {
	return reply[service.ProviderModelsResult](s.err)
}

func (s stub) ProviderPriceSet(
	context.Context,
	service.ProviderPriceSetRequest,
) (*service.ProviderPricesResult, error) {
	return reply[service.ProviderPricesResult](s.err)
}

func (s stub) TextGenerate(context.Context, service.TextGenerateRequest) (*service.Job, error) {
	return reply[service.Job](s.err)
}

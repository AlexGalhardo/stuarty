package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type providerModelsIn struct {
	ID      string `json:"id"                validate:"required" doc:"The id of a connection to an AI provider, con_ and 10 characters."`
	Refresh bool   `json:"refresh,omitempty"                     doc:"true reads the list from the provider again; the default serves a list at most ten minutes old."`
}

type providerPriceIn struct {
	Provider            string `json:"provider"                         validate:"required" doc:"The id of the provider, for example openai."`
	Model               string `json:"model"                            validate:"required" doc:"The model id, as provider:models lists it."`
	Currency            string `json:"currency,omitempty"                                   doc:"Three letters, for example USD; needed to set a price."`
	InputMicrosPerMTok  *int64 `json:"input_micros_per_mtok,omitempty"                      doc:"The price of a million input tokens, in millionths of the currency unit. Leave both prices out to remove the entry."`
	OutputMicrosPerMTok *int64 `json:"output_micros_per_mtok,omitempty"                     doc:"The price of a million output tokens, in millionths of the currency unit."`
}

type textGenerateIn struct {
	ProjectID    string   `json:"project_id"            validate:"required" doc:"The project the generation belongs to, prj_ and 10 characters."`
	ConnectionID string   `json:"connection_id"         validate:"required" doc:"The connection to the provider that writes the text."`
	Model        string   `json:"model"                 validate:"required" doc:"The model id, as provider:models lists it."`
	Prompt       string   `json:"prompt"                validate:"required" doc:"What to write, 1 to 100000 characters."`
	System       string   `json:"system,omitempty"                          doc:"Instructions for the model, at most 20000 characters."`
	MaxTokens    int      `json:"max_tokens,omitempty"                      doc:"The most tokens the model may write; the default is 1024. The estimate counts all of them."`
	Temperature  *float64 `json:"temperature,omitempty"                     doc:"0 to 2; leave out for the default of the provider."`
	WaitMS       int      `json:"wait_ms,omitempty"                         doc:"Wait up to this many milliseconds for the job to finish."`
	DryRun       bool     `json:"dry_run,omitempty"                         doc:"true answers the job it would create, with its estimate, and writes and sends nothing."`
}

func (h *apiHandler) providersList(c *web.Ctx, _ noIn) (service.ProviderListResult, error) {
	res, err := h.Service.ProviderList(c.Context(), service.ProviderListRequest{})

	return deref(res), err
}

func (h *apiHandler) providersModels(c *web.Ctx, in providerModelsIn) (service.ProviderModelsResult, error) {
	res, err := h.Service.ProviderModels(
		c.Context(),
		service.ProviderModelsRequest{ConnectionID: in.ID, Refresh: in.Refresh},
	)

	return deref(res), err
}

func (h *apiHandler) providersPriceSet(c *web.Ctx, in providerPriceIn) (service.ProviderPricesResult, error) {
	res, err := h.Service.ProviderPriceSet(c.Context(), service.ProviderPriceSetRequest(in))

	return deref(res), err
}

func (h *apiHandler) textGenerate(c *web.Ctx, in textGenerateIn) (service.Job, error) {
	res, err := h.Service.TextGenerate(c.Context(), service.TextGenerateRequest{
		ProjectID: in.ProjectID, ConnectionID: in.ConnectionID, WaitMS: in.WaitMS, DryRun: in.DryRun,
		Model: in.Model, Prompt: in.Prompt, System: in.System, MaxTokens: in.MaxTokens, Temperature: in.Temperature,
	})

	return deref(res), err
}

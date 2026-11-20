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
	Unit                string `json:"unit,omitempty"                                       doc:"Empty for a price of tokens, image for the price of one image, mchar for the price of a million characters of speech, second for the price of one second of video."`
	UnitMicros          *int64 `json:"unit_micros,omitempty"                                doc:"With a unit of image, mchar or second: the price of one image, of a million characters or of one second of video, in millionths of the currency unit; the token prices stay out."`
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

type imageGenerateIn struct {
	ProjectID    string `json:"project_id"        validate:"required" doc:"The project the images belong to, prj_ and 10 characters."`
	ConnectionID string `json:"connection_id"     validate:"required" doc:"The connection to the provider that makes the images."`
	Model        string `json:"model"             validate:"required" doc:"The model id, as provider:models lists it."`
	Prompt       string `json:"prompt"            validate:"required" doc:"What to draw, 1 to 4000 characters."`
	Size         string `json:"size,omitempty"                        doc:"Width and height in pixels joined by x; the default is 1024x1024. The model may make only some sizes."`
	Count        int    `json:"count,omitempty"                       doc:"How many images, 1 to 10; the default is 1. A model may make fewer at a time. The estimate counts all of them."`
	WaitMS       int    `json:"wait_ms,omitempty"                     doc:"Wait up to this many milliseconds for the job to finish."`
	DryRun       bool   `json:"dry_run,omitempty"                     doc:"true answers the job it would create, with its estimate, and writes and sends nothing."`
}

type voiceGenerateIn struct {
	ProjectID    string `json:"project_id"         validate:"required" doc:"The project the speech belongs to, prj_ and 10 characters."`
	ConnectionID string `json:"connection_id"      validate:"required" doc:"The connection to the provider that reads the text out."`
	Model        string `json:"model"              validate:"required" doc:"The model id, as provider:models lists it."`
	Text         string `json:"text"               validate:"required" doc:"What to read out, 1 to 4096 characters (the provider may take fewer)."`
	Voice        string `json:"voice"              validate:"required" doc:"The voice id, as voice:list names it."`
	Language     string `json:"language,omitempty"                     doc:"A language tag such as pt-BR; a provider that detects the language from the text ignores it."`
	Format       string `json:"format,omitempty"                       doc:"mp3 (the default), wav, flac, opus or aac."`
	WaitMS       int    `json:"wait_ms,omitempty"                      doc:"Wait up to this many milliseconds for the job to finish."`
	DryRun       bool   `json:"dry_run,omitempty"                      doc:"true answers the job it would create, with its estimate, and writes and sends nothing."`
}

type videoGenerateIn struct {
	ProjectID    string `json:"project_id"            validate:"required" doc:"The project the clip belongs to, prj_ and 10 characters."`
	ConnectionID string `json:"connection_id"         validate:"required" doc:"The connection to the provider that makes the clip."`
	Model        string `json:"model"                 validate:"required" doc:"The model id, as provider:models lists it."`
	Prompt       string `json:"prompt"                validate:"required" doc:"What the clip shows, 1 to 4000 characters."`
	Seconds      int    `json:"seconds,omitempty"                         doc:"The length of the clip in seconds, 1 to 60; the default is 5. The model may make only some lengths."`
	Aspect       string `json:"aspect,omitempty"                          doc:"16:9 (the default), 9:16, 1:1, 4:3 or 3:4. The model may make only some."`
	FirstFrame   string `json:"first_frame,omitempty"                     doc:"The id of an image asset of the project the clip starts from; it is sent to the provider after approval."`
	WaitMS       int    `json:"wait_ms,omitempty"                         doc:"Wait up to this many milliseconds for the job to finish."`
	DryRun       bool   `json:"dry_run,omitempty"                         doc:"true answers the job it would create, with its estimate, and writes and sends nothing."`
}

func (h *apiHandler) videoGenerate(c *web.Ctx, in videoGenerateIn) (service.Job, error) {
	res, err := h.Service.VideoGenerate(c.Context(), service.VideoGenerateRequest{
		ProjectID: in.ProjectID, ConnectionID: in.ConnectionID, WaitMS: in.WaitMS, DryRun: in.DryRun,
		Model: in.Model, Prompt: in.Prompt, Seconds: in.Seconds, Aspect: in.Aspect, FirstFrame: in.FirstFrame,
	})

	return deref(res), err
}

type voiceListIn struct {
	ID string `json:"id" validate:"required" doc:"The id of a connection to an AI provider, con_ and 10 characters."`
}

func (h *apiHandler) imageGenerate(c *web.Ctx, in imageGenerateIn) (service.Job, error) {
	res, err := h.Service.ImageGenerate(c.Context(), service.ImageGenerateRequest{
		ProjectID: in.ProjectID, ConnectionID: in.ConnectionID, WaitMS: in.WaitMS, DryRun: in.DryRun,
		Model: in.Model, Prompt: in.Prompt, Size: in.Size, Count: in.Count,
	})

	return deref(res), err
}

func (h *apiHandler) voiceGenerate(c *web.Ctx, in voiceGenerateIn) (service.Job, error) {
	res, err := h.Service.VoiceGenerate(c.Context(), service.VoiceGenerateRequest{
		ProjectID: in.ProjectID, ConnectionID: in.ConnectionID, WaitMS: in.WaitMS, DryRun: in.DryRun,
		Model: in.Model, Text: in.Text, Voice: in.Voice, Language: in.Language, Format: in.Format,
	})

	return deref(res), err
}

func (h *apiHandler) voiceList(c *web.Ctx, in voiceListIn) (service.VoiceListResult, error) {
	res, err := h.Service.VoiceList(c.Context(), service.VoiceListRequest{ConnectionID: in.ID})

	return deref(res), err
}

type imageModelEditIn struct {
	ID           string `json:"id"                 validate:"required" doc:"The image document, img_ and 10 characters."`
	ProjectID    string `json:"project_id"         validate:"required" doc:"The project that holds the generated file and its cost, prj_ and 10 characters."`
	ConnectionID string `json:"connection_id"      validate:"required" doc:"The connection to the provider: with the image_edit capability and accepting image for remove_background, with the image capability and accepting text for generate."`
	Operation    string `json:"operation"          validate:"required" doc:"remove_background (the picture of the layer is sent to the provider) or generate (only the prompt is sent)."`
	Model        string `json:"model"              validate:"required" doc:"The model id, as provider:models lists it."`
	LayerID      string `json:"layer_id,omitempty"                     doc:"remove_background: the image layer, required. generate: a layer whose box the new picture must cover; without it, the canvas."`
	Prompt       string `json:"prompt,omitempty"                       doc:"generate only: what to draw, 1 to 4000 characters."`
	Size         string `json:"size,omitempty"                         doc:"generate only: width and height in pixels joined by x; the default is the size nearest to the box to cover."`
	WaitMS       int    `json:"wait_ms,omitempty"                      doc:"Wait up to this many milliseconds for the job to finish."`
	DryRun       bool   `json:"dry_run,omitempty"                      doc:"true answers the job it would create, with its estimate, and writes and sends nothing."`
}

func (h *apiHandler) imageModelEdit(c *web.Ctx, in imageModelEditIn) (service.Job, error) {
	res, err := h.Service.ImageModelEdit(c.Context(), service.ImageModelEditRequest{
		ProjectID: in.ProjectID, WaitMS: in.WaitMS, DryRun: in.DryRun,
		ConnectionID: in.ConnectionID, ImageID: in.ID, Operation: in.Operation, Model: in.Model,
		LayerID: in.LayerID, Prompt: in.Prompt, Size: in.Size,
	})

	return deref(res), err
}

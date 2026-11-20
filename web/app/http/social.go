package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type socialConnectIn struct {
	Network      string `json:"network"                 validate:"required" doc:"The network to sign in to, for example youtube."`
	Label        string `json:"label,omitempty"                             doc:"A name for the connection, 1 to 60 characters; the name of the network when left out."`
	ClientID     string `json:"client_id,omitempty"                         doc:"The client id of the developer project you created on the network."`
	ClientSecret string `json:"client_secret,omitempty"                     doc:"The client secret of that project. It goes into the vault and is in no answer."`
	ConnectionID string `json:"connection_id,omitempty"                     doc:"Sign in again an existing connection of the network, instead of making one."`
	RedirectURI  string `json:"redirect_uri"            validate:"required" doc:"The address of this app followed by /social/callback; it must be the redirect address you authorised in the developer project."`
}

type socialCompleteIn struct {
	State string `json:"state"           validate:"required" doc:"The state the network sent back to the redirect address."`
	Code  string `json:"code,omitempty"                      doc:"The authorization code the network sent back."`
	Error string `json:"error,omitempty"                     doc:"The error the network sent back instead of a code, for example access_denied."`
}

type socialSelectIn struct {
	ConnectionID string `json:"connection_id" validate:"required" doc:"The connection whose account is chosen."`
	AccountID    string `json:"account_id"    validate:"required" doc:"The id of one of the choices the connection lists after the sign-in."`
}

type socialPriceIn struct {
	ConnectionID string `json:"connection_id" validate:"required"          doc:"The connection of the network whose price is corrected."`
	PostMicros   int64  `json:"post_micros"   validate:"required,positive" doc:"The price of a post, in millionths of the currency: 15000 is $0.015."`
	LinkMicros   int64  `json:"link_micros"   validate:"required,positive" doc:"The price of a post with a link, in millionths of the currency: 200000 is $0.20."`
}

type publishValidateIn struct {
	ProjectID    string   `json:"project_id"              validate:"required" doc:"The project the video was exported from."`
	ConnectionID string   `json:"connection_id"           validate:"required" doc:"The connection of the network and the account the video goes to."`
	Export       string   `json:"export"                  validate:"required" doc:"The id of a succeeded export job of the project: the video."`
	Thumbnail    string   `json:"thumbnail,omitempty"                         doc:"The id of a succeeded export job of a still of the project: the cover."`
	Title        string   `json:"title"                                       doc:"The title, at most 100 characters on YouTube."`
	Description  string   `json:"description,omitempty"                       doc:"The description, at most 5,000 bytes on YouTube."`
	Tags         []string `json:"tags,omitempty"                              doc:"The tags, at most 500 characters in all on YouTube."`
	Visibility   string   `json:"visibility,omitempty"                        doc:"private (the default), unlisted or public."`
	MadeForKids  *bool    `json:"made_for_kids,omitempty"                     doc:"Required: whether the video is made for kids."`
	AIDisclosure bool     `json:"ai_disclosure,omitempty"                     doc:"The video has realistic altered or synthetic content."`
}

type publishNowIn struct {
	ProjectID    string   `json:"project_id"              validate:"required" doc:"The project the video was exported from."`
	ConnectionID string   `json:"connection_id"           validate:"required" doc:"The connection of the network and the account the video goes to."`
	Export       string   `json:"export"                  validate:"required" doc:"The id of a succeeded export job of the project: the video."`
	Thumbnail    string   `json:"thumbnail,omitempty"                         doc:"The id of a succeeded export job of a still of the project: the cover."`
	Title        string   `json:"title"                                       doc:"The title, at most 100 characters on YouTube."`
	Description  string   `json:"description,omitempty"                       doc:"The description, at most 5,000 bytes on YouTube."`
	Tags         []string `json:"tags,omitempty"                              doc:"The tags, at most 500 characters in all on YouTube."`
	Visibility   string   `json:"visibility,omitempty"                        doc:"private (the default), unlisted or public."`
	MadeForKids  *bool    `json:"made_for_kids,omitempty"                     doc:"Required: whether the video is made for kids."`
	AIDisclosure bool     `json:"ai_disclosure,omitempty"                     doc:"The video has realistic altered or synthetic content."`
	TimelineID   string   `json:"timeline_id,omitempty"                       doc:"The timeline the job is kept on; required when the project has several."`
	WaitMS       int      `json:"wait_ms,omitempty"                           doc:"0 to 25000: hold the call until the job is final."`
	DryRun       any      `json:"dry_run,omitempty"`
}

type publishStatusIn struct {
	JobID string `json:"job_id" validate:"required" doc:"The job id that PublishNow answered."`
}

func (in *publishValidateIn) args() service.PublishArgs {
	return service.PublishArgs{
		ConnectionID: in.ConnectionID, Export: in.Export, Thumbnail: in.Thumbnail, Title: in.Title,
		Description: in.Description, Tags: in.Tags, Visibility: in.Visibility, MadeForKids: in.MadeForKids,
		AIDisclosure: in.AIDisclosure,
	}
}

func (in *publishNowIn) args() service.PublishArgs {
	v := publishValidateIn{
		ConnectionID: in.ConnectionID, Export: in.Export, Thumbnail: in.Thumbnail, Title: in.Title,
		Description: in.Description, Tags: in.Tags, Visibility: in.Visibility, MadeForKids: in.MadeForKids,
		AIDisclosure: in.AIDisclosure,
	}

	return v.args()
}

func (h *apiHandler) socialNetworks(c *web.Ctx, _ noIn) (service.SocialNetworkListResult, error) {
	res, err := h.Service.SocialNetworkList(c.Context(), service.SocialNetworkListRequest{})

	return deref(res), err
}

func (h *apiHandler) socialConnect(c *web.Ctx, in socialConnectIn) (service.SocialConnectBeginResult, error) {
	res, err := h.Service.SocialConnectBegin(c.Context(), service.SocialConnectBeginRequest(in))

	return deref(res), err
}

func (h *apiHandler) socialComplete(c *web.Ctx, in socialCompleteIn) (service.SocialAccount, error) {
	res, err := h.Service.SocialConnectComplete(c.Context(), service.SocialConnectCompleteRequest(in))

	return deref(res), err
}

func (h *apiHandler) socialAccounts(c *web.Ctx, _ noIn) (service.SocialAccountListResult, error) {
	res, err := h.Service.SocialAccountList(c.Context(), service.SocialAccountListRequest{})

	return deref(res), err
}

func (h *apiHandler) socialSelect(c *web.Ctx, in socialSelectIn) (service.SocialAccount, error) {
	res, err := h.Service.SocialAccountSelect(c.Context(), service.SocialAccountSelectRequest(in))

	return deref(res), err
}

func (h *apiHandler) socialPrice(c *web.Ctx, in socialPriceIn) (service.SocialAccount, error) {
	res, err := h.Service.SocialPriceSet(c.Context(), service.SocialPriceSetRequest(in))

	return deref(res), err
}

func (h *apiHandler) publishValidate(c *web.Ctx, in publishValidateIn) (service.PublishValidateResult, error) {
	res, err := h.Service.PublishValidate(c.Context(), service.PublishValidateRequest{
		ProjectID: in.ProjectID, PublishArgs: in.args(),
	})

	return deref(res), err
}

func (h *apiHandler) publishNow(c *web.Ctx, in publishNowIn) (service.Job, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.Job{}, err
	}

	res, err := h.Service.PublishNow(c.Context(), service.PublishNowRequest{
		ProjectID: in.ProjectID, TimelineID: in.TimelineID, PublishArgs: in.args(), WaitMS: in.WaitMS, DryRun: dryRun,
	})

	return deref(res), err
}

func (h *apiHandler) publishStatus(c *web.Ctx, in publishStatusIn) (service.PublishStatusResult, error) {
	res, err := h.Service.PublishStatus(c.Context(), service.PublishStatusRequest{JobID: in.JobID})

	return deref(res), err
}

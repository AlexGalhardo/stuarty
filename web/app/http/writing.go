package httpapi

import (
	"net/http"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The HTTP encoding of the writing operations (SDD-019 section 8). They carry no rule: the Service validates
// (ADR-0008) and core/writing counts.

type textListIn struct {
	ProjectID string `json:"project_id,omitempty" doc:"Only the texts of this project; omit it for every project the caller may see."`
	TextKind  string `json:"text_kind,omitempty"  doc:"script, copy or caption; omit it for every kind."`
	State     string `json:"state,omitempty"      doc:"active (default) or trashed."`
	pageIn
}

type textGetIn struct {
	ID       string `json:"id"                 validate:"required"`
	ItemID   string `json:"item_id"            validate:"required"`
	Revision int    `json:"revision,omitempty"                     doc:"A revision number from 1; default the latest."`
}

type textCreateIn struct {
	ID       string `json:"id"                  validate:"required"`
	Title    string `json:"title"               validate:"required" doc:"1 to 120 characters."`
	TextKind string `json:"text_kind,omitempty"                     doc:"script, copy (default) or caption."`
	Text     string `json:"text"                                    doc:"The Markdown, at most 256 KiB."`
	DryRun   any    `json:"dry_run,omitempty"`
}

type textWriteIn struct {
	ID           string `json:"id"                      validate:"required"`
	ItemID       string `json:"item_id"                 validate:"required"`
	Title        string `json:"title,omitempty"                             doc:"A new title; omit it to keep the title."`
	Text         string `json:"text"                                        doc:"The Markdown of the new revision, at most 256 KiB."`
	BaseRevision *int   `json:"base_revision,omitempty"                     doc:"The revision the writer read: a newer one is E_REVISION_CONFLICT."`
	DryRun       any    `json:"dry_run,omitempty"`
}

type textRevisionsIn struct {
	ID     string `json:"id"               validate:"required"`
	ItemID string `json:"item_id"          validate:"required"`
	Before int    `json:"before,omitempty"                     doc:"Start under this revision: the next_before of the previous page."`
	Limit  int    `json:"limit,omitempty"                      doc:"1 to 100; default 50."`
	From   int    `json:"from,omitempty"                       doc:"Compare this revision with To, by paragraph."`
	To     int    `json:"to,omitempty"                         doc:"The revision to compare with; default the latest."`
}

type textRevertIn struct {
	ID       string `json:"id"                validate:"required"`
	ItemID   string `json:"item_id"           validate:"required"`
	Revision int    `json:"revision"          validate:"required" doc:"The revision whose text comes back as a new revision."`
	DryRun   any    `json:"dry_run,omitempty"`
}

type textDraftIn struct {
	ID           string   `json:"id"                    validate:"required"`
	ItemID       string   `json:"item_id"               validate:"required"`
	ConnectionID string   `json:"connection_id"         validate:"required" doc:"The connection to the provider that writes the draft."`
	Model        string   `json:"model"                 validate:"required" doc:"The model id, as provider:models lists it."`
	Instruction  string   `json:"instruction"           validate:"required" doc:"What to write, 1 to 4000 characters."`
	Network      string   `json:"network,omitempty"                         doc:"A caption: draft the variant of this network only."`
	MaxTokens    int      `json:"max_tokens,omitempty"                      doc:"The most tokens the model may write; the estimate counts all of them."`
	Temperature  *float64 `json:"temperature,omitempty"                     doc:"0 to 2; leave out for the default of the provider."`
	WaitMS       int      `json:"wait_ms,omitempty"                         doc:"Wait up to this many milliseconds for the job to finish."`
	DryRun       bool     `json:"dry_run,omitempty"                         doc:"true answers the job it would create, with its estimate, and writes and sends nothing."`
}

type captionValidateIn struct {
	ProjectID string `json:"project_id,omitempty" doc:"The project of the caption, prj_ and 10 characters."`
	ItemID    string `json:"item_id,omitempty"    doc:"Check a stored caption."`
	Body      string `json:"body,omitempty"       doc:"Check a whole caption, saved or not: the parts that start with ## youtube, ## instagram, ## facebook, ## x or ## tiktok."`
	Text      string `json:"text,omitempty"       doc:"Check one variant; give its network too."`
	Network   string `json:"network,omitempty"    doc:"The network of Text, or the one variant of a stored caption to check."`
}

type scriptEstimateIn struct {
	ProjectID      string `json:"project_id,omitempty"       doc:"The project of the script, prj_ and 10 characters."`
	ItemID         string `json:"item_id,omitempty"          doc:"Estimate a stored script."`
	Text           string `json:"text,omitempty"             doc:"Estimate a script that is not saved."`
	WordsPerMinute int    `json:"words_per_minute,omitempty" doc:"60 to 300; default 150."`
}

func (h *apiHandler) textList(c *web.Ctx, in textListIn) (service.TextListResult, error) {
	res, err := h.Service.TextList(c.Context(), service.TextListRequest{
		ProjectID: in.ProjectID, TextKind: in.TextKind, State: in.State, Cursor: in.Cursor, Limit: in.Limit,
	})

	return deref(res), err
}

func (h *apiHandler) textGet(c *web.Ctx, in textGetIn) (service.TextGetResult, error) {
	res, err := h.Service.TextGet(
		c.Context(),
		service.TextGetRequest{ProjectID: in.ID, ItemID: in.ItemID, Revision: in.Revision},
	)

	return deref(res), err
}

func (h *apiHandler) textCreate(c *web.Ctx, in textCreateIn) (service.TextWriteResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.TextWriteResult{}, err
	}

	res, err := h.Service.TextWrite(c.Context(), service.TextWriteRequest{
		ProjectID: in.ID, Title: in.Title, TextKind: in.TextKind, Text: in.Text, DryRun: dryRun,
	})
	if err != nil {
		return service.TextWriteResult{}, err
	}

	if dryRun {
		return service.TextWriteResult{}, c.JSON(http.StatusOK, res)
	}

	return service.TextWriteResult{}, c.JSON(http.StatusCreated, res)
}

func (h *apiHandler) textWrite(c *web.Ctx, in textWriteIn) (service.TextWriteResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.TextWriteResult{}, err
	}

	res, err := h.Service.TextWrite(c.Context(), service.TextWriteRequest{
		ProjectID: in.ID, ItemID: in.ItemID, Title: in.Title, Text: in.Text, BaseRevision: in.BaseRevision,
		DryRun: dryRun,
	})

	return deref(res), err
}

func (h *apiHandler) textRevisions(c *web.Ctx, in textRevisionsIn) (service.TextRevisionsResult, error) {
	res, err := h.Service.TextRevisions(c.Context(), service.TextRevisionsRequest{
		ProjectID: in.ID, ItemID: in.ItemID, Before: in.Before, Limit: in.Limit, From: in.From, To: in.To,
	})

	return deref(res), err
}

func (h *apiHandler) textRevert(c *web.Ctx, in textRevertIn) (service.TextWriteResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.TextWriteResult{}, err
	}

	res, err := h.Service.TextRevert(c.Context(), service.TextRevertRequest{
		ProjectID: in.ID, ItemID: in.ItemID, Revision: in.Revision, DryRun: dryRun,
	})

	return deref(res), err
}

func (h *apiHandler) textDraft(c *web.Ctx, in textDraftIn) (service.Job, error) {
	res, err := h.Service.TextDraft(c.Context(), service.TextDraftRequest{
		ProjectID: in.ID, ItemID: in.ItemID, ConnectionID: in.ConnectionID, Model: in.Model,
		Instruction: in.Instruction, Network: in.Network, MaxTokens: in.MaxTokens, Temperature: in.Temperature,
		WaitMS: in.WaitMS, DryRun: in.DryRun,
	})

	return deref(res), err
}

func (h *apiHandler) captionValidate(c *web.Ctx, in captionValidateIn) (service.CaptionValidateResult, error) {
	res, err := h.Service.CaptionValidate(c.Context(), service.CaptionValidateRequest(in))

	return deref(res), err
}

func (h *apiHandler) scriptEstimate(c *web.Ctx, in scriptEstimateIn) (service.ScriptEstimateResult, error) {
	res, err := h.Service.ScriptEstimate(c.Context(), service.ScriptEstimateRequest(in))

	return deref(res), err
}

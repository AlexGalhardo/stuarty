package httpapi

import (
	"net/http"
	"net/url"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// The HTTP encoding of the operations on the production of a project (SDD-003 section 2.3.1). They carry no
// rule: the Service validates (ADR-0008).

type describeIn struct {
	ID        string `json:"id"                   validate:"required"`
	MaxTokens int    `json:"max_tokens,omitempty"                     doc:"The budget of the text in tokens, 200 to 6000; default 1500."`
}

type duplicateIn struct {
	ID     string `json:"id"                validate:"required"`
	Name   string `json:"name,omitempty"                        doc:"The name of the copy; default: the name and the word copy."`
	DryRun any    `json:"dry_run,omitempty"`
}

type itemAddIn struct {
	ID       string  `json:"id"                 validate:"required"`
	Kind     string  `json:"kind"               validate:"required" doc:"text, timeline or output; the brief is written with PUT /brief."`
	Title    string  `json:"title"              validate:"required" doc:"1 to 120 characters."`
	Position *int    `json:"position,omitempty"                     doc:"Where in the order of the items, from 0; default the end."`
	Text     string  `json:"text,omitempty"                         doc:"Markdown of a text item, at most 256 KiB."`
	TextKind string  `json:"text_kind,omitempty"                    doc:"A text item: script, copy (default) or caption."`
	Preset   string  `json:"preset,omitempty"                       doc:"A timeline: 9:16 (default), 1:1, 16:9 or 4:5."`
	FPS      fpsText `json:"fps,omitempty"                          doc:"A timeline: 30 (default); n or n/1001."`
	JobID    string  `json:"job_id,omitempty"                       doc:"An output: a succeeded export job of this project."`
	DryRun   any     `json:"dry_run,omitempty"`
}

type itemMoveIn struct {
	ID       string `json:"id"                validate:"required"`
	ItemID   string `json:"item_id"           validate:"required"`
	Position int    `json:"position"                              doc:"Where in the order of the items, from 0; past the end is the end."`
	DryRun   any    `json:"dry_run,omitempty"`
}

type itemRenameIn struct {
	ID     string `json:"id"                validate:"required"`
	ItemID string `json:"item_id"           validate:"required"`
	Title  string `json:"title"             validate:"required" doc:"1 to 120 characters."`
	DryRun any    `json:"dry_run,omitempty"`
}

type itemStateIn struct {
	ID     string `json:"id"                validate:"required"`
	ItemID string `json:"item_id"           validate:"required"`
	DryRun any    `json:"dry_run,omitempty"`
}

type briefGetIn struct {
	ID string `json:"id" validate:"required"`
}

type briefSetIn struct {
	ID     string `json:"id"                validate:"required"`
	Text   string `json:"text"                                  doc:"Markdown, at most 256 KiB; empty clears the brief."`
	DryRun any    `json:"dry_run,omitempty"`
}

func (h *apiHandler) projectDescribe(c *web.Ctx, in describeIn) (service.ProjectDescribeResult, error) {
	res, err := h.Service.ProjectDescribe(
		c.Context(),
		service.ProjectDescribeRequest{ProjectID: in.ID, MaxTokens: in.MaxTokens},
	)

	return deref(res), err
}

func (h *apiHandler) projectDuplicate(c *web.Ctx, in duplicateIn) (service.ProjectCreateResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ProjectCreateResult{}, err
	}

	res, err := h.Service.ProjectDuplicate(c.Context(), service.ProjectDuplicateRequest{
		ProjectID: in.ID, Name: in.Name, DryRun: dryRun,
	})
	if err != nil {
		return service.ProjectCreateResult{}, err
	}

	if dryRun {
		return service.ProjectCreateResult{}, c.JSON(http.StatusOK, res)
	}

	c.SetHeader("Location", BasePath+"/projects/"+url.PathEscape(res.ProjectID))

	return service.ProjectCreateResult{}, c.JSON(http.StatusCreated, res)
}

func (h *apiHandler) projectItemAdd(c *web.Ctx, in itemAddIn) (service.ProjectItemResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ProjectItemResult{}, err
	}

	res, err := h.Service.ProjectItemAdd(c.Context(), service.ProjectItemAddRequest{
		ProjectID: in.ID, Kind: in.Kind, Title: in.Title, Position: in.Position, Text: in.Text,
		TextKind: in.TextKind, Preset: in.Preset, FPS: string(in.FPS), JobID: in.JobID, DryRun: dryRun,
	})
	if err != nil {
		return service.ProjectItemResult{}, err
	}

	if dryRun {
		return service.ProjectItemResult{}, c.JSON(http.StatusOK, res)
	}

	return service.ProjectItemResult{}, c.JSON(http.StatusCreated, res)
}

func (h *apiHandler) projectItemMove(c *web.Ctx, in itemMoveIn) (service.ProjectItemResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ProjectItemResult{}, err
	}

	res, err := h.Service.ProjectItemMove(c.Context(), service.ProjectItemMoveRequest{
		ProjectID: in.ID, ItemID: in.ItemID, Position: in.Position, DryRun: dryRun,
	})

	return deref(res), err
}

func (h *apiHandler) projectItemRename(c *web.Ctx, in itemRenameIn) (service.ProjectItemResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ProjectItemResult{}, err
	}

	res, err := h.Service.ProjectItemRename(c.Context(), service.ProjectItemRenameRequest{
		ProjectID: in.ID, ItemID: in.ItemID, Title: in.Title, DryRun: dryRun,
	})

	return deref(res), err
}

func (h *apiHandler) projectItemState(
	c *web.Ctx, in itemStateIn, restore bool,
) (service.ProjectItemResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ProjectItemResult{}, err
	}

	req := service.ProjectItemTrashRequest{ProjectID: in.ID, ItemID: in.ItemID, DryRun: dryRun}

	var res *service.ProjectItemResult

	if restore {
		res, err = h.Service.ProjectItemRestore(c.Context(), req)
	} else {
		res, err = h.Service.ProjectItemTrash(c.Context(), req)
	}

	return deref(res), err
}

func (h *apiHandler) briefGet(c *web.Ctx, in briefGetIn) (service.BriefResult, error) {
	res, err := h.Service.BriefGet(c.Context(), service.BriefGetRequest{ProjectID: in.ID})

	return deref(res), err
}

func (h *apiHandler) briefSet(c *web.Ctx, in briefSetIn) (service.BriefResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.BriefResult{}, err
	}

	res, err := h.Service.BriefSet(
		c.Context(),
		service.BriefSetRequest{ProjectID: in.ID, Text: in.Text, DryRun: dryRun},
	)

	return deref(res), err
}

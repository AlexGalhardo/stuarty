package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// handlers are the tools whose Service methods exist; a tool without one is in
// Pending.
func (a *adapter) handlers() map[string]callFunc {
	return map[string]callFunc{
		"project_list":   projectList,
		"project_create": projectCreate,
		"project_get":    projectGet,
		"project_trash":  projectTrash,
		"edit":           edit,
		"command_help":   commandHelp,
		"history":        history,
		"lint":           lintTool,
		"media_import":   mediaImport,
		"media_inspect":  mediaInspect,
		"job_start":      jobStart,
		"job_get":        jobGet,
		"job_cancel":     jobCancel,
		"template":       template,
		"view":           view,
		"font_import":    fontImport,
		"connection":     connection,
		"image":          imageTool,
		"project_item":   projectItem,
		"library":        libraryTool,
	}
}

// decode reads the arguments, which parseArguments already checked against the
// schema, into a struct.
func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, &service.Error{Code: command.CodeValidation, Message: "the arguments are not valid."}
	}

	return v, nil
}

type pageArgs struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

func (p pageArgs) page() service.Page { return service.Page{Cursor: p.Cursor, Limit: p.Limit} }

func projectList(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		State string `json:"state"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.ProjectList(ctx, service.ProjectListRequest{State: in.State, Page: in.page()})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// fpsArg is a frame rate written as text ("30", "30000/1001") or as a number
// (30), as the CLI and the HTTP API take it (VV-198); the Service sees text.
type fpsArg string

func (f *fpsArg) UnmarshalJSON(b []byte) error {
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*f = fpsArg(n.String())

		return nil
	}

	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	*f = fpsArg(s)

	return nil
}

func projectCreate(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		Name   string `json:"name"`
		Preset string `json:"preset"`
		FPS    fpsArg `json:"fps"`
		ID     string `json:"id"`
		From   string `json:"from"`
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.From != "" {
		dup, derr := svc.ProjectDuplicate(ctx, service.ProjectDuplicateRequest{ProjectID: in.From, Name: in.Name})
		if derr != nil {
			return nil, derr
		}

		return &toolOutput{value: dup, text: dup.Text}, nil
	}

	res, err := svc.ProjectCreate(
		ctx,
		service.ProjectCreateRequest{Name: in.Name, Preset: in.Preset, FPS: string(in.FPS), ID: in.ID},
	)
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func projectGet(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID  string `json:"project_id"`
		TimelineID string `json:"timeline_id"`
		Detail     string `json:"detail"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.Detail == "overview" {
		return projectDescribe(ctx, svc, in.ProjectID, 0)
	}

	res, err := svc.ProjectGet(
		ctx,
		service.ProjectGetRequest{
			ProjectID:  in.ProjectID,
			TimelineID: in.TimelineID,
			Detail:     in.Detail,
			Page:       in.page(),
		},
	)
	if err != nil {
		return nil, err
	}

	if res.Document != nil {
		// The document is a bulk member, left out of the structured content, so the text carries it.
		doc, merr := json.Marshal(res.Document)
		if merr != nil {
			return nil, merr
		}

		res.Text = textview.Note + "\n" + string(doc)
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// projectTrash is the one tool of two operations: trash, or restore when asked.
func projectTrash(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID string `json:"project_id"`
		Restore   bool   `json:"restore"`
	}](raw)
	if err != nil {
		return nil, err
	}

	fn := svc.ProjectTrash
	if in.Restore {
		fn = svc.ProjectRestore
	}

	res, err := fn(ctx, service.ProjectTrashRequest{ProjectID: in.ProjectID})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// edit applies a batch to the document of its domain: a timeline (video, the default) or an image (SDD-003
// section 4.8). The operations are the registry's of that domain; the Service refuses a name it does not have.
func edit(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		Domain         string              `json:"domain"`
		ProjectID      string              `json:"project_id"`
		TimelineID     string              `json:"timeline_id"`
		ImageID        string              `json:"image_id"`
		BaseRevision   int64               `json:"base_revision"`
		Operations     []command.Operation `json:"operations"`
		IdempotencyKey string              `json:"idempotency_key"`
		DryRun         bool                `json:"dry_run"`
	}](raw)
	if err != nil {
		return nil, err
	}

	domain, err := routeDomain("edit", in.Domain)
	if err != nil {
		return nil, err
	}

	if domain == command.DomainImage {
		if err := checkMembers(
			"edit",
			domain,
			raw,
			[]string{"image_id"},
			[]string{"project_id", "timeline_id"},
		); err != nil {
			return nil, err
		}

		res, err := svc.ImageEdit(ctx, service.ImageEditRequest{
			ImageID: in.ImageID, BaseRevision: in.BaseRevision, Operations: in.Operations,
			IdempotencyKey: in.IdempotencyKey, DryRun: in.DryRun,
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: service.ImageEditText(res)}, nil
	}

	if err := checkMembers("edit", domain, raw, []string{"project_id"}, []string{"image_id"}); err != nil {
		return nil, err
	}

	res, err := svc.Edit(ctx, service.EditRequest{
		ProjectID: in.ProjectID, TimelineID: in.TimelineID, BaseRevision: in.BaseRevision, Operations: in.Operations,
		IdempotencyKey: in.IdempotencyKey, DryRun: in.DryRun,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: service.EditText(res)}, nil
}

func commandHelp(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.Domain == "export" {
		return exportHelp(ctx, svc, in.Name)
	}

	res, err := svc.CommandHelp(ctx, service.CommandHelpRequest{Name: in.Name, Domain: in.Domain})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// history is the change log of a timeline behind one tool (SDD-003 section 4.1): list, undo and redo, and since
// epic 36 revert, diff and the checkpoints.
func history(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[historyArgs](raw)
	if err != nil {
		return nil, err
	}

	domain, err := routeDomain("history", in.Domain)
	if err != nil {
		return nil, err
	}

	if domain == command.DomainImage {
		return imageHistory(ctx, svc, raw, in.Action, in.ImageID)
	}

	if err := checkMembers("history", domain, raw, []string{"project_id"}, []string{"image_id"}); err != nil {
		return nil, err
	}

	return videoHistory(ctx, svc, raw, &in)
}

func lintTool(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID  string `json:"project_id"`
		TimelineID string `json:"timeline_id"`
		Severity   string `json:"severity"`
		Preset     string `json:"preset"`
		Start      string `json:"start"`
		End        string `json:"end"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	// With a preset the lint is the fit check of SDD-004 section 8.2 (ExportCheck), over the range when one is
	// given, as export:check and GET /projects/{id}/export-check take it.
	if in.Preset != "" {
		chk, cerr := svc.ExportCheck(ctx, service.ExportCheckRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, Preset: in.Preset, Start: in.Start, End: in.End,
		})
		if cerr != nil {
			return nil, cerr
		}

		return &toolOutput{value: chk, text: chk.Text}, nil
	}

	if in.Start != "" || in.End != "" {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "start",
			Message: "start and end are the range of the fit check: they need preset.",
		}
	}

	res, err := svc.Lint(
		ctx,
		service.LintRequest{ProjectID: in.ProjectID, TimelineID: in.TimelineID, Severity: in.Severity, Page: in.page()},
	)
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func mediaImport(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID  string          `json:"project_id"`
		TimelineID string          `json:"timeline_id"`
		Paths      []string        `json:"paths"`
		AssetIDs   []string        `json:"asset_ids"`
		WaitMS     *int            `json:"wait_ms"`
		Hash       string          `json:"hash"`
		Origin     json.RawMessage `json:"origin"`
		Info       json.RawMessage `json:"info"`
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.Hash != "" && len(in.Paths) == 0 {
		return libraryEdit(ctx, svc, in.Hash, in.Info)
	}

	if in.ProjectID == "" || len(in.Paths) == 0 {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "paths",
			Message: "media_import needs project_id and paths, or a hash to edit.",
		}
	}

	origin, info, err := importBlocks(in.Origin, in.Info)
	if err != nil {
		return nil, err
	}

	if len(in.AssetIDs) > 0 && len(in.AssetIDs) != len(in.Paths) {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "asset_ids",
			Message: "asset_ids must have as many entries as paths.",
		}
	}

	files := make([]service.ImportFile, len(in.Paths))
	for i, p := range in.Paths {
		if strings.HasPrefix(p, "sha256:") {
			files[i].Hash = p
		} else {
			files[i].Path = p
		}

		if len(in.AssetIDs) > 0 {
			files[i].AssetID = in.AssetIDs[i]
		}
	}

	wait := defaultImportWaitMS
	if in.WaitMS != nil {
		wait = *in.WaitMS
	}

	res, err := svc.MediaImport(
		ctx,
		service.MediaImportRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, Files: files, WaitMS: wait, Origin: origin, Info: info,
		},
	)
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text, isError: len(res.Assets) > 0 && res.Imported == 0}, nil
}

// defaultImportWaitMS is the default of wait_ms in the media_import definition.
const defaultImportWaitMS = 10000

// mediaInspect is two operations behind one tool (SDD-003 section 4.1): with an
// asset_id it describes the asset, without one it lists the assets.
func mediaInspect(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID  string          `json:"project_id"`
		TimelineID string          `json:"timeline_id"`
		AssetID    string          `json:"asset_id"`
		Sections   []string        `json:"sections"`
		MinSilence string          `json:"min_silence"`
		Hash       string          `json:"hash"`
		Library    json.RawMessage `json:"library"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.Hash != "" || in.Library != nil || in.ProjectID == "" {
		return libraryInspect(ctx, svc, in.Hash, in.ProjectID, in.Library, in.page())
	}

	if in.AssetID == "" {
		res, lerr := svc.MediaList(
			ctx,
			service.MediaListRequest{ProjectID: in.ProjectID, TimelineID: in.TimelineID, Page: in.page()},
		)
		if lerr != nil {
			return nil, lerr
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}

	res, err := svc.MediaInspect(ctx, service.MediaInspectRequest{
		ProjectID:  in.ProjectID,
		TimelineID: in.TimelineID,
		AssetID:    in.AssetID,
		Sections:   in.Sections,
		MinSilence: in.MinSilence,
		Page:       in.page(),
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func jobStart(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID  string          `json:"project_id"`
		TimelineID string          `json:"timeline_id"`
		Kind       string          `json:"kind"`
		Args       json.RawMessage `json:"args"`
		WaitMS     int             `json:"wait_ms"`
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.Kind == service.JobTextDraft {
		return textDraft(ctx, svc, in.ProjectID, in.Args, in.WaitMS)
	}

	if in.Kind == service.JobExportBatch {
		return exportBatch(ctx, svc, service.ExportBatchRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, WaitMS: in.WaitMS,
		}, in.Args)
	}

	res, err := svc.JobStart(ctx, service.JobStartRequest{
		ProjectID: in.ProjectID, TimelineID: in.TimelineID, Kind: in.Kind, Args: in.Args, WaitMS: in.WaitMS,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// exportBatch is job_start with kind export_batch: args.items are the exports (SDD-004 section 8.4).
func exportBatch(
	ctx context.Context, svc service.Service, req service.ExportBatchRequest, args json.RawMessage,
) (*toolOutput, error) {
	in, err := decode[struct {
		Items []service.ExportBatchItem `json:"items"`
	}](args)
	if err != nil {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "args",
			Message: "args must be {items: [{preset, format, codec, start, end}]}.",
		}
	}

	req.Items = in.Items

	res, err := svc.ExportBatch(ctx, req)
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// exportHelp is command_help with domain export: the presets and the codecs of this machine.
func exportHelp(ctx context.Context, svc service.Service, name string) (*toolOutput, error) {
	if name != "" {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "name",
			Message: "domain export lists the presets and takes no name.",
		}
	}

	res, err := svc.ExportPresetList(ctx, service.ExportPresetListRequest{Codecs: true})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// jobGet reads one job (job_id) or lists the jobs of a project (project_id).
func jobCancel(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		JobID string `json:"job_id"`
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.JobCancel(ctx, service.JobCancelRequest{JobID: in.JobID})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func jobGet(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		JobID      string `json:"job_id"`
		ProjectID  string `json:"project_id"`
		TimelineID string `json:"timeline_id"`
		State      string `json:"state"`
		WaitMS     int    `json:"wait_ms"`
		ApprovalID string `json:"approval_id"`
		Approvals  string `json:"approvals"`
	}](raw)
	if err != nil {
		return nil, err
	}

	switch {
	case in.ApprovalID != "" || in.Approvals != "":
		if in.JobID != "" || (in.ApprovalID != "" && in.Approvals != "") {
			return nil, &service.Error{
				Code: command.CodeValidation, Path: "approval_id",
				Message: "give one of job_id, approval_id or approvals.",
			}
		}

		return approvalRead(ctx, svc, in.ApprovalID, in.Approvals, in.ProjectID)
	case in.JobID != "":
		res, gerr := svc.JobGet(ctx, service.JobGetRequest{JobID: in.JobID, WaitMS: in.WaitMS})
		if gerr != nil {
			return nil, gerr
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case in.ProjectID != "":
		res, lerr := svc.JobList(
			ctx,
			service.JobListRequest{ProjectID: in.ProjectID, TimelineID: in.TimelineID, State: in.State},
		)
		if lerr != nil {
			return nil, lerr
		}

		return &toolOutput{value: res, text: res.Text}, nil
	default:
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "job_id",
			Message: "give job_id, or project_id to list its jobs.",
		}
	}
}

// view is four operations behind one tool (SDD-003 section 4.2): a frame at "at", a contact sheet, in mode motion the
// brief of an external render, and in mode audio the measurement of the sound (epic 34). It is the only tool that
// returns an image block.
func view(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ProjectID  string   `json:"project_id"`
		TimelineID string   `json:"timeline_id"`
		Mode       string   `json:"mode"`
		At         string   `json:"at"`
		Times      []string `json:"times"`
		From       string   `json:"from"`
		Until      string   `json:"until"`
		Count      int      `json:"count"`
		MaxTokens  int      `json:"max_tokens"`
		ItemID     string   `json:"item_id"`
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.Mode == "audio" {
		if in.At != "" || len(in.Times) > 0 || in.Count != 0 || in.MaxTokens != 0 {
			return nil, &service.Error{
				Code: command.CodeValidation, Path: "mode", Message: "mode audio takes item_id, from and until only.",
				Hint: "from and until are the range; leave them out for the item, or for the timeline",
			}
		}

		measure, merr := svc.AudioMeasure(ctx, service.AudioMeasureRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, ItemID: in.ItemID, Start: in.From, End: in.Until,
		})
		if merr != nil {
			return nil, merr
		}

		return &toolOutput{value: measure, text: measure.Text}, nil
	}

	// As the CLI refuses --item outside --mode audio: a frame, a sheet and a motion brief are of the timeline.
	if in.ItemID != "" {
		return nil, &service.Error{
			Code: command.CodeValidation, Path: "item_id", Message: "item_id is for mode audio.",
			Hint: "use mode audio to measure one item",
		}
	}

	if in.Mode == "motion" {
		if in.At != "" || len(in.Times) > 0 || in.Count != 0 || in.MaxTokens != 0 {
			return nil, &service.Error{
				Code: command.CodeValidation, Path: "mode",
				Message: "at, times, count and max_tokens are not accepted in mode motion.",
				Hint:    "use from and until for the range",
			}
		}

		return motionTool(ctx, svc, in.ProjectID, in.TimelineID, in.From, in.Until)
	}

	var res *service.ViewResult

	if in.Mode == "sheet" {
		if in.At != "" {
			return nil, &service.Error{
				Code: command.CodeValidation, Path: "at", Message: "at is not accepted in mode sheet.",
				Hint: "use times, or from, until and count",
			}
		}

		res, err = svc.ViewSheet(ctx, service.ViewSheetRequest{
			ProjectID:  in.ProjectID,
			TimelineID: in.TimelineID,
			Times:      in.Times,
			From:       in.From,
			Until:      in.Until,
			Count:      in.Count,
			MaxTokens:  in.MaxTokens,
		})
	} else {
		res, err = svc.ViewFrame(ctx, service.ViewFrameRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, At: in.At, MaxTokens: in.MaxTokens,
		})
	}

	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text, image: res.Image, mime: res.MIME}, nil
}

// templateMembers are the members each action of the template tool uses
// (SDD-003 section 4.2): the others are refused, and the required ones must be
// there.
var templateMembers = map[string]struct{ required, optional []string }{
	"list":   {nil, []string{"cursor", "limit"}},
	"save":   {[]string{"project_id", "name"}, []string{"description"}},
	"new":    {[]string{"template_id", "name"}, []string{"fill", "id"}},
	"export": {[]string{"template_id"}, nil},
	"import": {[]string{"path"}, nil},
}

// checkTemplateMembers refuses a member the action does not use and names the
// path of a missing one.
func checkTemplateMembers(action string, raw json.RawMessage) error {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil {
		return &service.Error{Code: command.CodeValidation, Message: "the arguments are not valid."}
	}

	want, known := templateMembers[action]
	if !known {
		return &service.Error{
			Code: command.CodeValidation, Path: "action",
			Message: "action " + action + " is not a template action.", Hint: "use list, save, new, export or import",
		}
	}

	for name := range given {
		if name != "action" && !slices.Contains(want.required, name) && !slices.Contains(want.optional, name) {
			return &service.Error{
				Code: command.CodeValidation, Path: name,
				Message: name + " is not used by the action " + action + ".",
			}
		}
	}

	for _, name := range want.required {
		if _, ok := given[name]; !ok {
			return &service.Error{
				Code: command.CodeValidation, Path: name,
				Message: name + " is required by the action " + action + ".",
			}
		}
	}

	return nil
}

func template(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		Action      string                       `json:"action"`
		TemplateID  string                       `json:"template_id"`
		ProjectID   string                       `json:"project_id"`
		TimelineID  string                       `json:"timeline_id"`
		ID          string                       `json:"id"`
		Name        string                       `json:"name"`
		Description string                       `json:"description"`
		Fill        map[string]service.SlotValue `json:"fill"`
		Path        string                       `json:"path"`
		pageArgs
	}](raw)
	if err != nil {
		return nil, err
	}

	if err := checkTemplateMembers(in.Action, raw); err != nil {
		return nil, err
	}

	switch in.Action {
	case "list":
		res, err := svc.TemplateList(ctx, service.TemplateListRequest{Page: in.page()})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "save":
		res, err := svc.TemplateSave(ctx, service.TemplateSaveRequest{
			ProjectID: in.ProjectID, TimelineID: in.TimelineID, Name: in.Name, Description: in.Description,
		})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "export":
		res, err := svc.TemplateExport(ctx, service.TemplateRef{TemplateID: in.TemplateID})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	case "import":
		res, err := svc.TemplateImport(ctx, service.TemplateImportRequest{Path: in.Path})
		if err != nil {
			return nil, err
		}

		return &toolOutput{value: res, text: res.Text}, nil
	}

	res, err := svc.TemplateInstantiate(ctx, service.TemplateInstantiateRequest{
		TemplateID: in.TemplateID, Name: in.Name, Fill: in.Fill, ID: in.ID,
	})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func fontImport(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		Path string `json:"path"`
	}](raw)
	if err != nil {
		return nil, err
	}

	res, err := svc.FontImport(ctx, service.FontImportRequest{Path: in.Path})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// connection lists the destinations the user connected, or reads one. It never carries a credential.
func connection(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[struct {
		ID string `json:"id"`
	}](raw)
	if err != nil {
		return nil, err
	}

	if in.ID != "" {
		res, gerr := svc.ConnectionGet(ctx, service.ConnectionRef{ID: in.ID})
		if gerr != nil {
			return nil, gerr
		}

		return &toolOutput{value: res, text: res.Summary()}, nil
	}

	res, err := svc.ConnectionList(ctx, service.ConnectionListRequest{})
	if err != nil {
		return nil, err
	}

	lines := []string{fmt.Sprintf("%d connection(s)", len(res.Connections))}
	for i := range res.Connections {
		lines = append(lines, res.Connections[i].Summary())
	}

	return &toolOutput{value: res, text: strings.Join(lines, "\n")}, nil
}

// projectDescribe is project_get with detail overview: the description of the whole production (SDD-003 section
// 2.3.1), which has the project id, the name, the items and the text, and not the head of a timeline.
func projectDescribe(
	ctx context.Context, svc service.Service, projectID string, maxTokens int,
) (*toolOutput, error) {
	res, err := svc.ProjectDescribe(ctx, service.ProjectDescribeRequest{ProjectID: projectID, MaxTokens: maxTokens})
	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

// itemArgs are the arguments of project_item: one tool for the items of a production, its brief and its texts.
type itemArgs struct {
	Action    string            `json:"action"`
	ProjectID string            `json:"project_id"`
	ItemID    string            `json:"item_id"`
	Kind      string            `json:"kind"`
	Title     string            `json:"title"`
	Text      *string           `json:"text"`
	Answers   map[string]string `json:"answers"`
	Preset    string            `json:"preset"`
	FPS       fpsArg            `json:"fps"`
	JobID     string            `json:"job_id"`
	Position  *int              `json:"position"`
	// The texts of SDD-019: the revision read or compared from, the one compared to, the kind of a new text, the
	// network of a caption check and the pace of a script estimate.
	Revision       *int   `json:"revision"`
	Until          int    `json:"until"`
	TextKind       string `json:"text_kind"`
	Network        string `json:"network"`
	WordsPerMinute int    `json:"wpm"`
}

// projectItem is the operations on the items of a production behind one tool (SDD-003 section 4.1): add, move,
// rename, trash and restore, the brief and the briefing, and the texts (SDD-019: text, revisions, revert, check).
func projectItem(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[itemArgs](raw)
	if err != nil {
		return nil, err
	}

	if isWritingAction(in.Action) {
		return writingItem(ctx, svc, &in)
	}

	if in.Action == "brief" {
		return briefTool(ctx, svc, in.ProjectID, in.Text)
	}

	if in.Action == "briefing" {
		return briefingTool(ctx, svc, in.ProjectID, in.Answers)
	}

	var res *service.ProjectItemResult

	switch in.Action {
	case "add":
		res, err = svc.ProjectItemAdd(ctx, service.ProjectItemAddRequest{
			ProjectID: in.ProjectID, Kind: in.Kind, Title: in.Title, Position: in.Position, Text: deref(in.Text),
			TextKind: in.TextKind, Preset: in.Preset, FPS: string(in.FPS), JobID: in.JobID,
		})
	case "move":
		if in.Position == nil {
			return nil, missingArg("position", in.Action)
		}

		res, err = svc.ProjectItemMove(ctx, service.ProjectItemMoveRequest{
			ProjectID: in.ProjectID, ItemID: in.ItemID, Position: *in.Position,
		})
	case "rename":
		res, err = svc.ProjectItemRename(ctx, service.ProjectItemRenameRequest{
			ProjectID: in.ProjectID, ItemID: in.ItemID, Title: in.Title,
		})
	case "trash":
		res, err = svc.ProjectItemTrash(
			ctx,
			service.ProjectItemTrashRequest{ProjectID: in.ProjectID, ItemID: in.ItemID},
		)
	case "restore":
		res, err = svc.ProjectItemRestore(
			ctx,
			service.ProjectItemTrashRequest{ProjectID: in.ProjectID, ItemID: in.ItemID},
		)
	default:
		return nil, missingArg("action", "project_item")
	}

	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: res.Text}, nil
}

func missingArg(name, action string) error {
	return &service.Error{
		Code: command.CodeValidation, Path: name, Message: name + " is required for " + action + ".",
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// briefTool is project_item with action brief: with text it writes the brief, without it reads it.
func briefTool(ctx context.Context, svc service.Service, projectID string, text *string) (*toolOutput, error) {
	var (
		res *service.BriefResult
		err error
	)

	if text == nil {
		res, err = svc.BriefGet(ctx, service.BriefGetRequest{ProjectID: projectID})
	} else {
		res, err = svc.BriefSet(ctx, service.BriefSetRequest{ProjectID: projectID, Text: *text})
	}

	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: briefTextOf(res)}, nil
}

// briefTextOf is the text of a brief result: the Markdown in a block that says it is data, or the note that
// there is none.
func briefTextOf(res *service.BriefResult) string {
	if res.ItemID == "" {
		return "the project has no brief; write one with project_item action brief and text"
	}

	return textview.Note + "\n" + untrusted.Block(
		untrusted.SourceProjectText,
		untrusted.Attrs{"project": res.ProjectID},
		res.Text,
		0,
	)
}

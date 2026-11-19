package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/remote"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
	"github.com/AlexGalhardo/stuarty/web/bootstrap"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/vukemtest"
)

const actor = "contract"

// fixedNow keeps every timestamp of every run the same, so that the documents
// can be compared byte for byte.
var fixedNow = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

// step is one call of a scenario: a Service operation and its request.
type step struct {
	Call string `json:"call"`
	// As names another actor: the step goes straight to the Service of the
	// workspace with that actor, whatever the driver, so a scenario can have a
	// second writer (the conflict scenario).
	As   string          `json:"as"`
	Args json.RawMessage `json:"args"`
	// Settles marks a call whose answer depends on how far a job got when it
	// returned, a render on a slow machine: the call must succeed, its answer is
	// not compared between drivers, and a later call of the scenario waits for the
	// end of the job and is compared.
	Settles     bool `json:"settles"`
	ExpectError *struct {
		Code string `json:"code"`
	} `json:"expect_error"`
}

// driver reaches the Service in one way. Do returns the result document (the
// JSON of the result struct) and, for a failure, the error document.
type driver interface {
	name() string
	do(t *testing.T, s step) (result json.RawMessage, text string, fail *service.ErrorDocument)
	// service is the Service of the workspace this driver wrote, read directly.
	service() service.Service
	env() *testenv.Env
}

func newEnv(t *testing.T, o testenv.Options) *testenv.Env {
	t.Helper()

	o.Clock = func() time.Time { return fixedNow }

	return testenv.New(t, o)
}

func callerCtx() context.Context { return testenv.Caller("local:" + actor) }

// invoke decodes the request of a step, calls the method and returns the result.
func invoke[Req, Res any](ctx context.Context, s step, fn func(context.Context, Req) (*Res, error)) (any, error) {
	var req Req
	if err := json.Unmarshal(s.Args, &req); err != nil {
		return nil, fmt.Errorf("step %s: %w", s.Call, err)
	}

	res, err := fn(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// callService is the table of the Service operations of the Prototype that the
// scenarios use. TestDriversCoverService fails when the Service has one the
// table does not.
func callService(ctx context.Context, svc service.Service, s step) (any, error) {
	switch s.Call {
	case "ProjectList":
		return invoke(ctx, s, svc.ProjectList)
	case "ProjectCreate":
		return invoke(ctx, s, svc.ProjectCreate)
	case "ProjectGet":
		return invoke(ctx, s, svc.ProjectGet)
	case "Edit":
		return invoke(ctx, s, svc.Edit)
	case "HistoryList":
		return invoke(ctx, s, svc.HistoryList)
	case "Undo":
		return invoke(ctx, s, svc.Undo)
	case "Redo":
		return invoke(ctx, s, svc.Redo)
	case "Lint":
		return invoke(ctx, s, svc.Lint)
	case "MediaImport":
		return invoke(ctx, s, svc.MediaImport)
	case "MediaList":
		return invoke(ctx, s, svc.MediaList)
	case "AssetList":
		return invoke(ctx, s, svc.AssetList)
	case "AssetGet":
		return invoke(ctx, s, svc.AssetGet)
	case "AssetUses":
		return invoke(ctx, s, svc.AssetUses)
	case "AssetSetInfo":
		return invoke(ctx, s, svc.AssetSetInfo)
	case "AssetTag":
		return invoke(ctx, s, svc.AssetTag)
	case "MediaInspect":
		return invoke(ctx, s, svc.MediaInspect)
	case "JobStart":
		return invoke(ctx, s, svc.JobStart)
	case "JobGet":
		return invoke(ctx, s, svc.JobGet)
	case "JobList":
		return invoke(ctx, s, svc.JobList)
	case "TemplateList":
		return invoke(ctx, s, svc.TemplateList)
	case "TemplateSave":
		return invoke(ctx, s, svc.TemplateSave)
	case "TemplateInstantiate":
		return invoke(ctx, s, svc.TemplateInstantiate)
	case "TemplateExport":
		return invoke(ctx, s, svc.TemplateExport)
	case "TemplateImport":
		return invoke(ctx, s, svc.TemplateImport)
	case "ProjectTrash":
		return invoke(ctx, s, svc.ProjectTrash)
	case "ProjectRestore":
		return invoke(ctx, s, svc.ProjectRestore)
	case "CommandHelp":
		return invoke(ctx, s, svc.CommandHelp)
	case "ExportPresetList":
		return invoke(ctx, s, svc.ExportPresetList)
	case "ExportCheck":
		return invoke(ctx, s, svc.ExportCheck)
	case "ExportBatch":
		return invoke(ctx, s, svc.ExportBatch)
	case "MediaTrash":
		return invoke(ctx, s, svc.MediaTrash)
	case "MediaRestore":
		return invoke(ctx, s, svc.MediaRestore)
	case "JobCancel":
		return invoke(ctx, s, svc.JobCancel)
	case "ViewFrame":
		return invoke(ctx, s, svc.ViewFrame)
	case "ViewSheet":
		return invoke(ctx, s, svc.ViewSheet)
	case "Doctor":
		return invoke(ctx, s, svc.Doctor)
	case "FontList":
		return invoke(ctx, s, svc.FontList)
	case "FontImport":
		return invoke(ctx, s, svc.FontImport)
	case "BrandKitGet":
		return invoke(ctx, s, svc.BrandKitGet)
	case "BrandKitSet":
		return invoke(ctx, s, svc.BrandKitSet)
	case "ConnectionList":
		return invoke(ctx, s, svc.ConnectionList)
	case "ConnectionGet":
		return invoke(ctx, s, svc.ConnectionGet)
	case "ConnectionCreate":
		return invoke(ctx, s, svc.ConnectionCreate)
	case "ConnectionRemove":
		return invoke(ctx, s, svc.ConnectionRemove)
	case "ConnectionTest":
		return invoke(ctx, s, svc.ConnectionTest)
	case "SocialNetworkList":
		return invoke(ctx, s, svc.SocialNetworkList)
	case "SocialConnectBegin":
		return invoke(ctx, s, svc.SocialConnectBegin)
	case "SocialAccountList":
		return invoke(ctx, s, svc.SocialAccountList)
	case "PublishValidate":
		return invoke(ctx, s, svc.PublishValidate)
	case "PublishNow":
		return invoke(ctx, s, svc.PublishNow)
	case "PublishStatus":
		return invoke(ctx, s, svc.PublishStatus)
	case "ApprovalList":
		return invoke(ctx, s, svc.ApprovalList)
	case "ApprovalGet":
		return invoke(ctx, s, svc.ApprovalGet)
	case "ApprovalDecide":
		return invoke(ctx, s, svc.ApprovalDecide)
	case "ApprovalRuleList":
		return invoke(ctx, s, svc.ApprovalRuleList)
	case "ApprovalRuleSet":
		return invoke(ctx, s, svc.ApprovalRuleSet)
	case "ApprovalRuleRemove":
		return invoke(ctx, s, svc.ApprovalRuleRemove)
	case "CostSummary":
		return invoke(ctx, s, svc.CostSummary)
	case "CostCeilingSet":
		return invoke(ctx, s, svc.CostCeilingSet)
	case "ProviderList":
		return invoke(ctx, s, svc.ProviderList)
	case "ProviderModels":
		return invoke(ctx, s, svc.ProviderModels)
	case "ProviderPriceSet":
		return invoke(ctx, s, svc.ProviderPriceSet)
	case "TextGenerate":
		return invoke(ctx, s, svc.TextGenerate)
	case "AuditList":
		return invoke(ctx, s, svc.AuditList)
	case "ToolsInstall":
		return invoke(ctx, s, svc.ToolsInstall)
	case "ProjectBundleExport":
		return invoke(ctx, s, svc.ProjectBundleExport)
	case "ProjectBundleImport":
		return invoke(ctx, s, svc.ProjectBundleImport)
	case "ProjectOtioExport":
		return invoke(ctx, s, svc.ProjectOtioExport)
	case "ImageCreate":
		return invoke(ctx, s, svc.ImageCreate)
	case "ImageList":
		return invoke(ctx, s, svc.ImageList)
	case "ImageDescribe":
		return invoke(ctx, s, svc.ImageDescribe)
	case "ImageEdit":
		return invoke(ctx, s, svc.ImageEdit)
	case "ImageUndo":
		return invoke(ctx, s, svc.ImageUndo)
	case "ImageRedo":
		return invoke(ctx, s, svc.ImageRedo)
	case "ImageExport":
		return invoke(ctx, s, svc.ImageExport)
	case "ProjectOtioImport":
		return invoke(ctx, s, svc.ProjectOtioImport)
	case "ProjectDescribe":
		return invoke(ctx, s, svc.ProjectDescribe)
	case "ProjectDuplicate":
		return invoke(ctx, s, svc.ProjectDuplicate)
	case "ProjectItemAdd":
		return invoke(ctx, s, svc.ProjectItemAdd)
	case "ProjectItemMove":
		return invoke(ctx, s, svc.ProjectItemMove)
	case "ProjectItemRename":
		return invoke(ctx, s, svc.ProjectItemRename)
	case "ProjectItemTrash":
		return invoke(ctx, s, svc.ProjectItemTrash)
	case "ProjectItemRestore":
		return invoke(ctx, s, svc.ProjectItemRestore)
	case "BriefGet":
		return invoke(ctx, s, svc.BriefGet)
	case "BriefSet":
		return invoke(ctx, s, svc.BriefSet)
	case "BriefingQuestions":
		return invoke(ctx, s, svc.BriefingQuestions)
	case "BriefingAnswer":
		return invoke(ctx, s, svc.BriefingAnswer)
	}

	if res, ok, err := libraryCall(ctx, svc, s); ok {
		return res, err
	}

	return nil, fmt.Errorf("no mapping for %s", s.Call)
}

// serviceDriver runs the steps through a service.Service: the in-process one
// (the oracle) or the HTTP client in front of a server.
type serviceDriver struct {
	label string
	e     *testenv.Env
	svc   service.Service
}

func (d *serviceDriver) name() string             { return d.label }
func (d *serviceDriver) service() service.Service { return d.e.Service }
func (d *serviceDriver) env() *testenv.Env        { return d.e }

func (d *serviceDriver) do(t *testing.T, s step) (json.RawMessage, string, *service.ErrorDocument) {
	t.Helper()

	res, err := callService(callerCtx(), d.svc, s)
	if err != nil {
		if se, ok := errors.AsType[*service.Error](err); ok {
			doc := se.Document()

			return nil, "", &doc
		}

		t.Fatalf("%s %s: %v", d.label, s.Call, err)
	}

	return encode(t, res), textOf(res), nil
}

func encode(t *testing.T, v any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

// textOf is the text member of a result, which every read result has.
func textOf(res any) string {
	raw, err := json.Marshal(res)
	if err != nil {
		return ""
	}

	var m struct {
		Text string `json:"text"`
	}

	_ = json.Unmarshal(raw, &m)

	return m.Text
}

// httpDriver is the HTTP client in front of a server, which also says which
// routes the server has.
type httpDriver struct {
	serviceDriver
	url   string
	token string
	send  func(*http.Request) (*http.Response, error)
}

func newDirect(t *testing.T) driver {
	t.Helper()

	return newDirectWith(t, testenv.Options{})
}

func newDirectWith(t *testing.T, o testenv.Options) driver {
	t.Helper()

	e := newEnv(t, o)

	return &serviceDriver{label: "in-process", e: e, svc: e.Service}
}

// newHTTP serves the API of the environment on a test server and returns the
// driver that reaches it with the HTTP client of the CLI.
func newHTTP(t *testing.T) driver {
	t.Helper()

	return newHTTPWith(t, testenv.Options{})
}

func newHTTPWith(t *testing.T, o testenv.Options) driver {
	t.Helper()

	e := newEnv(t, o)

	const token = "vvl_c0ntr4ct00000000000000000000000000000000000000000000000000000000"

	now := func() time.Time { return fixedNow }

	deps := &httpapi.Deps{
		Service: e.Service, Registry: command.Default(), Workspace: e.Dir, Token: token,
		Sessions: auth.NewSessions(now), Launch: auth.NewLaunch(now), Now: now,
	}

	app := vukemtest.New(t, func(opts ...vukem.Option) (*vukem.App, error) { return bootstrap.NewWith(deps, opts...) })

	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)

	base := serviceDriver{label: "http", e: e, svc: remote.New(srv.URL+"/", token, srv.Client())}

	return &httpDriver{serviceDriver: base, url: srv.URL, token: token, send: srv.Client().Do}
}

// mcpDriver calls the tools of the MCP server over an in-memory session.
type mcpDriver struct {
	e  *testenv.Env
	cs *sdk.ClientSession
}

func (d *mcpDriver) name() string             { return "mcp" }
func (d *mcpDriver) service() service.Service { return d.e.Service }
func (d *mcpDriver) env() *testenv.Env        { return d.e }

func newMCP(t *testing.T) driver {
	t.Helper()

	return newMCPWith(t, testenv.Options{})
}

func newMCPWith(t *testing.T, o testenv.Options) driver {
	t.Helper()

	e := newEnv(t, o)

	srv, err := mcp.New(e.Service, mcp.Options{})
	if err != nil {
		t.Fatal(err)
	}

	ct, st := sdk.NewInMemoryTransports()

	ss, err := srv.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = ss.Close() })

	cs, err := sdk.NewClient(&sdk.Implementation{Name: actor, Version: "1"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = cs.Close() })

	return &mcpDriver{e: e, cs: cs}
}

// toolCall is how a Service operation is encoded as a tool call (SDD-003 7.1).
func toolCall(s step) (tool string, args map[string]any, err error) {
	args = map[string]any{}
	if len(s.Args) > 0 {
		if err := json.Unmarshal(s.Args, &args); err != nil {
			return "", nil, err
		}
	}

	switch s.Call {
	case "AssetList", "AssetGet", "AssetUses", "AssetSetInfo", "AssetTag":
		return assetToolCall(s.Call, args)
	case "ImageCreate", "ImageList", "ImageDescribe", "ImageEdit", "ImageUndo", "ImageRedo", "ImageExport":
		args["action"] = imageActions[s.Call]

		// the tool names the template to start from from_template, which is not an id member
		if v, ok := args["template_id"]; ok {
			delete(args, "template_id")

			args["from_template"] = v
		}

		return "image", args, nil
	case "ProjectList":
		return "project_list", args, nil
	case "ProjectCreate":
		delete(args, "dry_run")

		return "project_create", args, nil
	case "ProjectGet":
		return "project_get", args, nil
	case "Edit":
		return "edit", args, nil
	case "HistoryList":
		args["action"] = "list"

		return "history", args, nil
	case "Undo", "Redo":
		args["action"] = strings.ToLower(s.Call)

		return "history", args, nil
	case "Lint":
		return "lint", args, nil
	case "MediaImport":
		files, _ := args["files"].([]any)
		paths, ids := make([]any, 0, len(files)), make([]any, 0, len(files))

		for _, f := range files {
			m, _ := f.(map[string]any)
			if h, ok := m["hash"]; ok {
				paths = append(paths, h)
			} else {
				paths = append(paths, m["path"])
			}

			if id, ok := m["asset_id"]; ok {
				ids = append(ids, id)
			}
		}

		delete(args, "files")
		args["paths"] = paths

		if len(ids) > 0 {
			args["asset_ids"] = ids
		}

		return "media_import", args, nil
	case "FontImport":
		return "font_import", args, nil
	case "ConnectionList", "ConnectionGet":
		return "connection", args, nil
	case "MediaList":
		return "media_inspect", args, nil
	case "MediaInspect":
		return "media_inspect", args, nil
	case "JobStart":
		return "job_start", args, nil
	case "JobGet", "JobList":
		return "job_get", args, nil
	case "TemplateList":
		args["action"] = "list"

		return "template", args, nil
	case "TemplateSave":
		delete(args, "dry_run")

		args["action"] = "save"

		return "template", args, nil
	case "TemplateInstantiate":
		delete(args, "dry_run")

		args["action"] = "new"

		return "template", args, nil
	case "TemplateExport":
		args["action"] = "export"

		return "template", args, nil
	case "TemplateImport":
		delete(args, "dry_run")

		args["action"] = "import"

		return "template", args, nil
	case "ProjectTrash", "ProjectRestore":
		delete(args, "dry_run")

		if s.Call == "ProjectRestore" {
			args["restore"] = true
		}

		return "project_trash", args, nil
	case "CommandHelp":
		return "command_help", args, nil
	case "ExportPresetList":
		args["domain"] = "export"
		delete(args, "codecs")

		return "command_help", args, nil
	case "ExportCheck":
		return "lint", args, nil
	case "ExportBatch":
		items := args["items"]

		delete(args, "items")

		args["kind"] = service.JobExportBatch
		args["args"] = map[string]any{"items": items}

		return "job_start", args, nil
	case "JobCancel":
		return "job_cancel", args, nil
	case "TextGenerate":
		// a generation starts with job_start; the model and the prompt are its arguments
		delete(args, "dry_run")

		inner := map[string]any{}

		for _, k := range []string{"connection_id", "model", "prompt", "system", "max_tokens", "temperature"} {
			if v, ok := args[k]; ok {
				inner[k] = v

				delete(args, k)
			}
		}

		args["kind"], args["args"] = "text.generate", inner

		return "job_start", args, nil
	case "ViewFrame":
		args["mode"] = "frame"

		return "view", args, nil
	case "ViewSheet":
		args["mode"] = "sheet"

		return "view", args, nil
	case "ProjectDescribe":
		args["detail"] = "overview"

		return "project_get", args, nil
	case "ProjectDuplicate":
		delete(args, "dry_run")

		args["from"] = args["project_id"]
		delete(args, "project_id")

		return "project_create", args, nil
	case "ProjectItemAdd", "ProjectItemMove", "ProjectItemRename", "ProjectItemTrash", "ProjectItemRestore":
		delete(args, "dry_run")

		args["action"] = strings.ToLower(strings.TrimPrefix(s.Call, "ProjectItem"))

		return "project_item", args, nil
	case "BriefGet":
		args["action"] = "brief"

		return "project_item", args, nil
	case "BriefSet":
		delete(args, "dry_run")

		args["action"] = "brief"

		return "project_item", args, nil
	case "BriefingQuestions":
		args["action"] = "briefing"

		return "project_item", args, nil
	case "BriefingAnswer":
		delete(args, "dry_run")
		delete(args, "replace")

		args["action"] = "briefing"

		return "project_item", args, nil
	}

	if tool, args, ok, err := libraryTool(s); ok {
		return tool, args, err
	}

	return "", nil, fmt.Errorf("no tool for %s", s.Call)
}

func (d *mcpDriver) do(t *testing.T, s step) (json.RawMessage, string, *service.ErrorDocument) {
	t.Helper()

	tool, args, err := toolCall(s)
	if err != nil {
		t.Fatal(err)
	}

	res, err := d.cs.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("mcp %s: %v", tool, err)
	}

	if len(res.Content) != 1 {
		t.Fatalf("mcp %s: %d content blocks", tool, len(res.Content))
	}

	tc, _ := res.Content[0].(*sdk.TextContent)
	if tc == nil {
		t.Fatalf("mcp %s: the content is %T, want text (images come only from view)", tool, res.Content[0])
	}

	// A result that failed as a whole but carries data (media_import with nothing imported, VV-208) has
	// structured content; an error result has none.
	if res.IsError && res.StructuredContent == nil {
		// The last line of an error text is its envelope (SDD-003 section 1.3).
		_, last, _ := strings.Cut(tc.Text, "\n")
		last = last[strings.LastIndex(last, "\n")+1:]

		var doc service.ErrorDocument
		if err := json.Unmarshal([]byte(last), &doc); err != nil {
			t.Fatalf("mcp %s: the error text has no envelope line: %q", tool, tc.Text)
		}

		return nil, "", &doc
	}

	return encode(t, res.StructuredContent), tc.Text, nil
}

// cliDriver runs the commands of the CLI with --json over the Service of an
// environment.
type cliDriver struct {
	e *testenv.Env
}

func (d *cliDriver) name() string             { return "cli" }
func (d *cliDriver) service() service.Service { return d.e.Service }
func (d *cliDriver) env() *testenv.Env        { return d.e }

func newCLI(t *testing.T) driver {
	t.Helper()

	return newCLIWith(t, testenv.Options{})
}

func newCLIWith(t *testing.T, o testenv.Options) driver {
	t.Helper()

	return &cliDriver{e: newEnv(t, o)}
}

type fixedResolver struct{ svc service.Service }

func (r fixedResolver) Resolve(context.Context, cli.Target) (*cli.Connection, error) {
	return &cli.Connection{Service: r.svc, Workspace: "/ws"}, nil
}

// command is how a Service operation is encoded as a command line (SDD-003
// 7.1): the name, the arguments and what goes on standard input.
// command2argv is the command of a step with the --timeline of the calls that take one (SDD-003 section 1.7).
func command2argv(s step) (name string, argv []string, stdin string, err error) {
	name, argv, stdin, err = commandOf(s)
	if err != nil {
		return name, argv, stdin, err
	}

	var t struct {
		TimelineID string `json:"timeline_id"`
	}

	if len(s.Args) > 0 {
		_ = json.Unmarshal(s.Args, &t)
	}

	if t.TimelineID != "" {
		argv = append(argv, "--timeline", t.TimelineID)
	}

	return name, argv, stdin, nil
}

func commandOf(s step) (name string, argv []string, stdin string, err error) {
	var a struct {
		ProjectID string               `json:"project_id"`
		AssetID   string               `json:"asset_id"`
		JobID     string               `json:"job_id"`
		ID        string               `json:"id"`
		Name      string               `json:"name"`
		Preset    string               `json:"preset"`
		FPS       string               `json:"fps"`
		Detail    string               `json:"detail"`
		State     string               `json:"state"`
		Sections  []string             `json:"sections"`
		Kind      string               `json:"kind"`
		Files     []service.ImportFile `json:"files"`
		WaitMS    int                  `json:"wait_ms"`
		Items     []struct {
			Preset string `json:"preset"`
		} `json:"items"`
		Args struct {
			AssetID  string `json:"asset_id"`
			Language string `json:"language"`
			Format   string `json:"format"`
		} `json:"args"`
		BaseRevision *int64 `json:"base_revision"`

		TemplateID  string          `json:"template_id"`
		Description string          `json:"description"`
		Fill        json.RawMessage `json:"fill"`
		Path        string          `json:"path"`
		At          string          `json:"at"`
		Until       string          `json:"until"`
		IncludeLog  *bool           `json:"include_log"`
		AsNew       bool            `json:"as_new"`
		DryRun      bool            `json:"dry_run"`
		BrandColors []string        `json:"brand_colors"`
		BrandFonts  []string        `json:"brand_fonts"`

		ConnectionID string `json:"connection_id"`
		Model        string `json:"model"`
		Prompt       string `json:"prompt"`
		System       string `json:"system"`
		MaxTokens    int    `json:"max_tokens"`
		Provider     string `json:"provider"`
		Currency     string `json:"currency"`
		InMicros     *int64 `json:"input_micros_per_mtok"`
		OutMicros    *int64 `json:"output_micros_per_mtok"`
		Refresh      bool   `json:"refresh"`

		ItemID   string `json:"item_id"`
		Title    string `json:"title"`
		Text     string `json:"text"`
		Position *int   `json:"position"`

		Answers map[string]string `json:"answers"`
		Replace bool              `json:"replace"`
	}

	if len(s.Args) > 0 {
		if err := json.Unmarshal(s.Args, &a); err != nil {
			return "", nil, "", err
		}
	}

	proj := []string{"--project", a.ProjectID}
	flag := func(k, v string) []string {
		if v == "" {
			return nil
		}

		return []string{"--" + k, v}
	}

	switch s.Call {
	case "AssetList", "AssetGet", "AssetUses", "AssetSetInfo", "AssetTag":
		name, argv, err = assetArgv(s.Call, s.Args)

		return name, argv, "", err
	case "ImageCreate", "ImageList", "ImageDescribe", "ImageEdit", "ImageUndo", "ImageRedo", "ImageExport":
		return imageArgv(s)
	case "ProjectList":
		return "project:list", nil, "", nil
	case "ProjectCreate":
		argv = append(argv, flag("name", a.Name)...)
		argv = append(argv, flag("preset", a.Preset)...)
		argv = append(argv, flag("fps", a.FPS)...)

		return "project:create", append(argv, flag("id", a.ID)...), "", nil
	case "ProjectGet":
		return "project:show", append(proj, flag("detail", a.Detail)...), "", nil
	case "Edit":
		return "edit", proj, withoutProject(s.Args), nil
	case "HistoryList":
		return "history", proj, "", nil
	case "Undo":
		return "undo", proj, "", nil
	case "Redo":
		return "redo", proj, "", nil
	case "Lint":
		return "lint", proj, "", nil
	case "MediaImport":
		for _, f := range a.Files {
			if f.Path != "" {
				argv = append(argv, f.Path)
			}
		}

		for _, f := range a.Files {
			argv = append(argv, flag("hash", f.Hash)...)
		}

		for _, f := range a.Files {
			argv = append(argv, flag("asset-id", f.AssetID)...)
		}

		argv = append(argv, importFlags(s.Args)...)

		return "media:import", append(argv, proj...), "", nil
	case "MediaList":
		return "media:list", proj, "", nil
	case "MediaInspect":
		argv = append([]string{a.AssetID}, proj...)

		return "media:inspect", append(argv, flag("sections", strings.Join(a.Sections, ","))...), "", nil
	case "JobStart":
		if a.Kind == service.JobTranscribe {
			argv = append(slices.Clone(proj), "--asset", a.Args.AssetID)
			argv = append(argv, flag("language", a.Args.Language)...)

			if a.WaitMS == 0 {
				return "transcribe", append(argv, "--no-wait"), "", nil
			}

			return "transcribe", append(argv, "--wait"), "", nil
		}

		argv = append(slices.Clone(proj), flag("format", a.Args.Format)...)

		if a.WaitMS == 0 {
			return "export", append(argv, "--no-wait"), "", nil
		}

		return "export", argv, "", nil
	case "JobGet":
		return "job:show", []string{a.JobID, "--wait"}, "", nil
	case "JobList":
		return "job:list", append(proj, flag("state", a.State)...), "", nil
	case "TemplateList":
		return "template:list", nil, "", nil
	case "TemplateSave":
		argv = append(slices.Clone(proj), flag("name", a.Name)...)

		return "template:save", append(argv, flag("description", a.Description)...), "", nil
	case "TemplateExport":
		return "template:export", []string{a.TemplateID}, "", nil
	case "TemplateImport":
		return "template:import", []string{a.Path}, "", nil
	case "ProjectBundleExport":
		argv = slices.Clone(proj)
		if a.IncludeLog != nil && !*a.IncludeLog {
			argv = append(argv, "--no-log")
		}

		if a.WaitMS == 0 {
			return "bundle:export", append(argv, "--no-wait"), "", nil
		}

		return "bundle:export", append(argv, "--wait"), "", nil
	case "ProjectBundleImport":
		argv = []string{a.Path}
		if a.AsNew {
			argv = append(argv, "--as-new")
		}

		if a.DryRun {
			argv = append(argv, "--dry-run")
		}

		return "bundle:import", argv, "", nil
	case "ProjectOtioExport":
		return "otio:export", append(slices.Clone(proj), flag("out", a.Path)...), "", nil
	case "ProjectOtioImport":
		argv = append([]string{a.Path}, proj...)
		if a.BaseRevision != nil {
			argv = append(argv, "--base-revision", strconv.FormatInt(*a.BaseRevision, 10))
		}

		if a.DryRun {
			argv = append(argv, "--dry-run")
		}

		return "otio:import", argv, "", nil
	case "ProjectTrash":
		return "project:trash", []string{a.ProjectID}, "", nil
	case "ProjectRestore":
		return "project:restore", []string{a.ProjectID}, "", nil
	case "CommandHelp":
		return "command:help", []string{a.Name}, "", nil
	case "ExportPresetList":
		return "export:presets", nil, "", nil
	case "ExportCheck":
		return "export:check", append(slices.Clone(proj), flag("preset", a.Preset)...), "", nil
	case "ExportBatch":
		argv = slices.Clone(proj)
		for _, it := range a.Items {
			argv = append(argv, flag("preset", it.Preset)...)
		}

		if a.WaitMS == 0 {
			return "export:batch", append(argv, "--no-wait"), "", nil
		}

		return "export:batch", argv, "", nil
	case "MediaTrash":
		return "media:trash", append([]string{a.AssetID}, proj...), "", nil
	case "MediaRestore":
		return "media:restore", append([]string{a.AssetID}, proj...), "", nil
	case "JobCancel":
		return "job:cancel", []string{a.JobID}, "", nil
	case "ViewFrame":
		return "view", append(slices.Clone(proj), flag("at", a.At)...), "", nil
	case "ViewSheet":
		argv = append(slices.Clone(proj), "--mode", "sheet")

		return "view", append(argv, flag("until", a.Until)...), "", nil
	case "Doctor":
		return "doctor", []string{"--full"}, "", nil
	case "FontList":
		return "fonts", nil, "", nil
	case "FontImport":
		return "font:import", []string{a.Path}, "", nil
	case "BrandKitGet":
		return "brand:show", nil, "", nil
	case "BrandKitSet":
		return "brand:set", []string{"--kit", "-"}, string(s.Args), nil
	case "ConnectionList":
		return "connection:list", nil, "", nil
	case "ConnectionGet":
		return "connection:show", []string{a.ID}, "", nil
	case "ConnectionRemove":
		return "connection:remove", []string{a.ID}, "", nil
	case "ConnectionTest":
		return "connection:test", []string{a.ID}, "", nil
	case "ConnectionCreate":
		var c service.ConnectionCreateRequest
		if err := json.Unmarshal(s.Args, &c); err != nil {
			return "", nil, "", err
		}

		return "connection:add", []string{
			"--kind", c.Kind, "--label", c.Label, "--endpoint", c.Endpoint, "--accepts", strings.Join(c.Accepts, ","),
			"--hosts", strings.Join(c.Hosts, ","),
		}, c.Secret, nil
	case "SocialNetworkList":
		return "social:network:list", nil, "", nil
	case "SocialAccountList":
		return "social:account:list", nil, "", nil
	case "SocialConnectBegin":
		var r service.SocialConnectBeginRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		return "social:connect", slices.Concat(
			[]string{"--network", r.Network, "--redirect-uri", r.RedirectURI}, flag("client-id", r.ClientID),
			flag("label", r.Label), flag("connection", r.ConnectionID),
		), r.ClientSecret, nil
	case "PublishValidate", "PublishNow":
		var r service.PublishNowRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		name := "publish:validate"
		if s.Call == "PublishNow" {
			name = "publish:now"
		}

		argv := slices.Concat(proj, flag("connection", r.ConnectionID), flag("export", r.Export),
			flag("thumbnail", r.Thumbnail), flag("title", r.Title), flag("description", r.Description),
			flag("tags", strings.Join(r.Tags, ",")), flag("visibility", r.Visibility))
		if r.MadeForKids != nil {
			argv = append(argv, "--made-for-kids", map[bool]string{true: "yes", false: "no"}[*r.MadeForKids])
		}

		return name, argv, "", nil
	case "PublishStatus":
		return "publish:status", []string{a.JobID}, "", nil
	case "ApprovalList":
		return "approval:list", slices.Concat(flag("state", a.State), flag("project", a.ProjectID)), "", nil
	case "ApprovalGet":
		return "approval:show", []string{a.ID}, "", nil
	case "ApprovalDecide":
		var d service.ApprovalDecideRequest
		if err := json.Unmarshal(s.Args, &d); err != nil {
			return "", nil, "", err
		}

		return "approval:decide", slices.Concat([]string{d.ID, "--" + d.Decision}, flag("reason", d.Reason)), "", nil
	case "ApprovalRuleList":
		return "approval:rule:list", nil, "", nil
	case "ApprovalRuleSet":
		var r service.ApprovalRuleSetRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		return "approval:rule:set", slices.Concat(
			[]string{"--kind", r.Kind}, flag("id", r.ID), flag("connection", r.ConnectionID),
			flag("project", r.ProjectID), flag("currency", r.Currency),
		), "", nil
	case "ApprovalRuleRemove":
		return "approval:rule:remove", []string{a.ID}, "", nil
	case "CostSummary":
		return "cost:show", nil, "", nil
	case "CostCeilingSet":
		var c service.CostCeilingSetRequest
		if err := json.Unmarshal(s.Args, &c); err != nil {
			return "", nil, "", err
		}

		return "cost:ceiling", slices.Concat([]string{"--currency", c.Currency}, flag("project", c.ProjectID)), "", nil
	case "ProviderList":
		return "provider:list", nil, "", nil
	case "ProviderModels":
		argv = []string{a.ConnectionID}
		if a.Refresh {
			argv = append(argv, "--refresh")
		}

		return "provider:models", argv, "", nil
	case "ProviderPriceSet":
		argv = slices.Concat(
			[]string{"--provider", a.Provider, "--model", a.Model}, flag("currency", a.Currency),
		)

		if a.InMicros != nil {
			argv = append(argv, "--price-in", strconv.FormatInt(*a.InMicros, 10))
		}

		if a.OutMicros != nil {
			argv = append(argv, "--price-out", strconv.FormatInt(*a.OutMicros, 10))
		}

		return "provider:price", argv, "", nil
	case "TextGenerate":
		argv = slices.Concat(
			proj, []string{"--connection", a.ConnectionID, "--model", a.Model, "--prompt", a.Prompt},
			flag("system", a.System),
		)

		if a.MaxTokens > 0 {
			argv = append(argv, "--max-tokens", strconv.Itoa(a.MaxTokens))
		}

		if a.DryRun {
			argv = append(argv, "--dry-run")
		}

		return "text:generate", argv, "", nil
	case "AuditList":
		return "audit", flag("project", a.ProjectID), "", nil
	case "ToolsInstall":
		return "tools:install", append([]string{a.Name}, "--yes"), "", nil
	case "ProjectDescribe":
		return "project:describe", []string{a.ProjectID}, "", nil
	case "ProjectDuplicate":
		return "project:duplicate", append([]string{a.ProjectID}, flag("name", a.Name)...), "", nil
	case "ProjectItemAdd":
		argv = append(slices.Clone(proj), "--kind", a.Kind, "--title", a.Title)
		argv = append(argv, flag("preset", a.Preset)...)

		if a.Position != nil {
			argv = append(argv, "--position", strconv.Itoa(*a.Position))
		}

		if a.Text != "" {
			return "item:add", append(argv, "--text", "-"), a.Text, nil
		}

		return "item:add", argv, "", nil
	case "ProjectItemMove":
		position := 0
		if a.Position != nil {
			position = *a.Position
		}

		return "item:move", append([]string{a.ItemID, "--position", strconv.Itoa(position)}, proj...), "", nil
	case "ProjectItemRename":
		return "item:rename", append([]string{a.ItemID, "--title", a.Title}, proj...), "", nil
	case "ProjectItemTrash":
		return "item:trash", append([]string{a.ItemID}, proj...), "", nil
	case "ProjectItemRestore":
		return "item:restore", append([]string{a.ItemID}, proj...), "", nil
	case "BriefGet":
		return "brief:show", proj, "", nil
	case "BriefSet":
		return "brief:set", append(slices.Clone(proj), "--text", "-"), a.Text, nil
	case "BriefingQuestions":
		return "briefing:questions", flag("project", a.ProjectID), "", nil
	case "BriefingAnswer":
		raw, err := json.Marshal(a.Answers)
		if err != nil {
			return "", nil, "", err
		}

		argv = append(slices.Clone(proj), "--answers", "-")
		if a.Replace {
			argv = append(argv, "--replace")
		}

		return "briefing:answer", argv, string(raw), nil
	case "TemplateInstantiate":
		argv = append([]string{a.TemplateID}, flag("name", a.Name)...)
		argv = append(argv, flag("id", a.ID)...)

		for _, c := range a.BrandColors {
			argv = append(argv, "--brand-color", c)
		}

		for _, f := range a.BrandFonts {
			argv = append(argv, "--brand-font", f)
		}

		if len(a.Fill) == 0 {
			return "template:new", argv, "", nil
		}

		return "template:new", append(argv, "--fill", "-"), string(a.Fill), nil
	}

	if name, argv, stdin, ok, err := libraryCommand(s); ok {
		return name, argv, stdin, err
	}

	return "", nil, "", fmt.Errorf("no command for %s", s.Call)
}

func (d *cliDriver) do(t *testing.T, s step) (json.RawMessage, string, *service.ErrorDocument) {
	t.Helper()

	name, argv, stdin, err := command2argv(s)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	rt := &cli.Runtime{
		Stdin: strings.NewReader(stdin), Stdout: &stdout, Stderr: &stderr,
		Getenv: func(string) string { return "" }, Getwd: func() (string, error) { return t.TempDir(), nil },
		Resolver: fixedResolver{svc: d.e.Service},
	}

	argv = withJSONFlags(name, argv)

	code := rt.Execute(t.Context(), name, argv)

	return cliOutcome(t, name, code, stdout.Bytes(), stderr.String())
}

// cliOutcome reads what a command left behind: the exit code, standard output
// (one JSON document) and standard error (empty in --json mode).
func cliOutcome(
	t *testing.T, name string, code int, stdout []byte, stderr string,
) (json.RawMessage, string, *service.ErrorDocument) {
	t.Helper()

	if stderr != "" {
		t.Fatalf("cli %s: standard error is not empty in --json mode: %q", name, stderr)
	}

	if code == cli.ExitOK {
		raw := bytes.TrimSpace(stdout)

		var m struct {
			Text string `json:"text"`
		}

		_ = json.Unmarshal(raw, &m)

		return raw, m.Text, nil
	}

	// A job that ended failed or cancelled is a result, not an error: the command
	// prints the job and exits 1 (SDD-003 section 5.1).
	var job struct {
		Code  string `json:"code"`
		State string `json:"state"`
		Text  string `json:"text"`
	}

	if json.Unmarshal(stdout, &job) == nil && job.Code == "" && code == cli.ExitFailure &&
		(job.State == service.JobFailed || job.State == service.JobCancelled) {
		return bytes.TrimSpace(stdout), job.Text, nil
	}

	// An import in which files failed is a result too: the batch prints its entries and
	// exits 1 (VV-004).
	var batch struct {
		Code   string `json:"code"`
		Failed int    `json:"failed"`
		Text   string `json:"text"`
	}

	if json.Unmarshal(stdout, &batch) == nil && batch.Code == "" && code == cli.ExitFailure && batch.Failed > 0 {
		return bytes.TrimSpace(stdout), batch.Text, nil
	}

	var doc service.ErrorDocument
	if err := json.Unmarshal(stdout, &doc); err != nil || doc.Code == "" {
		t.Fatalf("cli %s: exit %d with no error document: %q", name, code, stdout)
	}

	if want := expectedExit(doc); code != want {
		t.Errorf("cli %s: exit %d for %s, want %d", name, code, doc.Code, want)
	}

	return nil, "", &doc
}

// withJSONFlags is what every driver adds to a command line.
func withJSONFlags(name string, argv []string) []string {
	argv = append(argv, "--json", "--actor", actor)
	if name == "edit" {
		argv = append(argv, "--ops", "-")
	}

	return argv
}

func expectedExit(doc service.ErrorDocument) int {
	if doc.Retryable {
		return cli.ExitTransient
	}

	return cli.ExitFailure
}

// withoutProject is the batch object of the edit input form: the project is a
// flag on the command line.
func withoutProject(args json.RawMessage) string {
	var m map[string]json.RawMessage

	_ = json.Unmarshal(args, &m)
	delete(m, "project_id")
	delete(m, "timeline_id")

	raw, err := json.Marshal(m)
	if err != nil {
		return ""
	}

	return string(raw)
}

// imageActions are the actions of the image tool, by the Service operation they stand for.
var imageActions = map[string]string{
	"ImageCreate": "create", "ImageList": "list", "ImageDescribe": "show", "ImageEdit": "edit", "ImageUndo": "undo",
	"ImageRedo": "redo", "ImageExport": "export",
}

// imageArgv encodes an image operation as the command line of the CLI (SDD-021 section 11).
func imageArgv(s step) (name string, argv []string, stdin string, err error) {
	var a map[string]any

	if len(s.Args) > 0 {
		if err := json.Unmarshal(s.Args, &a); err != nil {
			return "", nil, "", err
		}
	}

	str := func(k string) string {
		v, _ := a[k].(string)

		return v
	}

	num := func(k string) string {
		if f, ok := a[k].(float64); ok {
			return strconv.Itoa(int(f))
		}

		return ""
	}

	add := func(flag, v string) {
		if v != "" {
			argv = append(argv, "--"+flag, v)
		}
	}

	switch s.Call {
	case "ImageCreate":
		add("preset", str("preset"))
		add("width", num("width"))
		add("height", num("height"))
		add("name", str("name"))
		add("background", str("background"))
		add("from-template", str("template_id"))
		add("id", str("id"))

		if b, _ := a["template"].(bool); b {
			argv = append(argv, "--template")
		}

		if v, ok := a["values"]; ok {
			raw, merr := json.Marshal(v)
			if merr != nil {
				return "", nil, "", merr
			}

			argv, stdin = append(argv, "--values", "-"), string(raw)
		}

		return "image:create", argv, stdin, nil
	case "ImageList":
		add("templates", str("templates"))

		return "image:list", argv, "", nil
	case "ImageDescribe":
		return "image:show", []string{str("image_id")}, "", nil
	case "ImageEdit":
		return "image:edit", []string{str("image_id"), "--ops", "-"}, withoutKey(s.Args, "image_id"), nil
	case "ImageUndo", "ImageRedo":
		argv = []string{str("image_id")}
		if b, _ := a["dry_run"].(bool); b {
			argv = append(argv, "--dry-run")
		}

		return "image:" + strings.ToLower(strings.TrimPrefix(s.Call, "Image")), argv, "", nil
	}

	argv = []string{str("image_id")}
	add("format", str("format"))
	add("quality", num("quality"))
	add("preset", str("preset"))
	add("width", num("width"))
	add("height", num("height"))

	return "image:export", argv, "", nil
}

// withoutKey is the arguments of a step without one member, as the JSON of the standard input of a batch.
func withoutKey(args json.RawMessage, key string) string {
	var m map[string]json.RawMessage

	_ = json.Unmarshal(args, &m)
	delete(m, key)

	raw, err := json.Marshal(m)
	if err != nil {
		return ""
	}

	return string(raw)
}

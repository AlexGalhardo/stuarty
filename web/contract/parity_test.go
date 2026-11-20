package contract

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
)

// row is one line of the parity table of SDD-003 section 7.1 for an operation of
// the first service. Pending is the task that delivers the operation: until
// then it has no Service method and its tool and command are stubs.
type row struct {
	method  string // the Service method
	tool    string // the MCP tool, empty when the operation has none
	command string // the CLI command
	route   string // "METHOD /path" of the HTTP API
	pending string
	// httpOnly marks an operation with no MCP tool and no CLI command, by design
	// (SDD-003 section 7.1, exception E-4): only the HTTP route reaches it.
	httpOnly bool
	// noMethod marks an operation that is not a Service method (SDD-003 section
	// 7.1, exception E-1: the event stream lives in the hub of the owner).
	noMethod bool
}

var parity = []row{
	{method: "ProjectList", tool: "project_list", command: "project:list", route: "GET /projects"},
	{method: "ProjectCreate", tool: "project_create", command: "project:create", route: "POST /projects"},
	{method: "ProjectGet", tool: "project_get", command: "project:show", route: "GET /projects/{}"},
	{method: "ProjectTrash", tool: "project_trash", command: "project:trash", route: "POST /projects/{}/trash"},
	{method: "ProjectRestore", tool: "project_trash", command: "project:restore", route: "POST /projects/{}/restore"},
	{method: "Edit", tool: "edit", command: "edit", route: "POST /projects/{}/edit"},
	{method: "CommandHelp", tool: "command_help", command: "command:help", route: "GET /commands"},
	{method: "HistoryList", tool: "history", command: "history", route: "GET /projects/{}/history"},
	{method: "Undo", tool: "history", command: "undo", route: "POST /projects/{}/undo"},
	{method: "Redo", tool: "history", command: "redo", route: "POST /projects/{}/redo"},
	{method: "HistoryRevert", tool: "history", command: "revert", route: "POST /projects/{}/revert"},
	{method: "HistoryDiff", tool: "history", command: "diff", route: "GET /projects/{}/diff"},
	{method: "CheckpointCreate", tool: "history", command: "checkpoint:create", route: "POST /projects/{}/checkpoints"},
	{method: "CheckpointList", tool: "history", command: "checkpoint:list", route: "GET /projects/{}/checkpoints"},
	{
		method: "CheckpointRename", tool: "history", command: "checkpoint:rename",
		route: "PATCH /projects/{}/checkpoints/{}",
	},
	{
		method: "CheckpointRemove", tool: "history", command: "checkpoint:remove",
		route: "DELETE /projects/{}/checkpoints/{}",
	},
	{method: "MediaImport", tool: "media_import", command: "media:import", route: "POST /projects/{}/media"},
	{method: "MediaInspect", tool: "media_inspect", command: "media:inspect", route: "GET /projects/{}/media/{}"},
	{method: "MediaList", tool: "media_inspect", command: "media:list", route: "GET /projects/{}/media"},
	{method: "MediaTrash", command: "media:trash", route: "POST /projects/{}/media/{}/trash"},
	{method: "MediaRestore", command: "media:restore", route: "POST /projects/{}/media/{}/restore"},
	{method: "TemplateList", tool: "template", command: "template:list", route: "GET /templates"},
	{method: "TemplateSave", tool: "template", command: "template:save", route: "POST /templates"},
	{
		method: "TemplateInstantiate", tool: "template", command: "template:new",
		route: "POST /templates/{}/instantiate",
	},
	{method: "TemplateExport", tool: "template", command: "template:export", route: "GET /templates/{}/file"},
	{method: "TemplateImport", tool: "template", command: "template:import", route: "POST /templates/import"},
	{method: "Lint", tool: "lint", command: "lint", route: "GET /projects/{}/lint"},
	{method: "JobStart", tool: "job_start", command: "export", route: "POST /projects/{}/jobs"},
	{method: "JobGet", tool: "job_get", command: "job:show", route: "GET /jobs/{}"},
	{method: "JobCancel", tool: "job_cancel", command: "job:cancel", route: "POST /jobs/{}/cancel"},
	{method: "JobList", tool: "job_get", command: "job:list", route: "GET /projects/{}/jobs"},
	{method: "ViewFrame", tool: "view", command: "view", route: "GET /projects/{}/frame"},
	{method: "ViewSheet", tool: "view", command: "view", route: "GET /projects/{}/sheet"},
	{method: "AudioMeasure", tool: "view", command: "view", route: "GET /projects/{}/audio-measure"},
	// The hand-off from an external motion engine (SDD-011 section 15): view with mode motion.
	{method: "MotionBrief", tool: "view", command: "motion:brief", route: "GET /projects/{}/motion/brief"},
	// The check has no tool: the tool list has no room for it (SDD-011 section 15.5).
	{method: "MotionCheck", command: "motion:check", route: "GET /projects/{}/motion/check"},
	// The replacement is a batch of clip.replace_media in one edit call over MCP: it has no tool of its own.
	{method: "MotionReplace", command: "motion:replace", route: "POST /projects/{}/motion/replace"},
	{method: "TrashEmpty", route: "POST /trash/empty", httpOnly: true},
	{method: "Doctor", command: "doctor", route: "GET /system"},
	{method: "SnapPoints", route: "GET /projects/{}/snap-points", httpOnly: true},
	{method: "FontList", command: "fonts", route: "GET /fonts"},
	{method: "AssetList", tool: "media_inspect", command: "asset:list", route: "GET /assets"},
	{method: "AssetGet", tool: "media_inspect", command: "asset:show", route: "GET /assets/{}"},
	{method: "AssetUses", command: "asset:uses", route: "GET /assets/{}/uses"},
	{method: "AssetSetInfo", tool: "media_import", command: "asset:set", route: "PUT /assets/{}"},
	{method: "AssetTag", tool: "media_import", command: "asset:tag", route: "POST /assets/{}/tags"},
	{method: "AuditList", command: "audit", route: "GET /audit"},
	{method: "ExportFile", route: "GET /jobs/{}/file", httpOnly: true},
	// The CLI command tools:install is for the person at the terminal and has
	// no MCP tool (SDD-007 section 6.8, SDD-003 section 7.1).
	{method: "ToolsInstall", command: "tools:install", route: "POST /system/tools/install"},
	{method: "ToolsStatus", route: "GET /system/tools", httpOnly: true},
	// Key management is for a person who logged in, in network mode: no MCP tool
	// and no CLI command (SDD-000 section 1, SDD-007 section 4.2).
	{method: "KeyCreate", route: "POST /keys", httpOnly: true},
	{method: "KeyList", route: "GET /keys", httpOnly: true},
	{method: "KeyRevoke", route: "DELETE /keys/{}", httpOnly: true},
	{method: "Events", command: "events", route: "GET /projects/{}/events", noMethod: true},
	// Bundles move whole projects as files: no MCP tool (SDD-003 section 7.1,
	// exception E-3).
	{method: "ProjectBundleExport", command: "bundle:export", route: "POST /projects/{}/bundle"},
	{method: "ProjectBundleImport", command: "bundle:import", route: "POST /projects/import"},
	// The font import has the MCP tool font_import (VV-024); the brand kit has
	// none (SDD-000 section 1, exception E-3).
	{method: "FontImport", tool: "font_import", command: "font:import", route: "POST /fonts"},
	{method: "BrandKitGet", command: "brand:show", route: "GET /brand-kit"},
	{method: "BrandKitSet", command: "brand:set", route: "PUT /brand-kit"},
	// OpenTimelineIO moves a timeline as a file: no MCP tool (SDD-003 section 7.1, exception E-3).
	{method: "ProjectOtioExport", command: "otio:export", route: "GET /projects/{}/otio"},
	{method: "ProjectOtioImport", command: "otio:import", route: "POST /projects/{}/otio"},
	// The production of a project (SDD-003 section 2.3.1): the description is project_get with detail overview, a
	// copy is project_create with from, and the brief is project_item with action brief.
	{method: "ProjectDescribe", tool: "project_get", command: "project:describe", route: "GET /projects/{}/describe"},
	{
		method:  "ProjectDuplicate",
		tool:    "project_create",
		command: "project:duplicate",
		route:   "POST /projects/{}/duplicate",
	},
	{method: "ProjectItemAdd", tool: "project_item", command: "item:add", route: "POST /projects/{}/items"},
	{method: "ProjectItemMove", tool: "project_item", command: "item:move", route: "POST /projects/{}/items/{}/move"},
	{
		method: "ProjectItemRename", tool: "project_item", command: "item:rename",
		route: "POST /projects/{}/items/{}/rename",
	},
	{
		method:  "ProjectItemTrash",
		tool:    "project_item",
		command: "item:trash",
		route:   "POST /projects/{}/items/{}/trash",
	},
	{
		method: "ProjectItemRestore", tool: "project_item", command: "item:restore",
		route: "POST /projects/{}/items/{}/restore",
	},
	{method: "BriefGet", tool: "project_item", command: "brief:show", route: "GET /projects/{}/brief"},
	{method: "BriefSet", tool: "project_item", command: "brief:set", route: "PUT /projects/{}/brief"},
	// The guided briefing (SDD-005 section 5.12.1): project_item with action briefing.
	{
		method:  "BriefingQuestions",
		tool:    "project_item",
		command: "briefing:questions",
		route:   "GET /briefing/questions",
	},
	{method: "BriefingAnswer", tool: "project_item", command: "briefing:answer", route: "PUT /projects/{}/briefing"},
	// Connections (SDD-014 section 10): the read-only tool lists or reads one; creating, removing and
	// testing one take a credential or reach the network, so they have no tool.
	{method: "ConnectionList", tool: "connection", command: "connection:list", route: "GET /connections"},
	{method: "ConnectionGet", tool: "connection", command: "connection:show", route: "GET /connections/{}"},
	{method: "ConnectionCreate", command: "connection:add", route: "POST /connections"},
	{method: "ConnectionRemove", command: "connection:remove", route: "DELETE /connections/{}"},
	{method: "ConnectionTest", command: "connection:test", route: "POST /connections/{}/test"},
	// The image documents of SDD-021: one MCP tool with an action per operation.
	{method: "ImageCreate", tool: "image", command: "image:create", route: "POST /images"},
	{method: "ImageList", tool: "image", command: "image:list", route: "GET /images"},
	{method: "ImageDescribe", tool: "image", command: "image:show", route: "GET /images/{}"},
	{method: "ImageEdit", tool: "image", command: "image:edit", route: "POST /images/{}/edit"},
	{method: "ImageUndo", tool: "image", command: "image:undo", route: "POST /images/{}/undo"},
	{method: "ImageRedo", tool: "image", command: "image:redo", route: "POST /images/{}/redo"},
	{method: "ImageExport", tool: "image", command: "image:export", route: "POST /images/{}/export"},
	{method: "ImageImport", tool: "image", command: "image:import", route: "POST /images/import"},
	// The export presets, the check against one and the batch (SDD-004 sections 8.2 to 8.4). The MCP tools are the
	// ones that already exist (NFR-020 leaves no room for three more): command_help with domain export, lint with a
	// preset, and job_start with kind export_batch.
	{method: "ExportPresetList", tool: "command_help", command: "export:presets", route: "GET /export-presets"},
	{method: "ExportCheck", tool: "lint", command: "export:check", route: "GET /projects/{}/export-check"},
	{method: "ExportBatch", tool: "job_start", command: "export:batch", route: "POST /projects/{}/export-batches"},
	// The reads of the image editor screen (SDD-012 section 24.3): HTTP only, an agent has the text view.
	{method: "ImageImportFile", route: "POST /images/import/file", httpOnly: true},
	{method: "ImageFrame", route: "GET /images/{}/frame", httpOnly: true},
	{method: "ImageHistory", route: "GET /images/{}/history", httpOnly: true},
	{method: "ImageFile", route: "GET /images/{}/exports/{}", httpOnly: true},
	// The approval queue (SDD-015 section 10): no MCP tool, the budget of the tool list leaves no room
	// for one; deciding, writing a rule and setting a ceiling are for a person, and their commands
	// always fail for an agent.
	{method: "ApprovalList", tool: "job_get", command: "approval:list", route: "GET /approvals"},
	{method: "ApprovalGet", tool: "job_get", command: "approval:show", route: "GET /approvals/{}"},
	{method: "ApprovalDecide", command: "approval:decide", route: "POST /approvals/{}/decide"},
	{method: "ApprovalRuleList", command: "approval:rule:list", route: "GET /approval-rules"},
	{method: "ApprovalRuleSet", command: "approval:rule:set", route: "POST /approval-rules"},
	{method: "ApprovalRuleRemove", command: "approval:rule:remove", route: "DELETE /approval-rules/{}"},
	{method: "CostSummary", command: "cost:show", route: "GET /cost"},
	{method: "CostCeilingSet", command: "cost:ceiling", route: "PUT /cost/ceilings"},
	// Social networks and publishing (SDD-020 section 13): no MCP tool routes to them yet; an agent reaches them
	// through the CLI and HTTP. The sign-in redirect is the browser's: it has no command.
	{method: "SocialNetworkList", command: "social:network:list", route: "GET /social/networks"},
	{method: "SocialConnectBegin", command: "social:connect", route: "POST /social/connect"},
	{method: "SocialConnectComplete", route: "POST /social/complete", httpOnly: true},
	{method: "SocialAccountList", command: "social:account:list", route: "GET /social/accounts"},
	{method: "SocialAccountSelect", route: "POST /social/select", httpOnly: true},
	{method: "PublishValidate", command: "publish:validate", route: "POST /publish/validate"},
	{method: "PublishNow", tool: "job_start", command: "publish:now", route: "POST /publish"},
	{method: "PublishStatus", tool: "job_get", command: "publish:status", route: "GET /publish/{}"},
	// The calendar of publications (SDD-020 section 29): no MCP tool (author decision D-094); an agent plans
	// through the CLI and HTTP, and approval stays with a person.
	{method: "PublicationCreate", command: "publication:create", route: "POST /publications"},
	{method: "PublicationGet", command: "publication:show", route: "GET /publications/{}"},
	{method: "PublicationList", command: "publication:list", route: "GET /publications"},
	{method: "PublicationUpdate", command: "publication:update", route: "PUT /publications/{}"},
	{method: "PublicationSchedule", command: "publication:schedule", route: "POST /publications/{}/schedule"},
	{method: "PublicationSendNow", command: "publication:send-now", route: "POST /publications/{}/send-now"},
	{method: "PublicationRetry", command: "publication:retry", route: "POST /publications/{}/retry"},
	{method: "PublicationCancel", command: "publication:cancel", route: "POST /publications/{}/cancel"},
	// The AI providers (SDD-016 section 10): a generation starts with job_start (kind text.generate), the
	// byte budget of the tool list leaves no room for a tool of its own. Setting a price is for a person.
	{method: "ProviderList", command: "provider:list", route: "GET /providers"},
	{method: "ProviderModels", command: "provider:models", route: "GET /connections/{}/models"},
	{method: "ProviderPriceSet", command: "provider:price", route: "PUT /providers/prices"},
	{method: "TextGenerate", tool: "job_start", command: "text:generate", route: "POST /text/generate"},
	// The texts of a production (SDD-019): no tool of their own, the budget of the tool list leaves no room.
	// project_item has the actions text (list, read, write), revisions, revert and check (caption or script),
	// and job_start the kind text.draft, which a person approves.
	{method: "TextList", tool: "project_item", command: "text:list", route: "GET /texts"},
	{method: "TextGet", tool: "project_item", command: "text:show", route: "GET /projects/{}/texts/{}"},
	{method: "TextWrite", tool: "project_item", command: "text:write", route: "POST /projects/{}/texts"},
	{method: "TextWrite", tool: "project_item", command: "text:write", route: "PUT /projects/{}/texts/{}"},
	{
		method: "TextRevisions", tool: "project_item", command: "text:revisions",
		route: "GET /projects/{}/texts/{}/revisions",
	},
	{method: "TextRevert", tool: "project_item", command: "text:revert", route: "POST /projects/{}/texts/{}/revert"},
	{method: "TextDraft", tool: "job_start", command: "text:draft", route: "POST /projects/{}/texts/{}/draft"},
	{method: "CaptionValidate", tool: "project_item", command: "caption:validate", route: "POST /captions/validate"},
	{method: "ScriptEstimate", tool: "project_item", command: "script:estimate", route: "POST /scripts/estimate"},
	// Image and voice generation (SDD-016 section 6.1, epic 10): job_start again, with the kinds image.generate
	// and voice.generate; the voices of a connection are read with the CLI or HTTP, as its models are.
	{method: "ImageGenerate", tool: "job_start", command: "image:generate", route: "POST /image/generate"},
	{method: "VoiceGenerate", tool: "job_start", command: "voice:generate", route: "POST /voice/generate"},
	// Removing a background and generating into an image document (SDD-016 section 6.5, epic 30): job_start with
	// the kind image.edit.
	{method: "ImageModelEdit", tool: "job_start", command: "image:model-edit", route: "POST /images/{}/model-edit"},
	{method: "VoiceList", command: "voice:list", route: "GET /connections/{}/voices"},
	// Video generation (SDD-016 section 6.4, epic 11): job_start with the kind video.generate.
	{method: "VideoGenerate", tool: "job_start", command: "video:generate", route: "POST /video/generate"},
	// The sign-in to an AI provider (SDD-016 section 12) is made by a person in the browser: no tool, no command.
	{method: "ProviderSignInBegin", route: "POST /providers/signin", httpOnly: true},
	{method: "ProviderSignInComplete", route: "POST /providers/signin/complete", httpOnly: true},
	{method: "ProviderSignInList", route: "GET /providers/sign-ins", httpOnly: true},
	// The library (SDD-018): one MCP tool, library, serves the lists, reads, writes, searches, renders and
	// lints. Removing a note, importing and exporting skills read and write files by path (no tool), and
	// marking a skill as read is for a person at the browser (HTTP only).
	{method: "MemoryList", tool: "library", command: "memory:list", route: "GET /library/memory"},
	{method: "MemoryGet", tool: "library", command: "memory:show", route: "GET /library/memory/{}"},
	{method: "MemoryWrite", tool: "library", command: "memory:write", route: "POST /library/memory"},
	{method: "MemoryRemove", command: "memory:remove", route: "DELETE /library/memory/{}"},
	{method: "SkillList", tool: "library", command: "skill:list", route: "GET /library/skills"},
	{method: "SkillGet", tool: "library", command: "skill:show", route: "GET /library/skills/{}"},
	{method: "SkillWrite", tool: "library", command: "skill:write", route: "PUT /library/skills/{}"},
	{method: "SkillImport", command: "skill:import", route: "POST /library/skills/import"},
	{method: "SkillExport", command: "skill:export", route: "POST /library/skills/export"},
	{method: "SkillMarkReviewed", route: "POST /library/skills/{}/review", httpOnly: true},
	{method: "SkillLint", tool: "library", command: "skill:lint", route: "GET /library/skills/{}/lint"},
	{method: "PromptList", tool: "library", command: "prompt:list", route: "GET /library/prompts"},
	{method: "PromptGet", tool: "library", command: "prompt:show", route: "GET /library/prompts/{}"},
	{method: "PromptWrite", tool: "library", command: "prompt:write", route: "PUT /library/prompts/{}"},
	{method: "PromptRender", tool: "library", command: "prompt:render", route: "POST /library/prompts/{}/render"},
	{method: "LibrarySearch", tool: "library", command: "library:search", route: "GET /library/search"},
	{method: "LibraryReindex", command: "library:reindex", route: "POST /library/reindex"},
	// The hosted agent chat (SDD-017 section 10, SDD-003 exception E-7): no MCP tool, a hosted agent has no use
	// for the chat of another. The reads have a command; the profiles and the five operations that drive a session
	// are for a person in the browser (HTTP only).
	{method: "AgentProfileList", command: "agent:profiles", route: "GET /agents/profiles"},
	{method: "AgentProfileSet", route: "PUT /agents/profiles/{}", httpOnly: true},
	{method: "AgentProfileRemove", route: "DELETE /agents/profiles/{}", httpOnly: true},
	{method: "ChatSessionStart", route: "POST /projects/{}/chat/sessions", httpOnly: true},
	{method: "ChatSend", route: "POST /chat/sessions/{}/messages", httpOnly: true},
	{method: "ChatCancel", route: "POST /chat/sessions/{}/cancel", httpOnly: true},
	{method: "ChatAnswerPermission", route: "POST /chat/sessions/{}/permissions/{}", httpOnly: true},
	{method: "ChatSessionDelete", route: "DELETE /chat/sessions/{}", httpOnly: true},
	{method: "ChatSessionList", command: "chat:sessions", route: "GET /projects/{}/chat/sessions"},
	{method: "ChatSessionGet", command: "chat:session", route: "GET /chat/sessions/{}"},
	{method: "ChatTranscriptGet", command: "chat:transcript", route: "GET /chat/sessions/{}/transcript"},
	// The stream of a session is not a Service operation (SDD-017 section 8).
	{route: "GET /chat/sessions/{}/events", httpOnly: true, noMethod: true},
}

// notOperations are Service methods that are not operations an agent calls
// (SDD-003 section 7.1, exception E-2).
var notOperations = map[string]bool{"OpenFile": true, "PlanWindow": true}

var braces = regexp.MustCompile(`\{[^}]*\}`)

func TestParityTableCoversTheService(t *testing.T) {
	t.Parallel()

	known := map[string]bool{}
	for _, r := range parity {
		known[r.method] = true
	}

	svc := reflect.TypeFor[service.Service]()

	for m := range svc.Methods() {
		if !known[m.Name] && !notOperations[m.Name] {
			t.Errorf(
				"Service.%s is in no row of the parity table: give it a tool, a command and a route, or an exception of SDD-003 section 7.1",
				m.Name,
			)
		}
	}

	for _, r := range parity {
		_, has := svc.MethodByName(r.method)
		if r.noMethod {
			if has {
				t.Errorf("%s is marked as no Service method but the Service has it", r.method)
			}

			continue
		}

		if has == (r.pending != "") {
			t.Errorf("%s: pending=%q but the Service method exists=%v", r.method, r.pending, has)
		}
	}
}

func TestEveryOperationIsReachableOnEverySurface(t *testing.T) {
	d := newHTTP(t).(*httpDriver)

	tools := listedTools(t)
	commands, stubs := cli.CommandNames()
	routes := httpRoutes(t, d)

	for _, r := range parity {
		if r.pending != "" {
			// A pending operation is a declared stub, never a listed tool.
			if r.tool != "" && slices.Contains(tools, r.tool) {
				t.Errorf("%s is pending (%s) but %s is already listed", r.method, r.pending, r.tool)
			}

			if !slices.Contains(stubs, r.command) {
				t.Errorf(
					"%s is pending (%s): the command %s must be a stub that names its task",
					r.method,
					r.pending,
					r.command,
				)
			}

			continue
		}

		if r.httpOnly {
			if !slices.Contains(routes, r.route) {
				t.Errorf("%s: the HTTP route %s is not served", r.method, r.route)
			}

			continue
		}

		if r.tool != "" && !slices.Contains(tools, r.tool) {
			t.Errorf("%s: the MCP tool %s is not in tools/list", r.method, r.tool)
		}

		if !slices.Contains(commands, r.command) || slices.Contains(stubs, r.command) {
			t.Errorf("%s: the CLI command %s is missing or still a stub", r.method, r.command)
		}

		if !slices.Contains(routes, r.route) {
			t.Errorf("%s: the HTTP route %s is not served", r.method, r.route)
		}
	}

	for _, name := range tools {
		if !slices.ContainsFunc(parity, func(r row) bool { return r.tool == name }) {
			t.Errorf("the tool %s is in no row of the parity table", name)
		}
	}
}

func listedTools(t *testing.T) []string {
	t.Helper()

	d := newMCP(t).(*mcpDriver)

	list, err := d.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, 0, len(list.Tools))

	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}

	for name := range mcp.Pending {
		if slices.Contains(names, name) {
			t.Errorf("%s is pending and listed", name)
		}
	}

	return names
}

// httpRoutes are the "METHOD /path" pairs of the OpenAPI document of the server,
// without the base path and with every parameter as {}.
func httpRoutes(t *testing.T, d *httpDriver) []string {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, d.url+"/openapi.json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Authorization", "Bearer "+d.token)

	res, err := d.send(req)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("openapi: %d %.200s", res.StatusCode, raw)
	}

	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}

	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("openapi: %v", err)
	}

	var routes []string

	for path, ops := range doc.Paths {
		path = braces.ReplaceAllString(strings.TrimPrefix(path, "/api/v1"), "{}")

		for method := range ops {
			routes = append(routes, strings.ToUpper(method)+" "+path)
		}
	}

	return routes
}

// TestDriversCoverTheOperationsOfTheScenarios fails when a scenario names an
// operation that a driver cannot encode.
// TestDriversCoverService fails when an operation of the parity table has no
// encoding in a driver that its row says reaches it: the Service table always, the
// MCP tool table when the row has a tool, the CLI table when it has a command.
// Operations without a Service method (the event stream) and the exceptions E-1
// (no tool) are the only ones left out.
func TestDriversCoverService(t *testing.T) {
	t.Parallel()

	for _, r := range parity {
		// An operation only the HTTP route reaches (RF-131) has nothing to encode.
		if r.noMethod || r.httpOnly {
			continue
		}

		s := step{Call: r.method, Args: []byte(`{}`)}

		if !coversService(t, s) {
			t.Errorf("%s has no mapping in callService", r.method)
		}

		if _, _, err := toolCall(s); (err == nil) != (r.tool != "") {
			t.Errorf("%s: the tool %q and the MCP mapping disagree: %v", r.method, r.tool, err)
		}

		if _, _, _, err := command2argv(s); err != nil {
			t.Errorf("%s: no CLI mapping: %v", r.method, err)
		}
	}
}

// coversService reports whether callService knows the operation: an operation it
// does not know returns the "no mapping" error, one it does know calls the nil
// Service and panics.
func coversService(t *testing.T, s step) (known bool) {
	t.Helper()

	defer func() {
		if recover() != nil {
			known = true
		}
	}()

	_, err := callService(context.Background(), nil, s)

	return err == nil || !strings.Contains(err.Error(), "no mapping")
}

func TestDriversCoverTheOperationsOfTheScenarios(t *testing.T) {
	t.Parallel()

	steps, _ := loadScenario(t, "prototype")

	for _, s := range steps {
		if _, _, err := toolCall(s); err != nil {
			t.Errorf("mcp: %v", err)
		}

		if _, _, _, err := command2argv(s); err != nil {
			t.Errorf("cli: %v", err)
		}
	}
}

// TestBudgetsOfTheSurfacesArePrinted measures NFR-020 on the tools/list the
// client receives and prints it; the 200-item view of NFR-021 is measured in
// web/app/mcp (TestConciseViewOfAnItemHeavyProjectFitsTwoPages) and the reads of
// the scenario are printed here, so that a run shows the sizes.
func TestBudgetsOfTheSurfacesArePrinted(t *testing.T) {
	d := newMCP(t).(*mcpDriver)

	list, err := d.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	raw := encode(t, list)

	t.Logf(
		"NFR-020: tools/list is %d bytes, about %d tokens (limit %d bytes)",
		len(raw),
		len(raw)*10/32,
		mcp.MaxToolListBytes,
	)

	if len(raw) > mcp.MaxToolListBytes {
		t.Errorf("tools/list is %d bytes, over the %d of NFR-020 (SDD-003 section 4.1)", len(raw), mcp.MaxToolListBytes)
	}

	steps, fixtures := loadScenario(t, "prototype")

	for i, s := range steps {
		s = withFixtures(t, d, s, fixtures)

		_, text, fail := d.do(t, s)

		if fail != nil || s.Call != "ProjectGet" {
			continue
		}

		t.Logf("NFR-021: step %d project_get concise is %d characters, about %d tokens (limit 4000 a page)",
			i, len(text), len(text)/3)

		if len(text)/3 > 4000 {
			t.Errorf("project_get concise is about %d tokens", len(text)/3)
		}
	}
}

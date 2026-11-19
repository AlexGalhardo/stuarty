package agentdocs

// ToolRow maps an MCP tool to the CLI commands that do the same thing (SDD-003
// section 4.1 and section 8.4 item 11). A test checks every name against the
// tool list of the MCP server and the command list of the CLI.
type ToolRow struct {
	Tool     string
	Commands []string
}

// Tools is the tool table of SDD-003 section 4.1, for the tools in the build.
var Tools = []ToolRow{
	{"project_list", []string{"project:list"}},
	{"project_create", []string{"project:create"}},
	{"project_get", []string{"project:show"}},
	{"project_trash", []string{"project:trash", "project:restore"}},
	{"edit", []string{"edit"}},
	{"command_help", []string{"command:help"}},
	{"history", []string{"history", "undo", "redo"}},
	{"media_import", []string{"media:import"}},
	{"media_inspect", []string{"media:inspect", "media:list"}},
	{"view", []string{"view"}},
	{"lint", []string{"lint"}},
	{"job_start", []string{"export"}},
	{"job_get", []string{"job:show", "job:list"}},
	{"job_cancel", []string{"job:cancel"}},
	{"template", []string{"template:list", "template:save", "template:new", "template:export", "template:import"}},
	{"font_import", []string{"font:import"}},
	{"connection", []string{"connection:list", "connection:show"}},
	{
		"image",
		[]string{"image:create", "image:show", "image:edit", "image:export"},
	},
	{
		"project_item",
		[]string{"item:add", "brief:set"},
	},
	{"library", []string{"library:search"}},
}

// ErrorRow is one error code of SDD-000 section 8 and SDD-002 section A.8, with
// the usual way out. The HTTP status comes from service.Error.HTTPStatus.
type ErrorRow struct {
	Code      string
	Retryable bool
	Fix       string
}

// Errors lists the codes an agent can meet. A test checks that every code of
// the core is here.
var Errors = []ErrorRow{
	{"E_VALIDATION", false, "fix the argument"},
	{"E_INVALID_TIME", false, "add a unit: 2s, 60f"},
	{"E_UNKNOWN_COMMAND", false, "see command:help"},
	{"E_NOT_FOUND", false, "list, then use the id"},
	{"E_ID_EXISTS", false, "use another id, or omit it"},
	{"E_TYPE_MISMATCH", false, "wrong kind of target"},
	{"E_OVERLAP", false, "move it, or use another track"},
	{"E_OUT_OF_SOURCE", false, "stay inside the source"},
	{"E_INVALID_STATE", false, "restore it, or wait"},
	{"E_IN_USE", false, "remove its users first"},
	{"E_NOT_EMPTY", false, "empty it first"},
	{"E_LIMIT", false, "split the batch"},
	{"E_FONT_UNKNOWN", false, "use a listed font"},
	{"E_SLOT", false, "fill every required slot"},
	{"E_REVISION_CONFLICT", true, "read details.diff, re-plan, resend"},
	{"E_IDEMPOTENCY_MISMATCH", false, "use a new key"},
	{"E_INTERNAL", false, "report it as a bug"},
	{"E_UNAUTHORIZED", false, "set the token or key"},
	{"E_FORBIDDEN", false, "use a key with that scope"},
	{"E_WORKSPACE_LOCKED", true, "wait"},
	{"E_PATH_NOT_ALLOWED", false, "use an allowed path"},
	{"E_FILE_NOT_FOUND", false, "check the path"},
	{"E_UNSUPPORTED_MEDIA", false, "convert the file"},
	{"E_INGEST_FAILED", false, "see details.reason"},
	{"E_ASSET_NOT_READY", true, "poll job:show until ready"},
	{"E_FFMPEG_MISSING", false, "install FFmpeg"},
	{"E_NO_ENCODER", false, "use another codec"},
	{"E_EXPORT_FAILED", false, "see details.reason"},
	{"E_VERIFY_FAILED", false, "see details.checks"},
	{"E_TRANSCRIBE_UNAVAILABLE", false, "install the helper"},
	{"E_TRANSCRIBE_FAILED", false, "see details.reason"},
	{"E_QUEUE_FULL", true, "wait for a job"},
	{"E_RATE_LIMITED", true, "wait Retry-After s"},
	{"E_LIMIT_EXCEEDED", false, "over a ceiling"},
	{"E_CANCELLED", false, "start the job again"},
	{"E_NOTHING_TO_UNDO", false, "nothing to undo"},
	{"E_NOTHING_TO_REDO", false, "nothing to redo"},
	{"E_LINT_BLOCKING", false, "fix details.findings"},
	{"E_PAYLOAD_TOO_LARGE", false, "send a smaller batch"},
}

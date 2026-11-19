// Package httpapi is the HTTP adapter of service.Service (ADR-0017): one typed
// handler per Prototype operation of SDD-000 section 1, the error envelope, the
// local-mode trust checks of SDD-007 section 3 and the OpenAPI document. A
// handler parses its input, calls one Service method and formats the result;
// it holds no rule about timelines (ADR-0008).
package httpapi

import (
	"net/http"

	"github.com/AlexGalhardo/stuarty/service"
)

// BasePath is where the API lives (SDD-003 section 6.1).
const BasePath = "/api/v1"

// Endpoint is one row of the route and scope table (SDD-003 section 6.3, SDD-007
// section 5.2). The table is data: a route that has no row is not registered,
// and a test fails for a router route or a Service method without one.
type Endpoint struct {
	ID     string // the operation id and the route name
	Method string
	Path   string // under BasePath, except Root
	Root   bool   // Path is relative to the origin, not to BasePath
	Scope  string // "" for the routes that carry no scope (handshake, launch)
	// AlsoScope is a second scope that gets past the route; the Service decides
	// whether it is enough (JobCancel with media, SDD-007 section 5.2).
	AlsoScope string
	Service   string // the Service method that answers it, "" when it is not a service operation
	Status    int
	Summary   string
	// Pending marks an operation of the Prototype whose Service method arrives
	// with a later task; its handler answers E_INTERNAL until then.
	Pending string
	// Uncounted marks a route the rate limiters never count: the event stream and
	// the byte-serving routes that the editor calls many times a second (SDD-007
	// section 4.4).
	Uncounted bool
	// MaxInFlight caps the calls of one caller that run at once on a route that is
	// Uncounted but does work on a cache miss (ViewFrame renders a frame).
	MaxInFlight int
}

// FullPath is the path a client requests.
func (e Endpoint) FullPath() string {
	if e.Root {
		return e.Path
	}

	return BasePath + e.Path
}

// Endpoints is every route of the first service (SDD-000 section 1),
// plus the trash and restore routes whose Service methods already exist.
var Endpoints = []Endpoint{
	{
		ID:      "projects.list",
		Method:  http.MethodGet,
		Path:    "/projects",
		Scope:   service.ScopeRead,
		Service: "ProjectList",
		Status:  200,
		Summary: "List projects",
	},
	{
		ID:      "projects.create",
		Method:  http.MethodPost,
		Path:    "/projects",
		Scope:   service.ScopeEdit,
		Service: "ProjectCreate",
		Status:  201,
		Summary: "Create a project",
	},
	{
		ID:      "projects.get",
		Method:  http.MethodGet,
		Path:    "/projects/{id}",
		Scope:   service.ScopeRead,
		Service: "ProjectGet",
		Status:  200,
		Summary: "Read a project",
	},
	{
		ID:      "projects.bundle.export",
		Method:  http.MethodPost,
		Path:    "/projects/{id}/bundle",
		Scope:   service.ScopeRender,
		Service: "ProjectBundleExport",
		Status:  202,
		Summary: "Export a project as a .vvbundle",
	},
	{
		ID:        "projects.bundle.import",
		Method:    http.MethodPost,
		Path:      "/projects/import",
		Scope:     service.ScopeEdit,
		AlsoScope: service.ScopeMedia,
		Service:   "ProjectBundleImport",
		Status:    201,
		Summary:   "Import a .vvbundle as a project",
	},
	{
		ID: "projects.otio.export", Method: http.MethodGet, Path: "/projects/{id}/otio", Scope: service.ScopeRead,
		Service: "ProjectOtioExport", Status: 200, Summary: "Export a project as OpenTimelineIO",
	},
	{
		ID: "projects.otio.import", Method: http.MethodPost, Path: "/projects/{id}/otio", Scope: service.ScopeEdit,
		Service: "ProjectOtioImport", Status: 200, Summary: "Import an OpenTimelineIO file into a project",
	},
	{
		ID: "projects.describe", Method: http.MethodGet, Path: "/projects/{id}/describe", Scope: service.ScopeRead,
		Service: "ProjectDescribe", Status: 200, Summary: "Describe a project as text: brief, items and state",
	},
	{
		ID: "projects.duplicate", Method: http.MethodPost, Path: "/projects/{id}/duplicate", Scope: service.ScopeEdit,
		Service: "ProjectDuplicate", Status: 201, Summary: "Duplicate a project",
	},
	{
		ID: "projects.items.add", Method: http.MethodPost, Path: "/projects/{id}/items", Scope: service.ScopeEdit,
		Service: "ProjectItemAdd", Status: 201, Summary: "Add an item (text, timeline or output) to a project",
	},
	{
		ID: "projects.items.move", Method: http.MethodPost, Path: "/projects/{id}/items/{item_id}/move",
		Scope: service.ScopeEdit, Service: "ProjectItemMove", Status: 200, Summary: "Move an item of a project",
	},
	{
		ID: "projects.items.rename", Method: http.MethodPost, Path: "/projects/{id}/items/{item_id}/rename",
		Scope: service.ScopeEdit, Service: "ProjectItemRename", Status: 200, Summary: "Rename an item of a project",
	},
	{
		ID: "projects.items.trash", Method: http.MethodPost, Path: "/projects/{id}/items/{item_id}/trash",
		Scope: service.ScopeEdit, Service: "ProjectItemTrash", Status: 200, Summary: "Move an item to the trash",
	},
	{
		ID: "projects.items.restore", Method: http.MethodPost, Path: "/projects/{id}/items/{item_id}/restore",
		Scope: service.ScopeEdit, Service: "ProjectItemRestore", Status: 200, Summary: "Restore an item from the trash",
	},
	{
		ID: "projects.brief.get", Method: http.MethodGet, Path: "/projects/{id}/brief", Scope: service.ScopeRead,
		Service: "BriefGet", Status: 200, Summary: "Read the brief of a project",
	},
	{
		ID: "projects.brief.set", Method: http.MethodPut, Path: "/projects/{id}/brief", Scope: service.ScopeEdit,
		Service: "BriefSet", Status: 200, Summary: "Write the brief of a project",
	},
	{
		ID: "projects.trash", Method: http.MethodPost, Path: "/projects/{id}/trash", Scope: service.ScopeEdit,
		Service: "ProjectTrash", Status: 200, Summary: "Move a project to the trash",
	},
	{
		ID: "projects.restore", Method: http.MethodPost, Path: "/projects/{id}/restore", Scope: service.ScopeEdit,
		Service: "ProjectRestore", Status: 200, Summary: "Restore a project from the trash",
	},
	{
		ID: "projects.edit", Method: http.MethodPost, Path: "/projects/{id}/edit", Scope: service.ScopeEdit,
		Service: "Edit", Status: 200, Summary: "Apply a batch of operations (lint_scope fast)",
	},
	{
		ID:      "commands.list",
		Method:  http.MethodGet,
		Path:    "/commands",
		Scope:   service.ScopeRead,
		Service: "CommandHelp",
		Status:  200,
		Summary: "List the commands",
	},
	{
		ID: "commands.get", Method: http.MethodGet, Path: "/commands/{name}", Scope: service.ScopeRead,
		Service: "CommandHelp", Status: 200, Summary: "Explain one command",
	},
	{
		ID: "projects.history", Method: http.MethodGet, Path: "/projects/{id}/history", Scope: service.ScopeRead,
		Service: "HistoryList", Status: 200, Summary: "List the history of a project",
	},
	{
		ID: "projects.undo", Method: http.MethodPost, Path: "/projects/{id}/undo", Scope: service.ScopeEdit,
		Service: "Undo", Status: 200, Summary: "Undo the caller's latest step",
	},
	{
		ID: "projects.redo", Method: http.MethodPost, Path: "/projects/{id}/redo", Scope: service.ScopeEdit,
		Service: "Redo", Status: 200, Summary: "Redo the caller's latest undone step",
	},
	{
		ID: "projects.lint", Method: http.MethodGet, Path: "/projects/{id}/lint", Scope: service.ScopeRead,
		Service: "Lint", Status: 200, Summary: "Lint a project with every rule",
	},

	{
		ID: "media.import", Method: http.MethodPost, Path: "/projects/{id}/media", Scope: service.ScopeMedia,
		Service: "MediaImport", Status: 202, Summary: "Import media files",
	},
	{
		ID: "media.list", Method: http.MethodGet, Path: "/projects/{id}/media", Scope: service.ScopeRead,
		Service: "MediaList", Status: 200, Summary: "List the assets of a project",
	},
	{
		ID: "media.inspect", Method: http.MethodGet, Path: "/projects/{id}/media/{asset_id}", Scope: service.ScopeRead,
		Service: "MediaInspect", Status: 200, Summary: "Inspect an asset",
	},
	{
		ID: "media.trash", Method: http.MethodPost, Path: "/projects/{id}/media/{asset_id}/trash",
		Scope: service.ScopeMedia, Service: "MediaTrash", Status: 200, Summary: "Move an asset to the trash",
	},
	{
		ID: "media.restore", Method: http.MethodPost, Path: "/projects/{id}/media/{asset_id}/restore",
		Scope: service.ScopeMedia, Service: "MediaRestore", Status: 200, Summary: "Restore an asset from the trash",
	},
	{
		ID: "jobs.start", Method: http.MethodPost, Path: "/projects/{id}/jobs", Scope: service.ScopeRender,
		AlsoScope: service.ScopeMedia, Service: "JobStart", Status: 202, Summary: "Start a job",
	},
	{
		ID: "jobs.list", Method: http.MethodGet, Path: "/projects/{id}/jobs", Scope: service.ScopeRead,
		Service: "JobList", Status: 200, Summary: "List the jobs of a project",
	},
	{
		ID: "jobs.get", Method: http.MethodGet, Path: "/jobs/{job_id}", Scope: service.ScopeRead,
		Service: "JobGet", Status: 200, Summary: "Read a job",
	},
	{
		ID: "jobs.cancel", Method: http.MethodPost, Path: "/jobs/{job_id}/cancel", Scope: service.ScopeRender,
		AlsoScope: service.ScopeMedia, Service: "JobCancel", Status: 200, Summary: "Cancel a job",
	},
	{
		ID: "templates.list", Method: http.MethodGet, Path: "/templates", Scope: service.ScopeRead,
		Service: "TemplateList", Status: 200, Summary: "List the templates",
	},
	{
		ID: "templates.save", Method: http.MethodPost, Path: "/templates", Scope: service.ScopeEdit,
		Service: "TemplateSave", Status: 201, Summary: "Save a project as a template",
	},
	{
		ID:      "templates.instantiate",
		Method:  http.MethodPost,
		Path:    "/templates/{template_id}/instantiate",
		Scope:   service.ScopeEdit,
		Service: "TemplateInstantiate",
		Status:  201,
		Summary: "Make a project from a template",
	},
	{
		ID: "templates.import", Method: http.MethodPost, Path: "/templates/import", Scope: service.ScopeEdit,
		Service: "TemplateImport", Status: 201, Summary: "Import a template file",
	},
	{
		ID: "templates.export", Method: http.MethodPost, Path: "/templates/{template_id}/export",
		Scope: service.ScopeRead, Service: "TemplateExport", Status: 200, Summary: "Write the file of a template",
	},
	{
		ID: "templates.file", Method: http.MethodGet, Path: "/templates/{template_id}/file", Scope: service.ScopeRead,
		Service: "OpenFile", Status: 200, Summary: "Download the file of a template",
	},
	{
		ID: "trash.empty", Method: http.MethodPost, Path: "/trash/empty", Scope: service.ScopeAdmin,
		Service: "TrashEmpty", Status: 200,
		Summary: "Empty the trash for good (a browser session or an admin key only; the local token gets 403)",
	},
	{
		ID: "system.tools.install", Method: http.MethodPost, Path: "/system/tools/install", Scope: service.ScopeAdmin,
		Service: "ToolsInstall", Status: 202,
		Summary: "Install the transcription helper (a browser session or an admin key only; the local token gets 403)",
	},
	{
		ID: "system.tools", Method: http.MethodGet, Path: "/system/tools", Scope: service.ScopeRead,
		Service: "ToolsStatus", Status: 200, Summary: "List the installed helpers and the install progress",
	},
	{
		ID: "projects.frame", Method: http.MethodGet, Path: "/projects/{id}/frame", Scope: service.ScopeRead,
		Service: "ViewFrame", Uncounted: true, MaxInFlight: 2, Status: 200, Summary: "Render one frame",
	},
	{
		ID: "projects.sheet", Method: http.MethodGet, Path: "/projects/{id}/sheet", Scope: service.ScopeRead,
		Service: "ViewSheet", Status: 200, Summary: "Render a contact sheet",
	},
	{
		ID:      "projects.snap_points",
		Method:  http.MethodGet,
		Path:    "/projects/{id}/snap-points",
		Scope:   service.ScopeRead,
		Service: "SnapPoints",
		Status:  200,
		Summary: "Read the snap points and trim limits",
	},
	{
		ID: "system.get", Method: http.MethodGet, Path: "/system", Scope: service.ScopeRead,
		Service: "Doctor", Status: 200, Summary: "Read the diagnostic report",
	},
	{
		ID: "assets.list", Method: http.MethodGet, Path: "/assets", Scope: service.ScopeRead,
		Service: "AssetList", Status: 200, Summary: "List the files of the workspace by content hash",
	},
	{
		ID: "fonts.list", Method: http.MethodGet, Path: "/fonts", Scope: service.ScopeRead,
		Service: "FontList", Status: 200, Summary: "List the bundled font families and weights",
	},
	{
		ID: "fonts.import", Method: http.MethodPost, Path: "/fonts", Scope: service.ScopeMedia,
		Service: "FontImport", Status: 201, Summary: "Import a font file into the font store",
	},
	{
		ID: "brand.get", Method: http.MethodGet, Path: "/brand-kit", Scope: service.ScopeRead,
		Service: "BrandKitGet", Status: 200, Summary: "Read the brand kit",
	},
	{
		ID: "brand.set", Method: http.MethodPut, Path: "/brand-kit", Scope: service.ScopeEdit,
		Service: "BrandKitSet", Status: 200, Summary: "Replace the brand kit",
	},
	{
		ID: "connections.list", Method: http.MethodGet, Path: "/connections", Scope: service.ScopeRead,
		Service: "ConnectionList", Status: 200, Summary: "List the destinations the user connected (never a secret)",
	},
	{
		ID: "connections.get", Method: http.MethodGet, Path: "/connections/{id}", Scope: service.ScopeRead,
		Service: "ConnectionGet", Status: 200, Summary: "Read one connection (never a secret)",
	},
	{
		ID: "connections.create", Method: http.MethodPost, Path: "/connections", Scope: service.ScopeAdmin,
		Service: "ConnectionCreate", Status: 201, Summary: "Connect a destination; the secret goes into the vault",
	},
	{
		ID: "connections.remove", Method: http.MethodDelete, Path: "/connections/{id}", Scope: service.ScopeAdmin,
		Service: "ConnectionRemove", Status: 200, Summary: "Remove a connection and its credential",
	},
	{
		ID: "connections.test", Method: http.MethodPost, Path: "/connections/{id}/test", Scope: service.ScopeAdmin,
		Service: "ConnectionTest", Status: 200, Summary: "Make one request through a connection",
	},
	{
		ID: "library.memory.list", Method: http.MethodGet, Path: "/library/memory", Scope: service.ScopeRead,
		Service: "MemoryList", Status: 200, Summary: "List the memory notes",
	},
	{
		ID: "library.memory.get", Method: http.MethodGet, Path: "/library/memory/{name}", Scope: service.ScopeRead,
		Service: "MemoryGet", Status: 200, Summary: "Read a memory note",
	},
	{
		ID: "library.memory.write", Method: http.MethodPost, Path: "/library/memory", Scope: service.ScopeEdit,
		Service: "MemoryWrite", Status: 200, Summary: "Create or replace a memory note",
	},
	{
		ID:      "library.memory.remove",
		Method:  http.MethodDelete,
		Path:    "/library/memory/{name}",
		Scope:   service.ScopeEdit,
		Service: "MemoryRemove",
		Status:  200,
		Summary: "Remove a memory note",
	},
	{
		ID: "library.skills.list", Method: http.MethodGet, Path: "/library/skills", Scope: service.ScopeRead,
		Service: "SkillList", Status: 200, Summary: "List the skills",
	},
	{
		ID: "library.skills.get", Method: http.MethodGet, Path: "/library/skills/{name}", Scope: service.ScopeRead,
		Service: "SkillGet", Status: 200, Summary: "Read a skill",
	},
	{
		ID: "library.skills.write", Method: http.MethodPut, Path: "/library/skills/{name}", Scope: service.ScopeEdit,
		Service: "SkillWrite", Status: 200, Summary: "Create or replace a skill",
	},
	{
		ID:      "library.skills.import",
		Method:  http.MethodPost,
		Path:    "/library/skills/import",
		Scope:   service.ScopeMedia,
		Service: "SkillImport",
		Status:  201,
		Summary: "Import a skill from a folder or an archive; it starts unreviewed",
	},
	{
		ID: "library.skills.export", Method: http.MethodPost, Path: "/library/skills/export", Scope: service.ScopeMedia,
		Service: "SkillExport", Status: 200, Summary: "Export skills as folders in the Agent Skills layout",
	},
	{
		ID:      "library.skills.review",
		Method:  http.MethodPost,
		Path:    "/library/skills/{name}/review",
		Scope:   service.ScopeEdit,
		Service: "SkillMarkReviewed",
		Status:  200,
		Summary: "Mark a skill as read (a browser session only)",
	},
	{
		ID:      "library.skills.lint",
		Method:  http.MethodGet,
		Path:    "/library/skills/{name}/lint",
		Scope:   service.ScopeRead,
		Service: "SkillLint",
		Status:  200,
		Summary: "Run the lint on a skill",
	},
	{
		ID: "library.prompts.list", Method: http.MethodGet, Path: "/library/prompts", Scope: service.ScopeRead,
		Service: "PromptList", Status: 200, Summary: "List the prompts",
	},
	{
		ID: "library.prompts.get", Method: http.MethodGet, Path: "/library/prompts/{name}", Scope: service.ScopeRead,
		Service: "PromptGet", Status: 200, Summary: "Read a prompt",
	},
	{
		ID: "library.prompts.write", Method: http.MethodPut, Path: "/library/prompts/{name}", Scope: service.ScopeEdit,
		Service: "PromptWrite", Status: 200, Summary: "Create or replace a prompt",
	},
	{
		ID:      "library.prompts.render",
		Method:  http.MethodPost,
		Path:    "/library/prompts/{name}/render",
		Scope:   service.ScopeRead,
		Service: "PromptRender",
		Status:  200,
		Summary: "Replace the variables of a prompt",
	},
	{
		ID: "library.search", Method: http.MethodGet, Path: "/library/search", Scope: service.ScopeRead,
		Service: "LibrarySearch", Status: 200, Summary: "Search memory, skills and prompts",
	},
	{
		ID: "library.reindex", Method: http.MethodPost, Path: "/library/reindex", Scope: service.ScopeEdit,
		Service: "LibraryReindex", Status: 200, Summary: "Rebuild the index of the library from its files",
	},
	{
		ID: "audit.list", Method: http.MethodGet, Path: "/audit", Scope: service.ScopeAdmin,
		Service: "AuditList", Status: 200, Summary: "List the audit table",
	},
	{
		ID: "projects.plan", Method: http.MethodGet, Path: "/projects/{id}/plan", Scope: service.ScopeRead,
		Service: "PlanWindow", Uncounted: true, Status: 200, Summary: "Read the render plan of a window",
	},
	{
		ID: "events.stream", Method: http.MethodGet, Path: "/projects/{id}/events", Scope: service.ScopeRead,
		Uncounted: true, Status: 200, Summary: "Stream the events of a project",
	},

	{
		ID: "jobs.file", Method: http.MethodGet, Path: "/jobs/{job_id}/file", Scope: service.ScopeRead,
		Service: "ExportFile", Status: 200, Summary: "Download the file of a finished export",
	},
	{
		ID:        "media.proxy",
		Method:    http.MethodGet,
		Path:      "/projects/{id}/media/{asset_id}/proxy",
		Service:   "OpenFile",
		Scope:     service.ScopeRead,
		Uncounted: true, Status: 200,
		Summary: "Download the proxy of an asset",
	},
	{
		ID: "media.thumbnail", Method: http.MethodGet, Path: "/projects/{id}/media/{asset_id}/thumbnail",
		Service: "OpenFile",
		Scope:   service.ScopeRead, Uncounted: true, Status: 200, Summary: "Download a thumbnail of an asset",
	},
	{
		ID:        "media.strip",
		Method:    http.MethodGet,
		Path:      "/projects/{id}/media/{asset_id}/strip",
		Service:   "OpenFile",
		Scope:     service.ScopeRead,
		Uncounted: true, Status: 200,
		Summary: "Download the tile strip of an asset",
	},
	{
		ID:        "media.peaks",
		Method:    http.MethodGet,
		Path:      "/projects/{id}/media/{asset_id}/peaks",
		Service:   "OpenFile",
		Scope:     service.ScopeRead,
		Uncounted: true, Status: 200,
		Summary: "Download the audio peaks of an asset",
	},
	{
		ID:        "media.lut",
		Method:    http.MethodGet,
		Path:      "/projects/{id}/media/{asset_id}/lut",
		Service:   "OpenFile",
		Scope:     service.ScopeRead,
		Uncounted: true, Status: 200,
		Summary: "Download the parsed lookup table of a lut asset",
	},
	{
		ID:        "media.chunks",
		Method:    http.MethodGet,
		Path:      "/projects/{id}/media/{asset_id}/chunks",
		Service:   "OpenFile",
		Scope:     service.ScopeRead,
		Uncounted: true, Status: 200,
		Summary: "Download the chunk index of the video proxy of an asset",
	},
	{
		ID: "bitmaps.get", Method: http.MethodGet, Path: "/projects/{id}/bitmaps/{key}", Scope: service.ScopeRead,
		Uncounted: true, Status: 200, Summary: "Download a rasterized text or shape",
	},

	{
		ID: "handshake.get", Method: http.MethodGet, Path: "/handshake", Status: 200,
		Summary: "Prove that the server knows the local token",
	},
	{
		ID: "system.launch", Method: http.MethodPost, Path: "/system/launch", Scope: service.ScopeAdmin, Status: 200,
		Summary: "Issue a one-time browser launch code",
	},

	{
		ID:      "openapi.get",
		Method:  http.MethodGet,
		Path:    "/openapi.json",
		Root:    true,
		Scope:   service.ScopeRead,
		Status:  200,
		Summary: "The OpenAPI document",
	},
	{
		ID: "docs.get", Method: http.MethodGet, Path: "/api/docs", Root: true, Scope: service.ScopeRead, Status: 200,
		Summary: "The API reference page",
	},
	{
		ID: "docs.scalar", Method: http.MethodGet, Path: "/api/docs/scalar.js", Root: true, Scope: service.ScopeRead,
		Status: 200, Summary: "The API reference page script",
	},
	{
		ID: "docs.page", Method: http.MethodGet, Path: "/api/docs/page.js", Root: true, Scope: service.ScopeRead,
		Status: 200, Summary: "The API reference page shim",
	},
	{
		ID: "mcp.post", Method: http.MethodPost, Path: "/mcp", Root: true, Scope: service.ScopeRead, Status: 200,
		Summary: "The MCP server (Streamable HTTP, stateless)",
	},
	{
		ID: "auth.launch", Method: http.MethodGet, Path: "/auth/launch", Root: true, Status: 303,
		Summary: "Exchange a launch code for a browser session",
	},
	{
		ID: "keys.create", Method: http.MethodPost, Path: "/keys", Scope: service.ScopeRead, Service: "KeyCreate",
		Status: 201, Summary: "Create an API key (a logged-in session only; network mode)",
	},
	{
		ID: "keys.list", Method: http.MethodGet, Path: "/keys", Scope: service.ScopeRead, Service: "KeyList",
		Status: 200, Summary: "List your API keys (a logged-in session only; network mode)",
	},
	{
		ID: "keys.revoke", Method: http.MethodDelete, Path: "/keys/{key_id}", Scope: service.ScopeRead,
		Service: "KeyRevoke", Status: 200, Summary: "Revoke an API key (a logged-in session only; network mode)",
	},
	{
		ID: "users.list", Method: http.MethodGet, Path: "/users", Scope: service.ScopeAdmin, Status: 200,
		Summary: "List the users (an administrator session only; network mode)",
	},
	{
		ID: "users.create", Method: http.MethodPost, Path: "/users", Scope: service.ScopeAdmin, Status: 201,
		Summary: "Create a user (an administrator session only; network mode)",
	},
	{
		ID: "users.update", Method: http.MethodPatch, Path: "/users/{user_id}", Scope: service.ScopeAdmin, Status: 200,
		Summary: "Change the role of a user or disable or enable them (an administrator session only; network mode)",
	},
	{
		ID: "auth.login", Method: http.MethodPost, Path: "/auth/login", Root: true, Status: 200,
		Summary: "Log in with an email and a password (network mode)",
	},
	{
		ID: "auth.logout", Method: http.MethodPost, Path: "/auth/logout", Root: true, Status: 200,
		Summary: "End the browser session (network mode)",
	},
	// The image documents of SDD-021.
	{
		ID: "images.create", Method: http.MethodPost, Path: "/images", Scope: service.ScopeEdit,
		Service: "ImageCreate", Status: 200, Summary: "Create an image document, blank or from a template",
	},
	{
		ID: "images.list", Method: http.MethodGet, Path: "/images", Scope: service.ScopeRead,
		Service: "ImageList", Status: 200, Summary: "List the image documents and templates",
	},
	{
		ID: "images.get", Method: http.MethodGet, Path: "/images/{id}", Scope: service.ScopeRead,
		Service: "ImageDescribe", Status: 200, Summary: "Describe an image document: layers, findings and text view",
	},
	{
		ID: "images.edit", Method: http.MethodPost, Path: "/images/{id}/edit", Scope: service.ScopeEdit,
		Service: "ImageEdit", Status: 200, Summary: "Apply a batch of image operations",
	},
	{
		ID: "images.undo", Method: http.MethodPost, Path: "/images/{id}/undo", Scope: service.ScopeEdit,
		Service: "ImageUndo", Status: 200, Summary: "Undo the latest step of the caller on an image",
	},
	{
		ID: "images.redo", Method: http.MethodPost, Path: "/images/{id}/redo", Scope: service.ScopeEdit,
		Service: "ImageRedo", Status: 200, Summary: "Redo the latest undone step of the caller on an image",
	},
	{
		ID: "images.export", Method: http.MethodPost, Path: "/images/{id}/export", Scope: service.ScopeEdit,
		Service: "ImageExport", Status: 200, Summary: "Export an image document to a PNG, JPEG or WebP file",
	},
}

// Endpoint returns the row of an operation id.
func endpointOf(id string) Endpoint {
	for i := range Endpoints {
		if Endpoints[i].ID == id {
			return Endpoints[i]
		}
	}

	panic("httpapi: no endpoint " + id)
}

// endpointByID is endpointOf for an id that may not be a route of this package.
func endpointByID(id string) (Endpoint, bool) {
	for i := range Endpoints {
		if Endpoints[i].ID == id {
			return Endpoints[i], true
		}
	}

	return Endpoint{}, false
}

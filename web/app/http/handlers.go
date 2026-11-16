package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/AlexGalhardo/vukem/api"
	"github.com/AlexGalhardo/vukem/openapi"
	"github.com/AlexGalhardo/vukem/router"
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
)

// The input structs below are the HTTP encoding of the request structs of
// SDD-003 section 2: path and query parameters join the body members, and
// vukem derives the OpenAPI parameters and body from their tags. They carry no
// rule: the Service validates (ADR-0008).

type pageIn struct {
	Cursor string `json:"cursor,omitempty" doc:"The next_cursor of the previous page."`
	Limit  int    `json:"limit,omitempty"  doc:"The page size; the default depends on the operation."`
}

type listIn struct {
	State string `json:"state,omitempty" doc:"active (default), trashed or all."`
	pageIn
}

type createIn struct {
	Name   string `json:"name"              validate:"required" doc:"1 to 120 characters."`
	Preset string `json:"preset,omitempty"                      doc:"9:16 (default), 1:1, 16:9 or 4:5."`
	FPS    string `json:"fps,omitempty"                         doc:"30 (default); n or n/1001."`
	ID     string `json:"id,omitempty"                          doc:"An id of your own; E_ID_EXISTS when taken."`
	DryRun bool   `json:"dry_run,omitempty"`
}

type getIn struct {
	ID     string `json:"id"               validate:"required"`
	Detail string `json:"detail,omitempty"                     doc:"concise (default), full or document."`
	pageIn
}

type stateIn struct {
	ID     string `json:"id"                validate:"required"`
	DryRun bool   `json:"dry_run,omitempty"`
}

type mediaStateIn struct {
	ID      string `json:"id"                validate:"required"`
	AssetID string `json:"asset_id"          validate:"required"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

// Operation is one operation of a batch: the command name and its arguments.
// The OpenAPI document replaces args by the schema of each command (ADR-0014).
type Operation struct {
	Op   string         `json:"op"             validate:"required" doc:"A command name, as listed by GET /commands."`
	Args map[string]any `json:"args,omitempty"                     doc:"The arguments of the command."`
}

type editIn struct {
	ID             string      `json:"id"                        validate:"required"`
	BaseRevision   int64       `json:"base_revision"             validate:"required"               doc:"The revision the batch was planned against."`
	Operations     []Operation `json:"operations"                validate:"required,min=1,max=200"`
	IdempotencyKey string      `json:"idempotency_key,omitempty"                                   doc:"8 to 64 characters; the Idempotency-Key header is the same."`
	DryRun         bool        `json:"dry_run,omitempty"`
}

type historyIn struct {
	ID    string `json:"id"              validate:"required"`
	Actor string `json:"actor,omitempty"                     doc:"Only the steps of this actor."`
	pageIn
}

type lintIn struct {
	ID       string `json:"id"                 validate:"required"`
	Severity string `json:"severity,omitempty"                     doc:"warning (default, everything) or error."`
	pageIn
}

type commandIn struct {
	Name string `json:"name" validate:"required"`
}

type noIn struct{}

type handshakeIn struct {
	Nonce string `json:"nonce" validate:"required" doc:"32 lower-case hexadecimal characters."`
}

type handshakeOut struct {
	Proof string `json:"proof" doc:"HMAC-SHA256 keyed by the token over the nonce (SDD-007 section 3.8)."`
}

type launchOut struct {
	Code string `json:"code" doc:"A one-time code, valid for 120 seconds."`
	URL  string `json:"url"  doc:"The address that exchanges the code for a browser session."`
}

var zeroTime time.Time

func register[In, Out any](h *apiHandler, id string, fn func(*web.Ctx, In) (Out, error)) *router.Route {
	ep := endpointOf(id)

	return api.Register(h.r, ep.Method+" "+ep.FullPath(), h.op(id), func(c *web.Ctx, in In) (Out, error) {
		var zero Out

		if err := need(c, ep.Scope); err != nil {
			if ep.AlsoScope == "" {
				return zero, err
			}

			if err2 := need(c, ep.AlsoScope); err2 != nil {
				return zero, err
			}
		}

		return fn(c, in)
	})
}

// pending registers an operation whose Service method arrives with a later task.
func pending(h *apiHandler, id string) {
	register(h, id, func(_ *web.Ctx, _ noIn) (noIn, error) {
		return noIn{}, &service.Error{
			Code: command.CodeInternal, Message: "this operation is not available in this build yet.",
			Hint: "it arrives with " + endpointOf(id).Pending,
		}
	})
}

func (h *apiHandler) registerAll() {
	register(h, "auth.login", h.login).Use(h.loginThrottle.middleware)
	register(h, "auth.logout", h.logout)
	register(h, "keys.create", h.keysCreate)
	register(h, "keys.list", func(c *web.Ctx, _ noIn) (service.KeyListResult, error) {
		res, err := h.Service.KeyList(c.Context(), service.KeyListRequest{})
		return deref(res), err
	})
	register(h, "keys.revoke", func(c *web.Ctx, in keyRevokeIn) (service.Key, error) {
		res, err := h.Service.KeyRevoke(c.Context(), service.KeyRevokeRequest{KeyID: in.KeyID})
		return deref(res), err
	})
	register(h, "users.list", h.usersList)
	register(h, "users.create", h.usersCreate)
	register(h, "users.update", h.usersUpdate)
	register(h, "projects.list", h.projectsList)
	register(h, "projects.create", h.projectsCreate)
	register(h, "projects.get", h.projectsGet)
	register(h, "projects.trash", func(c *web.Ctx, in stateIn) (service.ProjectStateResult, error) {
		return stateResult(
			h.Service.ProjectTrash(
				c.Context(),
				service.ProjectTrashRequest{ProjectID: in.ID, DryRun: dry(c, in.DryRun)},
			),
		)
	})
	register(h, "projects.restore", func(c *web.Ctx, in stateIn) (service.ProjectStateResult, error) {
		return stateResult(
			h.Service.ProjectRestore(
				c.Context(),
				service.ProjectTrashRequest{ProjectID: in.ID, DryRun: dry(c, in.DryRun)},
			),
		)
	})
	register(h, "projects.edit", h.projectsEdit)
	register(h, "commands.list", func(c *web.Ctx, _ noIn) (service.CommandHelpResult, error) {
		res, err := h.Service.CommandHelp(c.Context(), service.CommandHelpRequest{})
		return deref(res), err
	})
	register(h, "commands.get", func(c *web.Ctx, in commandIn) (service.CommandHelpResult, error) {
		res, err := h.Service.CommandHelp(c.Context(), service.CommandHelpRequest{Name: in.Name})
		return deref(res), err
	})
	register(h, "projects.history", func(c *web.Ctx, in historyIn) (service.HistoryListResult, error) {
		res, err := h.Service.HistoryList(c.Context(), service.HistoryListRequest{
			ProjectID: in.ID, Actor: in.Actor, Page: page(in.pageIn),
		})
		if res != nil {
			revisionHeader(c, res.Revision)
		}

		return deref(res), err
	})
	register(h, "projects.undo", func(c *web.Ctx, in stateIn) (service.UndoResult, error) {
		return h.step(c, in, h.Service.Undo)
	})
	register(h, "projects.redo", func(c *web.Ctx, in stateIn) (service.UndoResult, error) {
		return h.step(c, in, h.Service.Redo)
	})
	register(h, "projects.lint", func(c *web.Ctx, in lintIn) (service.LintResult, error) {
		res, err := h.Service.Lint(
			c.Context(),
			service.LintRequest{ProjectID: in.ID, Severity: in.Severity, Page: page(in.pageIn)},
		)
		if res != nil {
			revisionHeader(c, res.Revision)
		}

		return deref(res), err
	})

	register(h, "events.stream", h.eventsStream)
	register(h, "projects.plan", h.projectsPlan)
	register(h, "bitmaps.get", func(c *web.Ctx, in bitmapIn) (noIn, error) {
		return h.serveAsset(c, "bitmap", in.ID, in.Key, "")
	})
	register(h, "projects.frame", h.projectsFrame)
	register(h, "projects.sheet", h.projectsSheet)

	for i := range Endpoints {
		if Endpoints[i].Pending != "" {
			pending(h, Endpoints[i].ID)
		}
	}

	register(h, "media.import", h.mediaImport)
	register(h, "media.list", h.mediaList)
	register(h, "media.inspect", h.mediaInspect)
	register(h, "media.trash", func(c *web.Ctx, in mediaStateIn) (service.MediaStateResult, error) {
		res, err := h.Service.MediaTrash(
			c.Context(),
			service.MediaTrashRequest{ProjectID: in.ID, AssetID: in.AssetID, DryRun: dry(c, in.DryRun)},
		)

		return deref(res), err
	})
	register(h, "media.restore", func(c *web.Ctx, in mediaStateIn) (service.MediaStateResult, error) {
		res, err := h.Service.MediaRestore(
			c.Context(),
			service.MediaTrashRequest{ProjectID: in.ID, AssetID: in.AssetID, DryRun: dry(c, in.DryRun)},
		)

		return deref(res), err
	})
	register(h, "jobs.start", h.jobStart)
	register(h, "jobs.list", h.jobList)
	register(h, "jobs.get", h.jobGet)
	register(h, "jobs.cancel", h.jobCancel)
	register(h, "templates.list", h.templateList)
	register(h, "templates.save", h.templateSave)
	register(h, "templates.instantiate", h.templateInstantiate)
	register(h, "trash.empty", h.trashEmpty)
	register(h, "system.tools.install", h.toolsInstall)
	register(h, "system.tools", h.toolsStatus)
	register(h, "templates.import", h.templateImport)
	register(h, "projects.bundle.export", h.projectBundleExport)
	register(h, "projects.bundle.import", h.projectBundleImport)
	register(h, "projects.otio.export", h.projectOtioExport)
	register(h, "projects.otio.import", h.projectOtioImport)
	register(h, "templates.export", h.templateExport)
	register(h, "templates.file", h.templateFile)
	register(h, "media.proxy", func(c *web.Ctx, in mediaFileIn) (noIn, error) {
		return h.serveAsset(c, "proxy", in.ID, in.AssetID, "")
	})
	register(h, "media.thumbnail", func(c *web.Ctx, in thumbnailIn) (noIn, error) {
		return h.serveAsset(c, "thumbnail", in.ID, in.AssetID, in.Width)
	})
	register(h, "media.strip", func(c *web.Ctx, in mediaFileIn) (noIn, error) {
		return h.serveAsset(c, "strip", in.ID, in.AssetID, "")
	})
	register(h, "media.peaks", func(c *web.Ctx, in mediaFileIn) (noIn, error) {
		return h.serveAsset(c, "peaks", in.ID, in.AssetID, "")
	})
	register(h, "jobs.file", h.jobsFile)
	register(h, "projects.snap_points", h.snapPoints)
	register(h, "fonts.list", h.fontList)
	register(h, "fonts.import", h.fontImport)
	register(h, "brand.get", h.brandKitGet)
	register(h, "brand.set", h.brandKitSet)
	register(h, "system.get", h.systemGet)
	register(h, "audit.list", h.auditList)
	register(h, "system.launch", h.systemLaunch)
	register(h, "handshake.get", func(_ *web.Ctx, in handshakeIn) (handshakeOut, error) {
		if !auth.ValidNonce(in.Nonce) {
			return handshakeOut{}, &service.Error{
				Code: command.CodeValidation, Path: "nonce",
				Message: "the nonce must be 32 lower-case hexadecimal characters.",
			}
		}

		return handshakeOut{Proof: auth.Proof(h.Token, in.Nonce)}, nil
	})

	ep := endpointOf("auth.launch")
	h.r.Get(ep.Path, h.launch).Name(ep.ID).Doc(openapi.Skip{})

	h.registerDocs()
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}

	return *p
}

func page(p pageIn) service.Page { return service.Page{Cursor: p.Cursor, Limit: p.Limit} }

// dry reads the body member or its alias ?dry_run=true (SDD-003 section 6.3).
func dry(c *web.Ctx, member bool) bool { return member || c.QueryBool("dry_run") }

func revisionHeader(c *web.Ctx, rev int64) {
	c.SetHeader("X-Vukem-Revision", strconv.FormatInt(rev, 10))
}

func stateResult(res *service.ProjectStateResult, err error) (service.ProjectStateResult, error) {
	return deref(res), err
}

func (h *apiHandler) projectsList(c *web.Ctx, in listIn) (service.ProjectListResult, error) {
	res, err := h.Service.ProjectList(c.Context(), service.ProjectListRequest{State: in.State, Page: page(in.pageIn)})

	return deref(res), err
}

func (h *apiHandler) projectsCreate(c *web.Ctx, in createIn) (service.ProjectCreateResult, error) {
	dryRun := dry(c, in.DryRun)

	res, err := h.Service.ProjectCreate(c.Context(), service.ProjectCreateRequest{
		Name: in.Name, Preset: in.Preset, FPS: in.FPS, ID: in.ID, DryRun: dryRun,
	})
	if err != nil {
		return service.ProjectCreateResult{}, err
	}

	// 201 with Location for a created project, 200 for a dry run; the handler
	// writes the answer itself because the status depends on the request.
	if dryRun {
		return service.ProjectCreateResult{}, c.JSON(http.StatusOK, res)
	}

	c.SetHeader("Location", BasePath+"/projects/"+url.PathEscape(res.ProjectID))

	return service.ProjectCreateResult{}, c.JSON(http.StatusCreated, res)
}

func (h *apiHandler) projectsGet(c *web.Ctx, in getIn) (service.ProjectGetResult, error) {
	res, err := h.Service.ProjectGet(c.Context(), service.ProjectGetRequest{
		ProjectID: in.ID, Detail: in.Detail, Page: page(in.pageIn),
	})
	if err != nil {
		return service.ProjectGetResult{}, err
	}

	revisionHeader(c, res.Revision)

	detail := in.Detail
	if detail == "" {
		detail = "concise"
	}

	if c.NotModified(`"r`+strconv.FormatInt(res.Revision, 10)+"-"+detail+"-"+res.State+`"`, zeroTime) {
		return service.ProjectGetResult{}, nil
	}

	return *res, nil
}

func (h *apiHandler) projectsEdit(c *web.Ctx, in editIn) (service.EditResult, error) {
	key := in.IdempotencyKey

	if header := c.Header("Idempotency-Key"); header != "" {
		if key != "" && key != header {
			return service.EditResult{}, &service.Error{
				Code: command.CodeValidation, Path: "idempotency_key",
				Message: "the Idempotency-Key header and the idempotency_key member differ.",
			}
		}

		key = header
	}

	ops := make([]command.Operation, len(in.Operations))

	for i, o := range in.Operations {
		args := o.Args
		if args == nil {
			args = map[string]any{}
		}

		raw, err := json.Marshal(args)
		if err != nil {
			return service.EditResult{}, &service.Error{
				Code:    command.CodeValidation,
				Path:    "operations",
				Message: "the arguments are not valid JSON.",
			}
		}

		ops[i] = command.Operation{Op: o.Op, Args: raw}
	}

	res, err := h.Service.Edit(c.Context(), service.EditRequest{
		ProjectID:      in.ID,
		BaseRevision:   in.BaseRevision,
		Operations:     ops,
		IdempotencyKey: key,
		DryRun:         dry(c, in.DryRun),
	})
	if err != nil {
		return service.EditResult{}, err
	}

	revisionHeader(c, res.Revision)

	return *res, nil
}

func (h *apiHandler) step(
	c *web.Ctx,
	in stateIn,
	fn func(ctx context.Context, req service.UndoRequest) (*service.UndoResult, error),
) (service.UndoResult, error) {
	res, err := fn(c.Context(), service.UndoRequest{ProjectID: in.ID, DryRun: dry(c, in.DryRun)})
	if err != nil {
		return service.UndoResult{}, err
	}

	revisionHeader(c, res.Revision)

	return *res, nil
}

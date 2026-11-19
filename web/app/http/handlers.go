package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/auth"

	"github.com/AlexGalhardo/vukem/api"
	"github.com/AlexGalhardo/vukem/openapi"
	"github.com/AlexGalhardo/vukem/router"
	"github.com/AlexGalhardo/vukem/web"
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
	Name   string  `json:"name"              validate:"required" doc:"1 to 120 characters."`
	Preset string  `json:"preset,omitempty"                      doc:"9:16 (default), 1:1, 16:9 or 4:5."`
	FPS    fpsText `json:"fps,omitempty"                         doc:"30 (default); n or n/1001, as text or as a number."`
	ID     string  `json:"id,omitempty"                          doc:"An id of your own; E_ID_EXISTS when taken."`
	DryRun any     `json:"dry_run,omitempty"`

	// Width and Height are not accepted: a project takes its size from preset. They are members so that a
	// request that sends them is refused instead of silently made 9:16 (VV-086).
	Width  *int `json:"width,omitempty"  doc:"Not accepted: the size comes from preset."`
	Height *int `json:"height,omitempty" doc:"Not accepted: the size comes from preset."`
}

// fpsText is the frame rate of a new project. The CLI takes --fps 30, so the API takes the JSON
// number 30 as well as the text "30" or "30000/1001" (VV-103); the service sees text.
type fpsText string

func (f *fpsText) UnmarshalJSON(b []byte) error {
	var n json.Number

	if err := json.Unmarshal(b, &n); err == nil {
		*f = fpsText(n.String())
		return nil
	}

	var s string

	if err := json.Unmarshal(b, &s); err != nil {
		return errors.New("must be a number or text such as 30 or 30000/1001")
	}

	*f = fpsText(s)

	return nil
}

type getIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Detail     string `json:"detail,omitempty"                          doc:"concise (default), full or document."`
	pageIn
}

type stateIn struct {
	ID     string `json:"id"                validate:"required"`
	DryRun any    `json:"dry_run,omitempty"`
}

// stepIn is the body of undo and redo. dry_run is read as it came (readDryRun).
type stepIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	DryRun     any    `json:"dry_run,omitempty"`
}

type mediaStateIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	AssetID    string `json:"asset_id"              validate:"required"`
	DryRun     any    `json:"dry_run,omitempty"`
}

// Operation is one operation of a batch: the command name and its arguments.
// The OpenAPI document replaces args by the schema of each command (ADR-0014).
type Operation struct {
	Op   string         `json:"op"             validate:"required" doc:"A command name, as listed by GET /commands."`
	Args map[string]any `json:"args,omitempty"                     doc:"The arguments of the command."`
}

type editIn struct {
	ID             string      `json:"id"                        validate:"required"`
	TimelineID     string      `json:"timeline_id,omitempty"                                       doc:"The timeline of the project; required when the project has several."`
	BaseRevision   int64       `json:"base_revision"             validate:"required"               doc:"The revision the batch was planned against."`
	Operations     []Operation `json:"operations"                validate:"required,min=1,max=200"`
	IdempotencyKey string      `json:"idempotency_key,omitempty"                                   doc:"8 to 64 characters; the Idempotency-Key header is the same."`
	DryRun         any         `json:"dry_run,omitempty"`
}

type historyIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Actor      string `json:"actor,omitempty"                           doc:"Only the steps of this actor."`
	pageIn
}

type lintIn struct {
	ID         string `json:"id"                    validate:"required"`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
	Severity   string `json:"severity,omitempty"                        doc:"warning (default, everything) or error."`
	pageIn
}

type commandIn struct {
	Name   string `json:"name"             validate:"required"`
	Domain string `json:"domain,omitempty"                     doc:"A domain of the product, video by default."`
}

type commandListIn struct {
	Domain string `json:"domain,omitempty" doc:"A domain of the product, video by default."`
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

		if err := explicitSize(c); err != nil {
			return zero, err
		}

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

// explicitSize refuses a limit or a count that the request names and that is below 1: the Service cannot
// tell limit=0 from an absent limit, and a page of zero items is never what the caller meant (VV-104).
func explicitSize(c *web.Ctx) error {
	for _, name := range []string{"limit", "count"} {
		raw := c.Query(name)
		if raw == "" {
			continue
		}

		if n, err := strconv.Atoi(raw); err == nil && n < 1 {
			return &service.Error{
				Code: command.CodeValidation, Path: name,
				Message: name + " is " + raw + "; it must be at least 1.",
				Hint:    "omit " + name + " for the default",
			}
		}
	}

	return nil
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
	register(h, "google.status", h.googleStatus)
	register(h, "google.start", h.googleStart)
	register(h, "google.complete", h.googleComplete)
	register(h, "google.link", h.googleLink)
	register(h, "google.unlink", h.googleUnlink)
	register(h, "google.settings.get", h.googleSettingsGet)
	register(h, "google.settings.put", h.googleSettingsPut)
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
		dryRun, err := readDryRun(c, in.DryRun)
		if err != nil {
			return service.ProjectStateResult{}, err
		}

		return stateResult(
			h.Service.ProjectTrash(
				c.Context(),
				service.ProjectTrashRequest{ProjectID: in.ID, DryRun: dryRun},
			),
		)
	})
	register(h, "projects.restore", func(c *web.Ctx, in stateIn) (service.ProjectStateResult, error) {
		dryRun, err := readDryRun(c, in.DryRun)
		if err != nil {
			return service.ProjectStateResult{}, err
		}

		return stateResult(
			h.Service.ProjectRestore(
				c.Context(),
				service.ProjectTrashRequest{ProjectID: in.ID, DryRun: dryRun},
			),
		)
	})
	register(h, "projects.edit", h.projectsEdit)
	register(h, "commands.list", func(c *web.Ctx, in commandListIn) (service.CommandHelpResult, error) {
		res, err := h.Service.CommandHelp(c.Context(), service.CommandHelpRequest{Domain: in.Domain})
		return deref(res), err
	})
	register(h, "commands.get", func(c *web.Ctx, in commandIn) (service.CommandHelpResult, error) {
		res, err := h.Service.CommandHelp(c.Context(), service.CommandHelpRequest{Name: in.Name, Domain: in.Domain})
		return deref(res), err
	})
	register(h, "projects.history", func(c *web.Ctx, in historyIn) (service.HistoryListResult, error) {
		res, err := h.Service.HistoryList(c.Context(), service.HistoryListRequest{
			ProjectID: in.ID, TimelineID: in.TimelineID, Actor: in.Actor, Page: page(in.pageIn),
		})
		if res != nil {
			revisionHeader(c, res.Revision)
		}

		return deref(res), err
	})
	register(h, "projects.undo", func(c *web.Ctx, in stepIn) (service.UndoResult, error) {
		return h.step(c, in, h.Service.Undo)
	})
	register(h, "projects.redo", func(c *web.Ctx, in stepIn) (service.UndoResult, error) {
		return h.step(c, in, h.Service.Redo)
	})
	h.registerHistoryOps()
	register(h, "projects.lint", func(c *web.Ctx, in lintIn) (service.LintResult, error) {
		res, err := h.Service.Lint(
			c.Context(),
			service.LintRequest{
				ProjectID:  in.ID,
				TimelineID: in.TimelineID,
				Severity:   in.Severity,
				Page:       page(in.pageIn),
			},
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
	register(h, "audio_previews.get", func(c *web.Ctx, in audioPreviewIn) (noIn, error) {
		return h.serveAsset(c, "audio_preview", in.ID, in.Key, in.From)
	})
	register(h, "projects.frame", h.projectsFrame)
	register(h, "projects.sheet", h.projectsSheet)
	register(h, "projects.motion.brief", h.motionBrief)
	register(h, "projects.motion.check", h.motionCheck)
	register(h, "projects.motion.replace", h.motionReplace)

	for i := range Endpoints {
		if Endpoints[i].Pending != "" {
			pending(h, Endpoints[i].ID)
		}
	}

	register(h, "media.import", h.mediaImport)
	register(h, "media.list", h.mediaList)
	register(h, "media.inspect", h.mediaInspect)
	register(h, "media.trash", func(c *web.Ctx, in mediaStateIn) (service.MediaStateResult, error) {
		dryRun, err := readDryRun(c, in.DryRun)
		if err != nil {
			return service.MediaStateResult{}, err
		}

		res, err := h.Service.MediaTrash(
			c.Context(),
			service.MediaTrashRequest{ProjectID: in.ID, TimelineID: in.TimelineID, AssetID: in.AssetID, DryRun: dryRun},
		)

		return deref(res), err
	})
	register(h, "media.restore", func(c *web.Ctx, in mediaStateIn) (service.MediaStateResult, error) {
		dryRun, err := readDryRun(c, in.DryRun)
		if err != nil {
			return service.MediaStateResult{}, err
		}

		res, err := h.Service.MediaRestore(
			c.Context(),
			service.MediaTrashRequest{ProjectID: in.ID, TimelineID: in.TimelineID, AssetID: in.AssetID, DryRun: dryRun},
		)

		return deref(res), err
	})
	register(h, "jobs.start", h.jobStart)
	register(h, "jobs.list", h.jobList)
	register(h, "jobs.get", h.jobGet)
	register(h, "jobs.cancel", h.jobCancel)
	register(h, "export.presets", h.exportPresetList)
	register(h, "export.check", h.exportCheck)
	register(h, "audio.measure", h.audioMeasure)
	register(h, "export.batch", h.exportBatch)
	register(h, "templates.list", h.templateList)
	register(h, "templates.save", h.templateSave)
	register(h, "templates.instantiate", h.templateInstantiate)
	h.registerImages()
	register(h, "trash.empty", h.trashEmpty)
	register(h, "system.tools.install", h.toolsInstall)
	register(h, "system.tools", h.toolsStatus)
	register(h, "templates.import", h.templateImport)
	register(h, "projects.bundle.export", h.projectBundleExport)
	register(h, "projects.bundle.import", h.projectBundleImport)
	register(h, "projects.otio.export", h.projectOtioExport)
	register(h, "projects.otio.import", h.projectOtioImport)
	register(h, "projects.describe", h.projectDescribe)
	register(h, "projects.duplicate", h.projectDuplicate)
	register(h, "projects.items.add", h.projectItemAdd)
	register(h, "projects.items.move", h.projectItemMove)
	register(h, "projects.items.rename", h.projectItemRename)
	register(h, "projects.items.trash", func(c *web.Ctx, in itemStateIn) (service.ProjectItemResult, error) {
		return h.projectItemState(c, in, false)
	})
	register(h, "projects.items.restore", func(c *web.Ctx, in itemStateIn) (service.ProjectItemResult, error) {
		return h.projectItemState(c, in, true)
	})
	register(h, "projects.brief.get", h.briefGet)
	register(h, "projects.brief.set", h.briefSet)
	register(h, "briefing.questions", h.briefingQuestions)
	register(h, "projects.briefing.answer", h.briefingAnswer)
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
	register(h, "media.lut", func(c *web.Ctx, in mediaFileIn) (noIn, error) {
		return h.serveAsset(c, "lut", in.ID, in.AssetID, "")
	})
	register(h, "media.chunks", func(c *web.Ctx, in mediaFileIn) (noIn, error) {
		return h.serveAsset(c, "chunks", in.ID, in.AssetID, "")
	})
	register(h, "jobs.file", h.jobsFile)
	register(h, "projects.snap_points", h.snapPoints)
	register(h, "fonts.list", h.fontList)
	register(h, "assets.list", h.assetList)
	register(h, "assets.get", h.assetGet)
	register(h, "assets.uses", h.assetUses)
	register(h, "assets.set", h.assetSet)
	register(h, "assets.tag", h.assetTag)
	register(h, "fonts.import", h.fontImport)
	register(h, "brand.get", h.brandKitGet)
	register(h, "brand.set", h.brandKitSet)
	register(h, "connections.list", h.connectionsList)
	register(h, "connections.get", h.connectionsGet)
	register(h, "connections.create", h.connectionsCreate)
	register(h, "connections.remove", h.connectionsRemove)
	register(h, "connections.test", h.connectionsTest)
	register(h, "approvals.list", h.approvalsList)
	register(h, "approvals.events", h.approvalsStream)
	register(h, "approvals.get", h.approvalsGet)
	register(h, "approvals.decide", h.approvalsDecide)
	register(h, "approval-rules.list", h.approvalRulesList)
	register(h, "approval-rules.set", h.approvalRulesSet)
	register(h, "approval-rules.remove", h.approvalRulesRemove)
	register(h, "cost.get", h.costGet)
	register(h, "cost.ceilings.set", h.costCeilingSet)
	register(h, "social.networks", h.socialNetworks)
	register(h, "social.connect", h.socialConnect)
	register(h, "social.complete", h.socialComplete)
	register(h, "social.accounts", h.socialAccounts)
	register(h, "social.select", h.socialSelect)
	register(h, "social.price", h.socialPrice)
	register(h, "publish.validate", h.publishValidate)
	register(h, "publish.now", h.publishNow)
	register(h, "publish.status", h.publishStatus)
	register(h, "publications.create", h.publicationsCreate)
	register(h, "publications.list", h.publicationsList)
	register(h, "publications.events", h.publicationsStream)
	register(h, "publications.get", h.publicationsGet)
	register(h, "publications.update", h.publicationsUpdate)
	register(h, "publications.schedule", h.publicationsSchedule)
	register(h, "publications.send-now", h.publicationsSendNow)
	register(h, "publications.retry", h.publicationsRetry)
	register(h, "publications.cancel", h.publicationsCancel)
	register(h, "providers.list", h.providersList)
	register(h, "providers.models", h.providersModels)
	register(h, "providers.prices.set", h.providersPriceSet)
	register(h, "providers.signin.begin", h.providersSignInBegin)
	register(h, "providers.signin.complete", h.providersSignInComplete)
	register(h, "providers.sign-ins", h.providersSignIns)
	register(h, "text.generate", h.textGenerate)
	register(h, "texts.list", h.textList)
	register(h, "texts.get", h.textGet)
	register(h, "texts.create", h.textCreate)
	register(h, "texts.write", h.textWrite)
	register(h, "texts.revisions", h.textRevisions)
	register(h, "texts.revert", h.textRevert)
	register(h, "texts.draft", h.textDraft)
	register(h, "captions.validate", h.captionValidate)
	register(h, "scripts.estimate", h.scriptEstimate)
	register(h, "image.generate", h.imageGenerate)
	register(h, "voice.generate", h.voiceGenerate)
	register(h, "images.model_edit", h.imageModelEdit)
	register(h, "video.generate", h.videoGenerate)
	register(h, "voice.list", h.voiceList)
	register(h, "library.memory.list", h.memoryList)
	register(h, "library.memory.get", h.memoryGet)
	register(h, "library.memory.write", h.memoryWrite)
	register(h, "library.memory.remove", h.memoryRemove)
	register(h, "library.skills.list", h.skillList)
	register(h, "library.skills.get", h.skillGet)
	register(h, "library.skills.write", h.skillWrite)
	register(h, "library.skills.import", h.skillImport)
	register(h, "library.skills.export", h.skillExport)
	register(h, "library.skills.review", h.skillReview)
	register(h, "library.skills.lint", h.skillLint)
	register(h, "library.prompts.list", h.promptList)
	register(h, "library.prompts.get", h.promptGet)
	register(h, "library.prompts.write", h.promptWrite)
	register(h, "library.prompts.render", h.promptRender)
	register(h, "library.search", h.librarySearch)
	register(h, "library.reindex", h.libraryReindex)
	register(h, "agents.profiles.list", h.agentProfilesList)
	register(h, "agents.profiles.set", h.agentProfilesSet)
	register(h, "agents.profiles.remove", h.agentProfilesRemove)
	register(h, "chat.sessions.start", h.chatStart)
	register(h, "chat.sessions.list", h.chatList)
	register(h, "chat.sessions.get", h.chatGet)
	register(h, "chat.sessions.events", h.chatStream)
	register(h, "chat.sessions.transcript", h.chatTranscript)
	register(h, "chat.sessions.send", h.chatSend)
	register(h, "chat.sessions.cancel", h.chatCancel)
	register(h, "chat.sessions.answer", h.chatAnswer)
	register(h, "chat.sessions.delete", h.chatDelete)
	register(h, "system.get", h.systemGet)
	register(h, "audit.list", h.auditList)
	register(h, "system.launch", h.systemLaunch)
	register(h, "system.scheduler", h.systemScheduler)
	register(h, "handshake.get", func(_ *web.Ctx, in handshakeIn) (handshakeOut, error) {
		if !auth.ValidNonce(in.Nonce) {
			return handshakeOut{}, &service.Error{
				Code: command.CodeValidation, Path: "nonce",
				Message: "the nonce must be 32 lower-case hexadecimal characters.",
			}
		}

		key := h.Token
		if h.Network != nil {
			key = h.RuntimeSecret
		}

		return handshakeOut{Proof: auth.Proof(key, in.Nonce)}, nil
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

// readDryRun reads dry_run as a JSON boolean, or the query alias ?dry_run=true
// (SDD-003 section 6.3). The binder copies that query text into the member and,
// for a bool field, also turns "yes", "no" and "1" into a boolean, so a string
// "no" would apply the write. The member is therefore untyped, and only true
// and false are accepted.
func readDryRun(c *web.Ctx, raw any) (bool, error) {
	query := c.Query("dry_run")
	if s, ok := raw.(string); ok && query != "" && s == query {
		raw = nil
	}

	if query != "" && query != "true" && query != "false" {
		return false, &service.Error{
			Code: command.CodeValidation, Path: "dry_run",
			Message: "dry_run must be true or false.",
			Hint:    "use the query dry_run=true or dry_run=false",
		}
	}

	member, err := jsonBool(raw, "dry_run")
	if err != nil {
		return false, err
	}

	return member || query == "true", nil
}

// jsonBool is a body member that must be a JSON boolean; absent is false. The query alias is text and is
// read by dry.
func jsonBool(v any, name string) (bool, error) {
	switch b := v.(type) {
	case nil:
		return false, nil
	case bool:
		return b, nil
	}

	return false, &service.Error{
		Code: command.CodeValidation, Path: name,
		Message: name + " must be true or false.", Hint: "send the JSON boolean true or false",
	}
}

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
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.ProjectCreateResult{}, err
	}

	if in.Width != nil || in.Height != nil {
		path := "width"
		if in.Width == nil {
			path = "height"
		}

		return service.ProjectCreateResult{}, &service.Error{
			Code: command.CodeValidation, Path: path,
			Message: "a project takes its size from preset, not from width and height.",
			Hint:    "send preset: 9:16 (default), 1:1, 16:9 or 4:5",
		}
	}

	res, err := h.Service.ProjectCreate(c.Context(), service.ProjectCreateRequest{
		Name: in.Name, Preset: in.Preset, FPS: string(in.FPS), ID: in.ID, DryRun: dryRun,
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
		ProjectID: in.ID, TimelineID: in.TimelineID, Detail: in.Detail, Page: page(in.pageIn),
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

	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.EditResult{}, err
	}

	res, err := h.Service.Edit(c.Context(), service.EditRequest{
		ProjectID: in.ID, TimelineID: in.TimelineID,
		BaseRevision:   in.BaseRevision,
		Operations:     ops,
		IdempotencyKey: key,
		DryRun:         dryRun,
	})
	if err != nil {
		return service.EditResult{}, err
	}

	revisionHeader(c, res.Revision)

	return *res, nil
}

func (h *apiHandler) step(
	c *web.Ctx,
	in stepIn,
	fn func(ctx context.Context, req service.UndoRequest) (*service.UndoResult, error),
) (service.UndoResult, error) {
	dryRun, err := readDryRun(c, in.DryRun)
	if err != nil {
		return service.UndoResult{}, err
	}

	res, err := fn(c.Context(), service.UndoRequest{ProjectID: in.ID, TimelineID: in.TimelineID, DryRun: dryRun})
	if err != nil {
		return service.UndoResult{}, err
	}

	revisionHeader(c, res.Revision)

	return *res, nil
}

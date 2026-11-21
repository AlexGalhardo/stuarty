package service

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/library"
	"github.com/AlexGalhardo/stuarty/service/providers"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/publish/destinations"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// Service is the single application boundary (ADR-0008): the UI, MCP, CLI and
// HTTP adapters call it. This is the part of the interface of SDD-003 section
// 2.1: projects, edits, history, lint, media, jobs, templates, fonts,
// bundles, OpenTimelineIO and system. Every method returns a *Error on
// failure and never a partial result with a non-nil error.
type Service interface {
	ProductionService
	WritingService
	BlogService
	BriefingService
	AssetLibraryService
	ChatService
	MotionService

	ProjectList(ctx context.Context, req ProjectListRequest) (*ProjectListResult, error)
	ProjectCreate(ctx context.Context, req ProjectCreateRequest) (*ProjectCreateResult, error)
	ProjectGet(ctx context.Context, req ProjectGetRequest) (*ProjectGetResult, error)
	ProjectTrash(ctx context.Context, req ProjectTrashRequest) (*ProjectStateResult, error)
	ProjectRestore(ctx context.Context, req ProjectTrashRequest) (*ProjectStateResult, error)

	Edit(ctx context.Context, req EditRequest) (*EditResult, error)
	CommandHelp(ctx context.Context, req CommandHelpRequest) (*CommandHelpResult, error)

	HistoryList(ctx context.Context, req HistoryListRequest) (*HistoryListResult, error)
	Undo(ctx context.Context, req UndoRequest) (*UndoResult, error)
	Redo(ctx context.Context, req UndoRequest) (*UndoResult, error)
	HistoryRevert(ctx context.Context, req HistoryRevertRequest) (*HistoryRevertResult, error)
	HistoryDiff(ctx context.Context, req HistoryDiffRequest) (*HistoryDiffResult, error)
	CheckpointCreate(ctx context.Context, req CheckpointCreateRequest) (*Checkpoint, error)
	CheckpointList(ctx context.Context, req CheckpointListRequest) (*CheckpointListResult, error)
	CheckpointRename(ctx context.Context, req CheckpointRenameRequest) (*Checkpoint, error)
	CheckpointRemove(ctx context.Context, req CheckpointRemoveRequest) (*Checkpoint, error)

	Lint(ctx context.Context, req LintRequest) (*LintResult, error)

	MediaImport(ctx context.Context, req MediaImportRequest) (*MediaImportResult, error)
	MediaInspect(ctx context.Context, req MediaInspectRequest) (*MediaInspectResult, error)
	MediaList(ctx context.Context, req MediaListRequest) (*MediaListResult, error)
	MediaTrash(ctx context.Context, req MediaTrashRequest) (*MediaStateResult, error)
	MediaRestore(ctx context.Context, req MediaTrashRequest) (*MediaStateResult, error)

	JobStart(ctx context.Context, req JobStartRequest) (*Job, error)
	ProjectBundleExport(ctx context.Context, req ProjectBundleExportRequest) (*Job, error)
	ProjectBundleImport(ctx context.Context, req ProjectBundleImportRequest) (*ProjectCreateResult, error)
	ProjectOtioExport(ctx context.Context, req ProjectOtioExportRequest) (*ProjectOtioExportResult, error)
	ProjectOtioImport(ctx context.Context, req ProjectOtioImportRequest) (*ProjectOtioImportResult, error)

	FontImport(ctx context.Context, req FontImportRequest) (*FontImportResult, error)
	BrandKitGet(ctx context.Context, req BrandKitGetRequest) (*BrandKit, error)
	BrandKitSet(ctx context.Context, req BrandKitSetRequest) (*BrandKit, error)

	// The library: memory notes, skills and prompts kept as files in the workspace (SDD-018). The text of every
	// entry is untrusted; a skill that a person has not reviewed is not offered to an agent.
	MemoryList(ctx context.Context, req MemoryListRequest) (*LibraryListResult, error)
	MemoryGet(ctx context.Context, req LibraryRef) (*MemoryGetResult, error)
	MemoryWrite(ctx context.Context, req MemoryWriteRequest) (*MemoryWriteResult, error)
	MemoryRemove(ctx context.Context, req LibraryRef) (*LibraryRemoveResult, error)
	SkillList(ctx context.Context, req SkillListRequest) (*LibraryListResult, error)
	SkillGet(ctx context.Context, req LibraryRef) (*SkillGetResult, error)
	SkillWrite(ctx context.Context, req SkillWriteRequest) (*SkillWriteResult, error)
	SkillImport(ctx context.Context, req SkillImportRequest) (*SkillImportResult, error)
	SkillMarkReviewed(ctx context.Context, req SkillReviewRequest) (*SkillReviewResult, error)
	SkillLint(ctx context.Context, req LibraryRef) (*SkillLintResult, error)
	SkillExport(ctx context.Context, req SkillExportRequest) (*SkillExportResult, error)
	PromptList(ctx context.Context, req PromptListRequest) (*LibraryListResult, error)
	PromptGet(ctx context.Context, req LibraryRef) (*PromptGetResult, error)
	PromptWrite(ctx context.Context, req PromptWriteRequest) (*PromptWriteResult, error)
	PromptRender(ctx context.Context, req PromptRenderRequest) (*PromptRenderResult, error)
	LibrarySearch(ctx context.Context, req LibrarySearchRequest) (*LibrarySearchResult, error)
	LibraryReindex(ctx context.Context, req LibraryReindexRequest) (*LibraryReindexResult, error)

	// The AI provider layer (SDD-016): the providers and their models, the price table, and a text generation,
	// which is a job of a kind that needs approval (SDD-015 section 5).
	ProviderList(ctx context.Context, req ProviderListRequest) (*ProviderListResult, error)
	ProviderModels(ctx context.Context, req ProviderModelsRequest) (*ProviderModelsResult, error)
	ProviderPriceSet(ctx context.Context, req ProviderPriceSetRequest) (*ProviderPricesResult, error)
	TextGenerate(ctx context.Context, req TextGenerateRequest) (*Job, error)
	ImageGenerate(ctx context.Context, req ImageGenerateRequest) (*Job, error)
	VoiceGenerate(ctx context.Context, req VoiceGenerateRequest) (*Job, error)
	VideoGenerate(ctx context.Context, req VideoGenerateRequest) (*Job, error)
	VoiceList(ctx context.Context, req VoiceListRequest) (*VoiceListResult, error)
	// ImageModelEdit removes the background of a layer, or generates an image into a document, with a provider
	// (SDD-016 section 6.5); a job of a kind that needs approval.
	ImageModelEdit(ctx context.Context, req ImageModelEditRequest) (*Job, error)
	// The sign-in to an AI provider in the browser (SDD-016 section 12): only a person begins and completes one.
	ProviderSignInBegin(ctx context.Context, req ProviderSignInBeginRequest) (*ProviderSignInBeginResult, error)
	ProviderSignInComplete(ctx context.Context, req ProviderSignInCompleteRequest) (*ProviderSignIn, error)
	ProviderSignInList(ctx context.Context, req ProviderSignInListRequest) (*ProviderSignInListResult, error)

	JobGet(ctx context.Context, req JobGetRequest) (*Job, error)
	JobList(ctx context.Context, req JobListRequest) (*JobListResult, error)
	JobCancel(ctx context.Context, req JobCancelRequest) (*Job, error)

	TemplateList(ctx context.Context, req TemplateListRequest) (*TemplateListResult, error)
	TemplateSave(ctx context.Context, req TemplateSaveRequest) (*TemplateResult, error)
	TemplateInstantiate(ctx context.Context, req TemplateInstantiateRequest) (*TemplateInstantiateResult, error)
	TemplateExport(ctx context.Context, req TemplateRef) (*FileResult, error)
	TemplateImport(ctx context.Context, req TemplateImportRequest) (*TemplateResult, error)

	// SnapPoints, FontList, ExportFile and AuditList are read operations of the
	// editor and of the administrator (SDD-000 section 1).
	SnapPoints(ctx context.Context, req SnapPointsRequest) (*SnapPointsResult, error)
	FontList(ctx context.Context, req FontListRequest) (*FontListResult, error)
	// AssetList lists the files the projects hold, by content hash, for the picker of a brand logo (VV-117).
	AssetList(ctx context.Context, req AssetListRequest) (*AssetListResult, error)
	ExportFile(ctx context.Context, req ExportFileRequest) (*File, error)
	// ExportPresetList, ExportCheck and ExportBatch are the export presets, the check against one and the batch of
	// exports of SDD-004 sections 8.2 to 8.4 (RF-087 to RF-089).
	ExportPresetList(ctx context.Context, req ExportPresetListRequest) (*ExportPresetListResult, error)
	ExportCheck(ctx context.Context, req ExportCheckRequest) (*ExportCheckResult, error)
	ExportBatch(ctx context.Context, req ExportBatchRequest) (*Job, error)
	// AudioMeasure is the loudness, true peak and noise floor of a range or an item with and without its audio
	// processors (SDD-004 section 7.1, epic 34).
	AudioMeasure(ctx context.Context, req AudioMeasureRequest) (*AudioMeasureResult, error)
	AuditList(ctx context.Context, req AuditListRequest) (*AuditListResult, error)

	// Doctor is the diagnostic report (SDD-003 section 2, SDD-011 section 8.6).
	Doctor(ctx context.Context, req DoctorRequest) (*DoctorResult, error)

	// TrashEmpty deletes what is in the trash for good; HTTP only, for a
	// browser session or an admin key (SDD-007 section 5.3).
	TrashEmpty(ctx context.Context, req TrashEmptyRequest) (*TrashEmptyResult, error)

	// ToolsInstall downloads the transcription helper and its model; it is the
	// only operation that does, and only a person can call it (SDD-007 6.8).
	ToolsInstall(ctx context.Context, req ToolsInstallRequest) (*ToolsInstallResult, error)
	ToolsStatus(ctx context.Context, req ToolsStatusRequest) (*ToolsStatusResult, error)

	// ViewFrame and ViewSheet render the current revision through the export
	// executor (SDD-003 section 2.7).
	ViewFrame(ctx context.Context, req ViewFrameRequest) (*ViewResult, error)
	ViewSheet(ctx context.Context, req ViewSheetRequest) (*ViewResult, error)

	// PlanWindow reads the render plan of a window of frames (SDD-004 section 10).
	PlanWindow(ctx context.Context, req PlanWindowRequest) (*PlanWindowResult, error)

	// OpenFile resolves a stored file of an asset for download; the caller
	// serves the bytes.
	OpenFile(ctx context.Context, ref FileRef) (*File, error)

	// KeyCreate, KeyList and KeyRevoke manage the API keys of network mode, for
	// a person who logged in; a key cannot call them (SDD-007 section 4.2).
	KeyCreate(ctx context.Context, req KeyCreateRequest) (*KeyCreateResult, error)
	KeyList(ctx context.Context, req KeyListRequest) (*KeyListResult, error)
	KeyRevoke(ctx context.Context, req KeyRevokeRequest) (*Key, error)

	// The image documents of SDD-021: creation (blank or from a template), the list, the text view, batches of
	// image operations, undo and redo, and the export to a file.
	ImageCreate(ctx context.Context, req ImageCreateRequest) (*ImageCreateResult, error)
	ImageList(ctx context.Context, req ImageListRequest) (*ImageListResult, error)
	ImageDescribe(ctx context.Context, req ImageDescribeRequest) (*ImageDescribeResult, error)
	ImageEdit(ctx context.Context, req ImageEditRequest) (*ImageEditResult, error)
	ImageUndo(ctx context.Context, req ImageStepRequest) (*ImageStepResult, error)
	ImageRedo(ctx context.Context, req ImageStepRequest) (*ImageStepResult, error)
	// ImageRevert and the checkpoints of an image are those of a timeline (SDD-003 section 2.5.1) for an image
	// document (SDD-021 section 4).
	ImageRevert(ctx context.Context, req ImageRevertRequest) (*ImageRevertResult, error)
	ImageDiff(ctx context.Context, req ImageDiffRequest) (*ImageDiffResult, error)
	ImageCheckpointCreate(ctx context.Context, req ImageCheckpointCreateRequest) (*Checkpoint, error)
	ImageCheckpointList(ctx context.Context, req ImageCheckpointListRequest) (*ImageCheckpointListResult, error)
	ImageCheckpointRename(ctx context.Context, req ImageCheckpointRenameRequest) (*Checkpoint, error)
	ImageCheckpointRemove(ctx context.Context, req ImageCheckpointRemoveRequest) (*Checkpoint, error)
	ImageExport(ctx context.Context, req ImageExportRequest) (*ImageExportResult, error)
	// ImageImport brings an image file into an image document: a new one, or a layer of ImageID (SDD-021 section 10.1).
	ImageImport(ctx context.Context, req ImageImportRequest) (*ImageImportResult, error)
	// ImageImportFile is ImageImport for a file that arrives as the bytes of a request (SDD-021 section 10.2); HTTP only.
	ImageImportFile(ctx context.Context, req ImageImportFileRequest) (*ImageImportResult, error)
	// ImageFrame, ImageHistory and ImageFile are the reads of the image editor screen (SDD-012 section 24.3); HTTP only.
	ImageFrame(ctx context.Context, req ImageFrameRequest) (*ImageFrameResult, error)
	ImageHistory(ctx context.Context, req ImageHistoryRequest) (*ImageHistoryResult, error)
	ImageFile(ctx context.Context, req ImageFileRequest) (*File, error)
	// ConnectionList and ConnectionGet read the destinations the user connected; ConnectionCreate,
	// ConnectionRemove and ConnectionTest are for an administrator. No result carries a credential
	// (SDD-014 section 5).
	ConnectionList(ctx context.Context, req ConnectionListRequest) (*ConnectionListResult, error)
	ConnectionGet(ctx context.Context, req ConnectionRef) (*connections.Connection, error)
	ConnectionCreate(ctx context.Context, req ConnectionCreateRequest) (*connections.Connection, error)
	ConnectionRemove(ctx context.Context, req ConnectionRef) (*connections.Connection, error)
	ConnectionTest(ctx context.Context, req ConnectionRef) (*ConnectionTestResult, error)

	// The approval queue, its rules and the cost ceilings (SDD-015). Reading is for every caller that
	// may read; ApprovalDecide, ApprovalRuleSet, ApprovalRuleRemove and CostCeilingSet are for a
	// person in a privileged browser session and answer E_PRIVILEGED_SESSION to everyone else.
	ApprovalList(ctx context.Context, req ApprovalListRequest) (*ApprovalListResult, error)
	ApprovalGet(ctx context.Context, req ApprovalRef) (*approval.Request, error)
	ApprovalDecide(ctx context.Context, req ApprovalDecideRequest) (*approval.Request, error)
	ApprovalRuleList(ctx context.Context, req ApprovalRuleListRequest) (*ApprovalRuleListResult, error)
	ApprovalRuleSet(ctx context.Context, req ApprovalRuleSetRequest) (*approval.RuleView, error)
	ApprovalRuleRemove(ctx context.Context, req ApprovalRef) (*approval.RuleView, error)
	CostSummary(ctx context.Context, req CostSummaryRequest) (*CostSummaryResult, error)
	CostCeilingSet(ctx context.Context, req CostCeilingSetRequest) (*CostSummaryResult, error)

	// The social networks and publishing (SDD-020). A publication is a job of a kind that needs approval:
	// PublishNow answers a job in waiting_approval, and nothing leaves the machine before a person approves.
	SocialNetworkList(ctx context.Context, req SocialNetworkListRequest) (*SocialNetworkListResult, error)
	SocialConnectBegin(ctx context.Context, req SocialConnectBeginRequest) (*SocialConnectBeginResult, error)
	SocialConnectComplete(ctx context.Context, req SocialConnectCompleteRequest) (*SocialAccount, error)
	SocialAccountList(ctx context.Context, req SocialAccountListRequest) (*SocialAccountListResult, error)
	SocialAccountSelect(ctx context.Context, req SocialAccountSelectRequest) (*SocialAccount, error)
	SocialPriceSet(ctx context.Context, req SocialPriceSetRequest) (*SocialAccount, error)
	PublishValidate(ctx context.Context, req PublishValidateRequest) (*PublishValidateResult, error)
	PublishNow(ctx context.Context, req PublishNowRequest) (*Job, error)
	PublishStatus(ctx context.Context, req PublishStatusRequest) (*PublishStatusResult, error)

	// The calendar of publications (SDD-020 sections 23 to 31): a publication with one target per account, each
	// with its time. PublicationSchedule, PublicationSendNow and PublicationRetry only ask for approval: a
	// target is sent by the scheduler, after a person or a rule the person wrote approved it.
	PublicationCreate(ctx context.Context, req PublicationCreateRequest) (*Publication, error)
	PublicationGet(ctx context.Context, req PublicationGetRequest) (*Publication, error)
	PublicationList(ctx context.Context, req PublicationListRequest) (*PublicationListResult, error)
	PublicationUpdate(ctx context.Context, req PublicationUpdateRequest) (*Publication, error)
	PublicationSchedule(ctx context.Context, req PublicationScheduleRequest) (*Publication, error)
	PublicationSendNow(ctx context.Context, req PublicationSendNowRequest) (*Publication, error)
	PublicationRetry(ctx context.Context, req PublicationRetryRequest) (*Publication, error)
	PublicationCancel(ctx context.Context, req PublicationCancelRequest) (*Publication, error)
}

// Event is what the service publishes after the change is durable (SDD-006
// section 3). The hub that carries it is the business of the events task; the
// service only calls Options.Publish.
type Event struct {
	// Type is "batch.committed", "project.updated", "nested.updated", "job.updated" or
	// "asset.updated".
	Type        string
	ProjectID   string
	Revision    int64
	Kind        string // edit, undo or redo, for batch.committed; the job kind for job.updated
	Actor       string
	Description string
	At          time.Time

	// The members below are set by job.updated and asset.updated, which are
	// not replayable and carry no revision.
	JobID   string
	AssetID string // the asset id of this project, for asset.updated
	// TimelineID is the timeline that changed, for nested.updated: ProjectID is then a timeline that holds it.
	TimelineID string
	State      string
	Progress   float64
	Message    string
	Error      *Error
	Result     *JobResult
	// Approval is the request of approval.requested and approval.decided (SDD-015 section 7), and
	// Owner the user it belongs to in network mode, so that a member's stream carries their own only.
	Approval *approval.Request
	Owner    string
	// Publication is the publication of publication.updated (SDD-020 section 29).
	Publication *pub.Publication

	// The members below are set by image.updated (SDD-006 section 12): ImageID names the document, and
	// Revision, Kind, Actor and At are those of the committed batch.
	ImageID      string
	BaseRevision int64
	Diff         json.RawMessage
}

// EventImageUpdated is the type of the event a committed batch of an image document publishes.
const EventImageUpdated = "image.updated"

// Options configure New. Store is required.
type Options struct {
	Store Store
	// Registry is the command registry of the build; default command.Default().
	Registry *command.Registry
	// Now is the service clock (SDD-005 section 2 item 2); default time.Now.
	Now func() time.Time
	// NewIDs makes identifiers; default timeline.NewID (cryptographic).
	NewIDs func(prefix string) string
	// Publish receives events after the commit, in revision order per project.
	Publish func(Event)
	// Lint is the environment of the lint rules. SafeArea defaults to
	// plan.SafeArea; TextSize is the text Measurer that production supplies
	// (web/app/owner/lintmeasure.go); while it is nil, L_SAFE_AREA is skipped.
	Lint lint.Env

	// Workspace is the workspace directory: the content store, the artifacts
	// and the exports are under it.
	Workspace string
	// Media is the import and ingest machinery; see Media.
	Media Media
	// Exporter renders and verifies exports; JobStart for an export answers
	// E_INTERNAL without one.
	Exporter Exporter
	// Enqueue delivers a job to the workers. Without one the job rows are
	// created and nothing runs them.
	Enqueue Enqueuer
	// Limits are the ceilings of SDD-007 section 9.1.
	Limits Limits
	// Version is the binary version written into the template files.
	Version string
	// BuiltInSkills is the set of skills that ship with the product, one folder per skill; the service
	// installs it into the library at start (SDD-018 section 16). Nil installs none.
	BuiltInSkills fs.FS
	// Transcriber is the transcription provider of the transcribe job; without
	// one JobStart for it answers E_TRANSCRIBE_UNAVAILABLE.
	Transcriber transcribe.Provider
	// Tools is the install step of the helper; nil means it is not available.
	Tools *Tools
	// Doctor is what the diagnostic report learns from the process.
	Doctor DoctorOptions
	// Keys stores the API keys of network mode; without one the key operations
	// answer E_NOT_FOUND, as in local mode.
	Keys KeyStore
	// Connections is the registry of the destinations the user connected and the holder of their
	// credentials; without one the service builds a registry over the store with a vault in the
	// workspace that has no master key source, so a connection cannot be created (SDD-014).
	Connections *connections.Manager
	// Social is the registry of the social network connectors (SDD-020); without one the build publishes
	// to no network and the social operations answer empty lists.
	Social *social.Registry
	// Blog is the registry of the blog destinations (SDD-020 part 5); without one the service has the five of the
	// build.
	Blog *destinations.Registry
	// Providers is the registry of the AI providers, with their adapters registered (SDD-016); without one
	// the service has a registry with no provider, so no generation can start.
	Providers *providers.Registry
	// SignIns are the sign-ins to an AI provider of this build (SDD-016 section 12); without any the
	// sign-in operations answer that none is built in, and no credential is ever renewed.
	SignIns *oauth.SignIns
	// JobKinds are the job kinds that need approval, registered by the epics of publishing and
	// generation (SDD-015 section 5). The product registers none of its own; tests register one.
	JobKinds []ApprovableKind
	// Heavy limits the heavy operations (jobs, media, template, font, bundle and OpenTimelineIO imports and exports) per actor; nil counts nothing.
	Heavy RateLimiter
	// HeavyPerMinute is the limit Heavy enforces, for the error message; zero is HeavyPerMinute.
	HeavyPerMinute int
	// Log receives the job lines (start, end, failure); nil discards them.
	Log *slog.Logger
	// WebP encodes the WebP export of an image; without one that format answers webp_unavailable (SDD-021 section 6).
	WebP export.WebPEncoder
	// JobWait is how long TrashEmpty waits for the jobs of a trashed project to stop; zero is 15 s.
	JobWait time.Duration
	// SchedulerClock is the clock the publication scheduler sleeps on; tests move it. Default: the service
	// clock to read and the system timer to wait.
	SchedulerClock pub.Clock
	// Agents hosts the agent a user installed, for the chat (SDD-017). Nil in a process that only runs a
	// command: the chat operations that need a process then answer E_AGENT_UNAVAILABLE, and the reads work.
	Agents *agenthost.Host
}

type svc struct {
	store       Store
	reg         *command.Registry
	now         func() time.Time
	newIDs      func(string) string
	publish     func(Event)
	lintEnv     lint.Env
	workspace   string
	media       Media
	exporter    Exporter
	enqueue     Enqueuer
	limits      Limits
	version     string
	transcriber transcribe.Provider
	tools       *Tools
	toolsState  toolsState
	doctor      DoctorOptions
	keys        KeyStore
	conns       *connections.Manager
	kinds       map[string]*ApprovableKind
	social      *social.Registry
	blog        *destinations.Registry
	flows       *oauth.Flows
	signInSet   *oauth.SignIns
	signFlows   *oauth.Flows
	// imageFrames keeps the pictures of the lower layers of an image document between two frames of the editor,
	// so an edit of the layer on top redraws only that layer (SDD-021 section 6.1).
	imageFrames *export.ImageCache
	providers   *providers.Registry
	lib         *library.Manager // the files of memory, skills and prompts; nil without a workspace
	heavy       RateLimiter
	heavyLimitN int
	log         *slog.Logger
	jobWait     time.Duration
	agents      *agenthost.Host // the hosted agent chat; nil without a host (SDD-017)
	webp        export.WebPEncoder
	doctorMu    sync.Mutex
	doctorCaps  *ffmpeg.Capabilities

	// pubs is the calendar of publications and sched the loop that sends the approved ones at their time
	// (SDD-020 sections 23 to 29).
	pubs  *pub.Manager
	sched *pub.Scheduler

	storeMu sync.RWMutex

	// startedAt is when this service was made: a bundle temp file older than it belongs to a process
	// that died (SDD-005 section 13.4). The wall clock, because file times are wall clock.
	startedAt time.Time

	running registry
	bitmaps bitmaps
	// previews are the processed sounds of the timeline preview that a plan window handed out (SDD-004 section 7.1).
	previews audioPreviews
	fonts    *enginestore.Fonts // user fonts of the workspace; nil without one
	snaps    snapCache
	sems     map[string]chan struct{}
	// peaksGate admits one computation of peaks on request at a time (SDD-011 section 6.5).
	peaksGate chan struct{}

	// defaultIngest is true when New installed ingest.Run. A substitute ingester
	// does not touch the runner, so a nil runner is not a missing tool.
	defaultIngest bool
}

// New returns the in-process Service over a store.
func New(o Options) (Service, error) {
	if o.Store == nil {
		return nil, errors.New("service: Options.Store is required")
	}

	// The first text of the process would wait for the renderer module to compile (SDD-013 section 4.6), which
	// is a one-time cost of about a second; starting it now keeps it out of the first view (NFR-004).
	go rasterize.Warm()

	s := &svc{
		store:       o.Store,
		reg:         o.Registry,
		now:         o.Now,
		newIDs:      o.NewIDs,
		publish:     o.Publish,
		lintEnv:     o.Lint,
		workspace:   o.Workspace,
		media:       o.Media,
		exporter:    o.Exporter,
		enqueue:     o.Enqueue,
		limits:      o.Limits.withDefaults(),
		version:     o.Version,
		transcriber: o.Transcriber,
		tools:       o.Tools,
		doctor:      o.Doctor,
		keys:        o.Keys,
		conns:       o.Connections,
		social:      o.Social,
		blog:        o.Blog,
		signInSet:   o.SignIns,
		heavy:       o.Heavy,
		heavyLimitN: o.HeavyPerMinute,
		log:         o.Log,
		imageFrames: export.NewImageCache(0),
		jobWait:     o.JobWait,
		agents:      o.Agents,
		webp:        o.WebP,
		startedAt:   time.Now(),
	}

	s.providers = o.Providers

	kinds, err := registerKinds(append(slices.Clone(o.JobKinds),
		s.textGenerateKind(), s.imageGenerateKind(), s.voiceGenerateKind(), s.textDraftKind(),
		s.videoGenerateKind()))
	if err != nil {
		return nil, err
	}

	s.kinds = kinds
	s.registerPublishKind()
	s.registerImageEditKind()

	if s.workspace != "" {
		s.fonts = enginestore.NewFonts(s.workspace)
		s.lib = library.New(filepath.Join(s.workspace, "library"), o.Store.Library(), o.Now)
	}

	if s.conns == nil {
		s.conns = connections.NewDefault(o.Store.Connections(), s.workspace, s.now)
	}

	if s.social != nil {
		s.social.Bind(s.conns)
	}

	if s.blog == nil {
		s.blog = destinations.NewRegistry(destinations.Config{})
	}

	s.blog.Bind(s.conns)

	if s.providers == nil {
		prices := ""
		if s.workspace != "" {
			prices = filepath.Join(s.workspace, "providers", providers.PriceFile)
		}

		s.providers = providers.NewRegistry(s.conns, providers.NewPriceBook(prices), s.now)
	}

	s.sems = map[string]chan struct{}{
		JobIngest:     make(chan struct{}, s.limits.MaxIngests),
		JobExport:     make(chan struct{}, s.limits.MaxExports),
		JobTranscribe: make(chan struct{}, s.limits.MaxTranscribes),
	}
	s.peaksGate = make(chan struct{}, 1)

	if s.media.Ingest == nil {
		s.media.Ingest = ingest.Run
		s.defaultIngest = true
	}

	if s.media.FreeBytes == nil {
		s.media.FreeBytes = workspace.FreeBytes
	}

	if s.reg == nil {
		s.reg = command.Default()
	}

	if s.now == nil {
		s.now = time.Now
	}

	s.flows = oauth.NewFlows(s.now)
	s.signFlows = oauth.NewFlows(s.now)

	// A sign-in credential is renewed by the manager before a closure gets it (SDD-014 section 13).
	if s.signInSet != nil {
		s.conns.SetRefresher(oauth.NewRefresher(s.signInSet, s.now))
	}

	if s.jobWait <= 0 {
		s.jobWait = jobWait
	}

	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}

	if s.lib != nil && o.BuiltInSkills != nil {
		// A failure here must not stop the product: the library works without the built-in skills.
		if _, err := s.lib.InstallBuiltInSet(context.Background(), o.BuiltInSkills, s.version); err != nil {
			s.log.Warn("the built-in skills were not installed", "error", err)
		}
	}

	if s.newIDs == nil {
		s.newIDs = timeline.NewID
	}

	if s.lintEnv.SafeArea == nil {
		s.lintEnv.SafeArea = func(w, h int) (x, y, sw, sh int) {
			r := plan.SafeArea(w, h)
			return int(r.X), int(r.Y), int(r.W), int(r.H)
		}
	}

	s.pubs = pub.NewManager(pubEnv{s}, pubGateway{s})

	clock := o.SchedulerClock
	if clock == nil {
		clock = svcClock{now: s.now}
	}

	s.sched = pub.NewScheduler(s.pubs, pub.SchedulerOptions{Clock: clock, Log: s.log, Housekeeping: s.expireDue})

	return s, nil
}

func (s *svc) nowMs() int64 { return s.now().UnixMilli() }

// emit publishes after the commit; a nil Publish is no event.
func (s *svc) emit(e Event) {
	if s.publish != nil {
		s.publish(e)
	}
}

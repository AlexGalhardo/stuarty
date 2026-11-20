package service

import (
	"context"
	"slices"
)

// Scopes of a caller (ADR-0019, SDD-007 section 4).
const (
	ScopeRead   = "read"
	ScopeEdit   = "edit"
	ScopeMedia  = "media"
	ScopeRender = "render"
	ScopeAdmin  = "admin"
)

// AllScopes is the five scopes: what a local caller holds.
var AllScopes = []string{ScopeRead, ScopeEdit, ScopeMedia, ScopeRender, ScopeAdmin}

// Caller is what authentication established (SDD-007 section 5.1) and the
// adapters put in the context.
type Caller struct {
	Actor       string   // "user:<id>", "key:<key id>" or "local:<label>" (SDD-000 section 3)
	Scopes      []string // subset of read, edit, media, render, admin
	OwnerUserID string   // network mode: the user who owns the key or session; empty in local mode
	ProjectID   string   // a key limited to one project; empty means any
	// Browser is true for a person at the editor, who proved it with the
	// session cookie. The local token and the agent surfaces are not.
	Browser bool
	// Operator is true for the person at the terminal of the owner machine, set
	// only by the CLI command tools:install after its confirmation. It is the
	// third caller class that may start the install step.
	Operator bool
}

type callerKey struct{}

// WithCaller returns a context carrying c.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFrom returns the caller of the context.
func CallerFrom(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)
	return c, ok && c.Actor != ""
}

// Scopes is the scope each method needs (SDD-007 section 5.1 item 2). A test
// lists the methods of the Service interface and fails when one has no entry.
var Scopes = map[string]string{
	"ProjectList":         ScopeRead,
	"ProjectCreate":       ScopeEdit,
	"ProjectGet":          ScopeRead,
	"ProjectTrash":        ScopeEdit,
	"ProjectRestore":      ScopeEdit,
	"Edit":                ScopeEdit,
	"CommandHelp":         ScopeRead,
	"HistoryList":         ScopeRead,
	"Undo":                ScopeEdit,
	"Redo":                ScopeEdit,
	"HistoryRevert":       ScopeEdit,
	"HistoryDiff":         ScopeRead,
	"CheckpointCreate":    ScopeEdit,
	"CheckpointList":      ScopeRead,
	"CheckpointRename":    ScopeEdit,
	"CheckpointRemove":    ScopeEdit,
	"Lint":                ScopeRead,
	"MediaImport":         ScopeMedia,
	"MediaInspect":        ScopeRead,
	"MediaList":           ScopeRead,
	"MediaTrash":          ScopeMedia,
	"MediaRestore":        ScopeMedia,
	"JobStart":            ScopeRender,
	"ExportPresetList":    ScopeRead,
	"ExportCheck":         ScopeRead,
	"AudioMeasure":        ScopeRead,
	"ExportBatch":         ScopeRender,
	"ProjectBundleExport": ScopeRender,
	"ProjectBundleImport": ScopeEdit,
	"ProjectOtioExport":   ScopeRead,
	"ProjectOtioImport":   ScopeEdit,
	"FontImport":          ScopeMedia,
	"BrandKitGet":         ScopeRead,
	"BrandKitSet":         ScopeEdit,
	"MemoryList":          ScopeRead,
	"MemoryGet":           ScopeRead,
	"MemoryWrite":         ScopeEdit,
	"MemoryRemove":        ScopeEdit,
	"SkillList":           ScopeRead,
	"SkillGet":            ScopeRead,
	"SkillWrite":          ScopeEdit,
	"SkillImport":         ScopeMedia,
	"SkillMarkReviewed":   ScopeEdit,
	"SkillLint":           ScopeRead,
	"SkillExport":         ScopeMedia,
	"PromptList":          ScopeRead,
	"PromptGet":           ScopeRead,
	"PromptWrite":         ScopeEdit,
	"PromptRender":        ScopeRead,
	"LibrarySearch":       ScopeRead,
	"LibraryReindex":      ScopeEdit,
	"JobGet":              ScopeRead,
	"JobList":             ScopeRead,
	"JobCancel":           ScopeRender,

	"TemplateList":        ScopeRead,
	"TemplateSave":        ScopeEdit,
	"TemplateInstantiate": ScopeEdit,
	"TemplateExport":      ScopeRead,
	"TemplateImport":      ScopeEdit,
	"TrashEmpty":          ScopeAdmin,
	"Doctor":              ScopeRead,
	"SnapPoints":          ScopeRead,
	"FontList":            ScopeRead,
	"AssetList":           ScopeRead,
	"ExportFile":          ScopeRead,
	"AuditList":           ScopeAdmin,
	"ToolsInstall":        ScopeAdmin,
	"ToolsStatus":         ScopeRead,
	"PlanWindow":          ScopeRead,
	"ViewFrame":           ScopeRead,
	"ViewSheet":           ScopeRead,
	"OpenFile":            ScopeRead,
	"KeyCreate":           ScopeRead,
	"KeyList":             ScopeRead,
	"KeyRevoke":           ScopeRead,

	// The hosted agent chat (SDD-017 section 10). The operations that act are also for a person only: that is
	// checked before the scope.
	"AgentProfileList":     ScopeRead,
	"AgentProfileSet":      ScopeAdmin,
	"AgentProfileRemove":   ScopeAdmin,
	"ChatSessionStart":     ScopeEdit,
	"ChatSend":             ScopeEdit,
	"ChatCancel":           ScopeEdit,
	"ChatAnswerPermission": ScopeEdit,
	"ChatSessionDelete":    ScopeEdit,
	"ChatSessionList":      ScopeRead,
	"ChatSessionGet":       ScopeRead,
	"ChatTranscriptGet":    ScopeRead,

	"ProjectDescribe":    ScopeRead,
	"ProjectDuplicate":   ScopeEdit,
	"ProjectItemAdd":     ScopeEdit,
	"ProjectItemMove":    ScopeEdit,
	"ProjectItemRename":  ScopeEdit,
	"ProjectItemTrash":   ScopeEdit,
	"ProjectItemRestore": ScopeEdit,
	"BriefGet":           ScopeRead,
	"BriefSet":           ScopeEdit,
	"BriefingQuestions":  ScopeRead,
	"BriefingAnswer":     ScopeEdit,
	"ImageCreate":        ScopeEdit,
	"ImageList":          ScopeRead,
	"ImageDescribe":      ScopeRead,
	"ImageEdit":          ScopeEdit,
	"ImageUndo":          ScopeEdit,
	"ImageRedo":          ScopeEdit,
	"ImageExport":        ScopeEdit,
	"ImageFrame":         ScopeRead,
	"ImageHistory":       ScopeRead,
	"ImageFile":          ScopeRead,
	"ConnectionList":     ScopeRead,
	"ConnectionGet":      ScopeRead,
	"ConnectionCreate":   ScopeAdmin,
	"ConnectionRemove":   ScopeAdmin,
	"ConnectionTest":     ScopeAdmin,
	// ApprovalDecide is read plus Caller.Browser (SDD-015 section 4), as KeyCreate is.
	"ApprovalList":       ScopeRead,
	"ApprovalGet":        ScopeRead,
	"ApprovalDecide":     ScopeRead,
	"ApprovalRuleList":   ScopeRead,
	"ApprovalRuleSet":    ScopeAdmin,
	"ApprovalRuleRemove": ScopeAdmin,
	// The social networks and publishing (SDD-020 section 9).
	"SocialNetworkList":     ScopeRead,
	"SocialConnectBegin":    ScopeAdmin,
	"SocialConnectComplete": ScopeAdmin,
	"SocialAccountList":     ScopeRead,
	"SocialAccountSelect":   ScopeAdmin,
	"PublishValidate":       ScopeRead,
	"PublishNow":            ScopeRender,
	"PublishStatus":         ScopeRead,
	// The calendar of publications (SDD-020 section 28). Scheduling, sending now and retrying only ask for
	// approval, so they take the scope of PublishNow.
	"PublicationCreate":   ScopeEdit,
	"PublicationGet":      ScopeRead,
	"PublicationList":     ScopeRead,
	"PublicationUpdate":   ScopeEdit,
	"PublicationSchedule": ScopeRender,
	"PublicationSendNow":  ScopeRender,
	"PublicationRetry":    ScopeRender,
	"PublicationCancel":   ScopeEdit,
	"CostSummary":         ScopeRead,
	"CostCeilingSet":      ScopeAdmin,

	"AssetGet":     ScopeRead,
	"AssetUses":    ScopeRead,
	"AssetSetInfo": ScopeMedia,
	"AssetTag":     ScopeMedia,

	"ProviderList":     ScopeRead,
	"ProviderModels":   ScopeRead,
	"ProviderPriceSet": ScopeAdmin,
	"TextGenerate":     ScopeEdit,
	"ImageGenerate":    ScopeEdit,
	"VoiceGenerate":    ScopeEdit,
	"VoiceList":        ScopeRead,
	"TextList":         ScopeRead,
	"TextGet":          ScopeRead,
	"TextWrite":        ScopeEdit,
	"TextRevisions":    ScopeRead,
	"TextRevert":       ScopeEdit,
	"TextDraft":        ScopeEdit,
	"CaptionValidate":  ScopeRead,
	"ScriptEstimate":   ScopeRead,

	"ProviderSignInBegin":    ScopeAdmin,
	"ProviderSignInComplete": ScopeAdmin,
	"ProviderSignInList":     ScopeRead,

	"MotionBrief":   ScopeRead,
	"MotionCheck":   ScopeRead,
	"MotionReplace": ScopeEdit,
}

// authorize checks the caller first, as every method does: a missing caller
// is E_UNAUTHORIZED, a missing scope or a key limited to another project is
// E_FORBIDDEN. projectID is "" for operations that do not name a project; an
// operation that names one calls s.authorizeProject, since the id it receives
// may be a timeline of the key's project (SDD-003 section 1.7 rule 4).
func authorize(ctx context.Context, method, projectID string) (Caller, error) {
	scope, listed := Scopes[method]
	if !listed {
		scope = ""
	}

	return authorizeScope(ctx, scope, projectID)
}

// authorizeScope is authorize for a method whose scope depends on the request:
// JobStart needs render for an export and media for a transcription. An empty
// scope is one that no caller holds.
func authorizeScope(ctx context.Context, scope, projectID string) (Caller, error) {
	c, ok := CallerFrom(ctx)
	if !ok {
		return Caller{}, newError(CodeUnauthorized, "the request carries no credentials.").
			withHint("send the local token or an API key")
	}

	if scope == "" || !slices.Contains(c.Scopes, scope) {
		// details.scope names the missing scope (SDD-003 §6.2); the refusal about
		// the project below carries none.
		return Caller{}, newError(CodeForbidden, "the caller lacks the %q scope.", scope).
			withHint("use a key with the %s scope", scope).
			withDetail("scope", scope)
	}

	if c.ProjectID != "" && projectID != "" && c.ProjectID != projectID {
		return Caller{}, newError(CodeForbidden, "the key is limited to another project.").
			withHint("use a key that covers this project")
	}

	return c, nil
}

// authorizeProject is authorize for an operation that names a project or, in its place, a timeline (SDD-003
// section 1.7 rule 2): a key limited to one project reaches every timeline of it (rule 4), so the id is resolved to
// its project before it is held to the key, and before anything of that project is read. A timeline of another
// project and an id of nothing are refused alike, with the refusal authorize gives a project outside the key.
func (s *svc) authorizeProject(ctx context.Context, method, id string) (Caller, error) {
	return authorize(ctx, method, s.keyProjectOf(ctx, id))
}

// authorizeProjectScope is authorizeProject for a method whose scope depends on the request.
func (s *svc) authorizeProjectScope(ctx context.Context, scope, id string) (Caller, error) {
	return authorizeScope(ctx, scope, s.keyProjectOf(ctx, id))
}

// keyProjectOf is the project an id stands for, as a key limited to one project is held to it: the project of a
// timeline, or the id itself when it is no timeline (a project made before ADR-0039, or nothing). For any other
// caller the id is returned as it is and nothing is read.
func (s *svc) keyProjectOf(ctx context.Context, id string) string {
	if c, ok := CallerFrom(ctx); !ok || c.ProjectID == "" || id == "" {
		return id
	}

	if owner, err := s.store.Productions().ProjectOfTimeline(ctx, id); err == nil {
		return owner
	}

	return id
}

// Roles of network mode (SDD-007 section 4.1).
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// RoleScopes is what a role may do: an administrator holds the five scopes, a
// member the four that work on own projects. An unknown role holds none.
func RoleScopes(role string) []string {
	switch role {
	case RoleAdmin:
		return slices.Clone(AllScopes)
	case RoleMember:
		return []string{ScopeRead, ScopeEdit, ScopeMedia, ScopeRender}
	}

	return nil
}

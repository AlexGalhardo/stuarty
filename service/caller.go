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
	"ProjectList":    ScopeRead,
	"ProjectCreate":  ScopeEdit,
	"ProjectGet":     ScopeRead,
	"ProjectTrash":   ScopeEdit,
	"ProjectRestore": ScopeEdit,
	"Edit":           ScopeEdit,
	"CommandHelp":    ScopeRead,
	"HistoryList":    ScopeRead,
	"Undo":           ScopeEdit,
	"Redo":           ScopeEdit,
	"Lint":           ScopeRead,
	"MediaImport":    ScopeMedia,
	"MediaInspect":   ScopeRead,
	"MediaList":      ScopeRead,
	"MediaTrash":     ScopeMedia,
	"MediaRestore":   ScopeMedia,
	"JobStart":       ScopeRender,
	"JobGet":         ScopeRead,
	"JobList":        ScopeRead,
	"JobCancel":      ScopeRender,

	"TemplateList":        ScopeRead,
	"TemplateSave":        ScopeEdit,
	"TemplateInstantiate": ScopeEdit,
	"TemplateExport":      ScopeRead,
	"TemplateImport":      ScopeEdit,
	"TrashEmpty":          ScopeAdmin,
	"PlanWindow":          ScopeRead,
	"ViewFrame":           ScopeRead,
	"ViewSheet":           ScopeRead,
	"OpenFile":            ScopeRead,
}

// authorize checks the caller first, as every method does: a missing caller
// is E_UNAUTHORIZED, a missing scope or a key limited to another project is
// E_FORBIDDEN. projectID is "" for operations that do not name a project.
func authorize(ctx context.Context, method, projectID string) (Caller, error) {
	c, ok := CallerFrom(ctx)
	if !ok {
		return Caller{}, newError(CodeUnauthorized, "the request carries no credentials.").
			withHint("send the local token or an API key")
	}

	scope, listed := Scopes[method]
	if !listed || !slices.Contains(c.Scopes, scope) {
		return Caller{}, newError(CodeForbidden, "the caller lacks the %q scope.", scope).
			withHint("use a key with the %s scope", scope)
	}

	if c.ProjectID != "" && projectID != "" && c.ProjectID != projectID {
		return Caller{}, newError(CodeForbidden, "the key is limited to another project.").
			withHint("use a key that covers this project")
	}

	return c, nil
}

package service

import (
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
)

// TemplateListRequest pages the template library.
type TemplateListRequest struct{ Page }

// SlotInfo describes one slot of a template (SDD-003 section 2.9).
type SlotInfo struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"` // "media" | "text" | "caption"
	Required       bool   `json:"required"`
	Description    string `json:"description"               untrusted:"true"`
	DurationPolicy string `json:"duration_policy,omitempty"`
}

// TemplateSummary is a template without its document.
type TemplateSummary struct {
	TemplateID  string            `json:"template_id"`
	Name        string            `json:"name"             untrusted:"true"`
	Description string            `json:"description"      untrusted:"true"`
	Slots       []SlotInfo        `json:"slots"`
	Colors      map[string]string `json:"colors,omitempty"`
	Width       int               `json:"width"`
	Height      int               `json:"height"`
	Duration    TimeValue         `json:"duration"`
}

// TemplateListResult answers TemplateList.
type TemplateListResult struct {
	Envelope
	Templates  []TemplateSummary `json:"templates"`
	NextCursor *string           `json:"next_cursor"`
	Text       string            `json:"text"`
}

// TemplateSaveRequest makes a template from a project.
type TemplateSaveRequest struct {
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	DryRun      bool   `json:"dry_run,omitempty"`
}

// TemplateRef names a template.
type TemplateRef struct {
	TemplateID string `json:"template_id"`
}

// TemplateResult answers TemplateSave and TemplateImport.
type TemplateResult struct {
	Envelope
	Template TemplateSummary `json:"template"`
	// Deduplicated is true when an import found the same content already stored and created nothing (VV-128).
	Deduplicated bool   `json:"deduplicated,omitempty"`
	Text         string `json:"text"`
}

// SlotValue fills one slot; exactly one member is given.
type SlotValue struct {
	AssetID  string            `json:"asset_id,omitempty"`
	SourceIn string            `json:"source_in,omitempty"` // a time; with asset_id
	Text     *string           `json:"text,omitempty"`
	Words    []command.WordArg `json:"words,omitempty"`
}

// TemplateInstantiateRequest makes a project from a template.
type TemplateInstantiateRequest struct {
	TemplateID string               `json:"template_id"`
	Name       string               `json:"name"`
	Fill       map[string]SlotValue `json:"fill,omitempty"`
	ID         string               `json:"id,omitempty"`
	DryRun     bool                 `json:"dry_run,omitempty"`
	// BrandColors are names of template.colors that take the colour of the brand kit, and BrandFonts
	// are families of the kit added to the fonts of the project (SDD-005 section 5.11).
	BrandColors []string `json:"brand_colors,omitempty"`
	BrandFonts  []string `json:"brand_fonts,omitempty"`
}

// TemplateInstantiateResult is the new project and what is still to fill.
type TemplateInstantiateResult struct {
	ProjectID string         `json:"project_id"`
	Revision  int64          `json:"revision"`
	Unfilled  []string       `json:"unfilled"`
	Findings  []lint.Finding `json:"findings"`
	Text      string         `json:"text"`
}

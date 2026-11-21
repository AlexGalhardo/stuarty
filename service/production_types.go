package service

import (
	"time"

	"github.com/AlexGalhardo/stuarty/core/project"
)

// ProjectItem is an item of a production (SDD-003 section 2.3.1).
type ProjectItem struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"` // "brief" | "text" | "timeline" | "output" | "publication"
	Title      string    `json:"title"                 untrusted:"true"`
	Position   int       `json:"position"`
	State      string    `json:"state"`                 // "active" | "trashed"
	TimelineID string    `json:"timeline_id,omitempty"` // kind timeline
	JobID      string    `json:"job_id,omitempty"`      // kind output: the export job
	Format     string    `json:"format,omitempty"`      // kind output: the destination format; publication: its networks
	Status     string    `json:"status,omitempty"`      // kind publication: pending_approval or published, from its targets
	Text       string    `json:"text,omitempty"        untrusted:"true"`
	TextKind   string    `json:"text_kind,omitempty"` // kind text: "script" | "copy" | "caption" (SDD-019)
	UpdatedAt  time.Time `json:"updated_at"`
}

// ProductionState is what a project has and lacks (RF-303); core/project computes it.
type ProductionState = project.Production

// ProjectDescribeRequest asks for the text view of a production.
type ProjectDescribeRequest struct {
	ProjectID string `json:"project_id"`
	MaxTokens int    `json:"max_tokens,omitempty"` // default 1500; 200 to 6000
}

// ProjectDescribeResult is the production as text, with its structure.
type ProjectDescribeResult struct {
	Envelope
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"        untrusted:"true"`
	State       string          `json:"state"`
	Description string          `json:"description" untrusted:"true"`
	Goal        ProjectGoal     `json:"goal"`
	Complete    bool            `json:"complete"` // false for a project made before epic 56 that nobody completed
	Production  ProductionState `json:"production"`
	Items       []ProjectItem   `json:"items"`
	Text        string          `json:"text"`
}

// ProjectDuplicateRequest copies a project.
type ProjectDuplicateRequest struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name,omitempty"` // default: the name followed by " copy"
	DryRun    bool   `json:"dry_run,omitempty"`
}

// ProjectItemAddRequest adds an item to a production.
type ProjectItemAddRequest struct {
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind"`                // "text", "timeline" or "output"
	Title     string `json:"title"`               // 1 to 120 characters
	Position  *int   `json:"position,omitempty"`  // default: the end
	Text      string `json:"text,omitempty"`      // kind text
	TextKind  string `json:"text_kind,omitempty"` // kind text: "script", "copy" (default) or "caption"
	Preset    string `json:"preset,omitempty"`    // kind timeline
	FPS       string `json:"fps,omitempty"`       // kind timeline
	JobID     string `json:"job_id,omitempty"`    // kind output: a succeeded export job of this project
	DryRun    bool   `json:"dry_run,omitempty"`
}

// ProjectItemMoveRequest moves an item to a position.
type ProjectItemMoveRequest struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
	Position  int    `json:"position"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// ProjectItemRenameRequest changes the title of an item.
type ProjectItemRenameRequest struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
	Title     string `json:"title"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// ProjectItemTrashRequest trashes or restores an item.
type ProjectItemTrashRequest struct {
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// ProjectItemResult is the item an operation ended with and the state of its production.
type ProjectItemResult struct {
	Envelope
	ProjectID  string          `json:"project_id"`
	Item       ProjectItem     `json:"item"`
	Production ProductionState `json:"production"`
	Text       string          `json:"text"`
}

// BriefGetRequest reads the brief of a project.
type BriefGetRequest struct {
	ProjectID string `json:"project_id"`
}

// BriefSetRequest writes the brief of a project.
type BriefSetRequest struct {
	ProjectID string `json:"project_id"`
	Text      string `json:"text"` // Markdown; empty clears the brief
	DryRun    bool   `json:"dry_run,omitempty"`
}

// BriefResult is the brief of a project.
type BriefResult struct {
	Envelope
	ProjectID string     `json:"project_id"`
	ItemID    string     `json:"item_id,omitempty"` // empty when the project has no brief
	Text      string     `json:"text"                 untrusted:"true"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

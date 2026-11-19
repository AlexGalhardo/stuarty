package service

import "time"

// The requests and results of the library operations (SDD-018): memory notes, skills and prompts, kept as
// files in the workspace. Every text that came from a file is tagged untrusted: it was written by a
// person, by another agent or by a stranger, and it reaches an agent as data (SDD-007 section 8).

// LibraryEntry is one row of a list: what is known of an entry without its body.
type LibraryEntry struct {
	Kind        string    `json:"kind"` // memory, skill or prompt
	Name        string    `json:"name"`
	Description string    `json:"description"             untrusted:"true"`
	Scope       string    `json:"scope,omitempty"`  // memory: workspace or a project id
	Origin      string    `json:"origin,omitempty"` // skills: built-in, user or imported
	Reviewed    bool      `json:"reviewed"`         // skills: a person read it as it is now
	Kept        bool      `json:"kept"`             // memory: a person kept it
	Actor       string    `json:"actor,omitempty"`
	Tags        []string  `json:"tags,omitempty"          untrusted:"true"`
	State       string    `json:"state"` // ok, or invalid when the file cannot be read
	Error       string    `json:"error,omitempty"         untrusted:"true"`
	LintErrors  int       `json:"lint_errors,omitempty"`
	LintWarns   int       `json:"lint_warnings,omitempty"`
	HasScripts  bool      `json:"has_scripts,omitempty"` // skills: the folder holds a script, stored inert
	UpdatedAt   time.Time `json:"updated_at"`
	// Warning is a sentence of the product that says why an entry is not offered to an agent.
	Warning string `json:"warning,omitempty"`
}

// LibraryFinding is one result of the skill lint.
type LibraryFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // error or warning
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// LibraryRef names one entry.
type LibraryRef struct {
	Name string `json:"name"`
	// AgentView is set by an agent surface: the call then sees only what is reviewed or kept.
	AgentView bool `json:"agent_view,omitempty"`
}

// SkillReviewRequest marks a skill as read: its name and the content hash of the skill as the person read it
// (SkillGetResult.ContentHash). A skill that changed since is not marked (SDD-018 section 7).
type SkillReviewRequest struct {
	Name        string `json:"name"`
	ContentHash string `json:"content_hash"`
}

// LibraryListResult is a list of entries of one kind.
type LibraryListResult struct {
	Envelope
	Kind    string         `json:"kind"`
	Entries []LibraryEntry `json:"entries"`
	Text    string         `json:"text"`
}

// MemoryListRequest lists the notes.
type MemoryListRequest struct {
	Scope             string   `json:"scope,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	IncludeUnreviewed bool     `json:"include_unreviewed,omitempty"` // notes no person kept yet
	AgentView         bool     `json:"agent_view,omitempty"`
}

// MemoryGetResult is a note.
type MemoryGetResult struct {
	Envelope
	Entry LibraryEntry `json:"entry"`
	Body  string       `json:"body"  untrusted:"true"`
	Text  string       `json:"text"`
}

// MemoryWriteRequest writes a note: it creates it, or replaces the one of that name.
type MemoryWriteRequest struct {
	Name        string   `json:"name,omitempty"` // empty: made from the description
	Description string   `json:"description"`
	Scope       string   `json:"scope,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Body        string   `json:"body,omitempty"`
	// Keep is for a person: true keeps the note, false releases it. A note a person writes is kept.
	Keep *bool `json:"keep,omitempty"`
}

// MemoryWriteResult is the note that was written.
type MemoryWriteResult struct {
	Envelope
	Entry LibraryEntry `json:"entry"`
	Text  string       `json:"text"`
}

// LibraryRemoveResult says what was removed.
type LibraryRemoveResult struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Text string `json:"text"`
}

// SkillListRequest lists the skills.
type SkillListRequest struct {
	Origin            string `json:"origin,omitempty"`
	IncludeUnreviewed bool   `json:"include_unreviewed,omitempty"`
	AgentView         bool   `json:"agent_view,omitempty"`
}

// SkillFileView is one file of a skill folder.
type SkillFileView struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	Script bool   `json:"script,omitempty"` // stored inert, never run
	Binary bool   `json:"binary,omitempty"` // not text: its content is not returned
	Text   string `json:"text,omitempty"   untrusted:"true"`
}

// SkillGetResult is a skill: its text and its files.
type SkillGetResult struct {
	Envelope
	Entry         LibraryEntry     `json:"entry"`
	License       string           `json:"license,omitempty"       untrusted:"true"`
	Compatibility string           `json:"compatibility,omitempty" untrusted:"true"`
	AllowedTools  string           `json:"allowed_tools,omitempty" untrusted:"true"`
	Source        string           `json:"source,omitempty"        untrusted:"true"`
	Body          string           `json:"body"                    untrusted:"true"`
	Files         []SkillFileView  `json:"files,omitempty"`
	Findings      []LibraryFinding `json:"findings,omitempty"`
	// ContentHash is the hash of the content returned, which a person sends back to mark it as read.
	ContentHash string `json:"content_hash"`
	Text        string `json:"text"`
}

// SkillFileInput is a file of a skill that a caller writes.
type SkillFileInput struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// SkillWriteRequest writes a skill. The origin, the source and the review are the product's to record.
type SkillWriteRequest struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	AllowedTools  string            `json:"allowed_tools,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Body          string            `json:"body"`
	// Files are the other files of the folder. Omitted (null) keeps those the skill has; an empty list removes them.
	Files []SkillFileInput `json:"files"`
}

// SkillWriteResult is the skill that was written, with the findings of the lint.
type SkillWriteResult struct {
	Envelope
	Entry    LibraryEntry     `json:"entry"`
	Findings []LibraryFinding `json:"findings,omitempty"`
	Text     string           `json:"text"`
}

// SkillImportRequest imports a skill from a folder or an archive on the machine of the server.
type SkillImportRequest struct {
	Path    string `json:"path"`             // absolute: a folder, a .zip or a .tar.gz
	Source  string `json:"source,omitempty"` // where it came from; the base name of the path if empty
	Replace bool   `json:"replace,omitempty"`
}

// SkillImportResult is the skill that was imported: unreviewed, with the findings of the lint.
type SkillImportResult struct {
	Envelope
	Entry    LibraryEntry     `json:"entry"`
	Findings []LibraryFinding `json:"findings,omitempty"`
	Files    int              `json:"files"`
	Text     string           `json:"text"`
}

// SkillReviewResult is the skill after a person marked it as read.
type SkillReviewResult struct {
	Envelope
	Entry LibraryEntry `json:"entry"`
	Text  string       `json:"text"`
}

// SkillLintResult is the lint of a skill.
type SkillLintResult struct {
	Name     string           `json:"name"`
	Findings []LibraryFinding `json:"findings"`
	Errors   int              `json:"errors"`
	Warnings int              `json:"warnings"`
	Text     string           `json:"text"`
}

// SkillExportRequest exports skills as folders in the Agent Skills layout.
type SkillExportRequest struct {
	Names []string `json:"names,omitempty"` // empty: every reviewed skill
}

// SkillExportResult says what was exported.
type SkillExportResult struct {
	Names []string `json:"names"`
	Path  string   `json:"path,omitempty"` // the folder that holds them; to a local caller only
	Text  string   `json:"text"`
}

// PromptListRequest lists the prompts.
type PromptListRequest struct {
	Tags      []string `json:"tags,omitempty"`
	AgentView bool     `json:"agent_view,omitempty"`
}

// PromptGetResult is a prompt.
type PromptGetResult struct {
	Envelope
	Entry     LibraryEntry      `json:"entry"`
	Body      string            `json:"body"                untrusted:"true"`
	Variables []string          `json:"variables,omitempty"`
	Defaults  map[string]string `json:"defaults,omitempty"  untrusted:"true"`
	Text      string            `json:"text"`
}

// PromptWriteRequest writes a prompt.
type PromptWriteRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Tags        []string          `json:"tags,omitempty"`
	Defaults    map[string]string `json:"defaults,omitempty"`
	Body        string            `json:"body"`
}

// PromptWriteResult is the prompt that was written.
type PromptWriteResult struct {
	Envelope
	Entry     LibraryEntry `json:"entry"`
	Variables []string     `json:"variables,omitempty"`
	Text      string       `json:"text"`
}

// PromptRenderRequest replaces the variables of a prompt.
type PromptRenderRequest struct {
	Name      string            `json:"name"`
	Values    map[string]string `json:"values,omitempty"`
	AgentView bool              `json:"agent_view,omitempty"`
}

// PromptRenderResult is the text of a prompt with its variables replaced.
type PromptRenderResult struct {
	Envelope
	Name     string `json:"name"`
	Rendered string `json:"rendered" untrusted:"true"`
	Text     string `json:"text"`
}

// LibrarySearchRequest is one full-text query over the three kinds.
type LibrarySearchRequest struct {
	Query             string   `json:"query"`
	Kinds             []string `json:"kinds,omitempty"`
	Scope             string   `json:"scope,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	Origin            string   `json:"origin,omitempty"`
	IncludeUnreviewed bool     `json:"include_unreviewed,omitempty"`
	Limit             int      `json:"limit,omitempty"`  // default 20, at most 50
	Budget            int      `json:"budget,omitempty"` // characters, default 4000, at most 16000
	AgentView         bool     `json:"agent_view,omitempty"`
}

// LibraryHit is a result of a search.
type LibraryHit struct {
	LibraryEntry
	Snippet string `json:"snippet" untrusted:"true"`
}

// LibrarySearchResult is the answer of a search; it fits its budget.
type LibrarySearchResult struct {
	Envelope
	Hits      []LibraryHit `json:"hits"`
	Total     int          `json:"total"`
	Truncated bool         `json:"truncated,omitempty"`
	Text      string       `json:"text"`
}

// LibraryReindexRequest has no members.
type LibraryReindexRequest struct{}

// LibraryReindexResult says what the rebuild of the index found.
type LibraryReindexResult struct {
	Envelope
	Memory  int      `json:"memory"`
	Skills  int      `json:"skills"`
	Prompts int      `json:"prompts"`
	Invalid int      `json:"invalid"`
	Ignored []string `json:"ignored,omitempty" untrusted:"true"` // file names that are not valid names
	Text    string   `json:"text"`
}

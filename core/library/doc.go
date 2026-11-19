// Package library holds the pure rules of the library of a workspace: memory
// notes, skills and prompts kept as Markdown files with front matter (ADR-0040,
// SDD-018). It parses and writes front matter, validates names and fields,
// renders prompt variables, computes the content hash a review is bound to, and
// lints a skill. It reads no file and opens no connection: the files, the index
// and the import are in service/library.
package library

// Kind is one of the three kinds of entry of the library.
type Kind string

// The kinds. An entry is identified by its kind and its name.
const (
	KindMemory Kind = "memory"
	KindSkill  Kind = "skill"
	KindPrompt Kind = "prompt"
)

// Kinds lists the kinds in the order the sections of the dashboard show them.
var Kinds = []Kind{KindMemory, KindSkill, KindPrompt}

// Valid reports whether k is a kind of the library.
func (k Kind) Valid() bool {
	return k == KindMemory || k == KindSkill || k == KindPrompt
}

// Key is the identifier of an entry: "<kind>/<name>".
func Key(k Kind, name string) string { return string(k) + "/" + name }

// The limits of SDD-018 section 12.
const (
	MaxNameLength       = 64
	MaxDescription      = 200 // memory and prompt descriptions
	MaxSkillDescription = 1024
	MaxCompatibility    = 500
	MaxTags             = 10
	MaxFrontMatter      = 8 << 10

	MaxMemoryBody  = 8 << 10
	MaxPromptBody  = 32 << 10
	MaxValueLength = 8 << 10 // one value of a prompt variable

	MaxSkillMD    = 64 << 10
	MaxSkillFile  = 512 << 10
	MaxSkillBytes = 2 << 20
	MaxSkillFiles = 100

	MaxSearchTerms  = 8
	MaxTermLength   = 64
	MaxSnippet      = 300
	DefaultBudget   = 4000
	MaxBudget       = 16000
	DefaultHits     = 20
	MaxHits         = 50
	MaxNotes        = 2000
	MaxNotesPerDay  = 50
	MaxSkills       = 500
	MaxPrompts      = 500
	MaxArchiveBytes = 8 << 20
)

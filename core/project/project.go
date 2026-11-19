// Package project holds the pure rules of a production (ADR-0039, SDD-005 section 5.12): which kinds of item a
// project holds, how they are ordered, what may be added or trashed, and what the production state is. It does
// no I/O. The timeline document is not here: a timeline is one item of a project and the document keeps its own
// package (core/timeline).
package project

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// PrefixItem is the identifier prefix of a project item (SDD-000 section 2).
const PrefixItem = "pit"

// Limits of the text a person writes into a project, and of the size of a production: a project never holds
// more than a bundle of the whole project may carry, so that its bundle imports again (SDD-005 sections 5.12
// and 16.1.1).
const (
	MaxTitle     = 120
	MaxText      = 256 << 10 // bytes of a brief or of a text item
	MaxItems     = 256       // active items of a project
	MaxTimelines = 64        // active timelines of a project
)

// Kind is the kind of an item.
type Kind string

// The kinds that exist. Image (epic 27) and Publication (epic 20) are reserved: they are known names that are
// not accepted yet.
const (
	Brief       Kind = "brief"
	Text        Kind = "text"
	Timeline    Kind = "timeline"
	Output      Kind = "output"
	Image       Kind = "image"
	Publication Kind = "publication"
)

// Kinds lists the kinds an item may have today, in the order a production shows them.
func Kinds() []Kind { return []Kind{Brief, Text, Timeline, Output} }

// Known reports whether k is a kind of item, reserved or not.
func (k Kind) Known() bool { return k.Allowed() || k.Reserved() }

// Allowed reports whether an item of kind k can exist today.
func (k Kind) Allowed() bool {
	switch k {
	case Brief, Text, Timeline, Output:
		return true
	case Image, Publication:
		return false
	}

	return false
}

// Reserved reports whether k is a kind a later epic adds.
func (k Kind) Reserved() bool { return k == Image || k == Publication }

// Item is the part of a project item the rules read.
type Item struct {
	ID       string
	Kind     Kind
	Position int
	Trashed  bool
	// Text is the Markdown of a brief or of a text item; only its emptiness matters here.
	Text string
	// Status is the state of a publication: "pending_approval" or "published". Other kinds leave it empty.
	Status string
}

// The statuses of a publication item (epic 20 gives them their transitions).
const (
	StatusPendingApproval = "pending_approval"
	StatusPublished       = "published"
)

// Production is what a person or an agent reads to know what a project has and what it lacks (RF-303).
type Production struct {
	HasBrief         bool `json:"has_brief"`
	HasScript        bool `json:"has_script"`
	Texts            int  `json:"texts"`
	Timelines        int  `json:"timelines"`
	Outputs          int  `json:"outputs"`
	PendingApprovals int  `json:"pending_approvals"`
	Published        int  `json:"published"`
}

// ProductionState computes the state of a production from its items. A trashed item counts for nothing; a brief
// or a text that is only white space is not there.
func ProductionState(items []Item) Production {
	var p Production

	for i := range items {
		it := &items[i]
		if it.Trashed {
			continue
		}

		switch it.Kind {
		case Brief:
			p.HasBrief = p.HasBrief || strings.TrimSpace(it.Text) != ""
		case Text:
			p.HasScript = p.HasScript || strings.TrimSpace(it.Text) != ""
			p.Texts++
		case Timeline:
			p.Timelines++
		case Output:
			p.Outputs++
		case Image:
		case Publication:
			switch it.Status {
			case StatusPendingApproval:
				p.PendingApprovals++
			case StatusPublished:
				p.Published++
			}
		}
	}

	return p
}

// RuleError is a rule broken. Code is a code of SDD-000 section 8 (E_VALIDATION, E_INVALID_STATE,
// E_LIMIT_EXCEEDED); the service turns it into its error.
type RuleError struct {
	Code    string
	Path    string
	Message string
	Hint    string
}

func (v *RuleError) Error() string { return v.Message }

func validation(path, hint, format string, args ...any) *RuleError {
	return &RuleError{Code: "E_VALIDATION", Path: path, Hint: hint, Message: fmt.Sprintf(format, args...)}
}

// CheckTitle is the rule of an item title: 1 to MaxTitle characters, some visible text, no control or
// direction character (the rule of a project name, VV-049).
func CheckTitle(title string) *RuleError {
	if n := utf8.RuneCountInString(title); n < 1 || n > MaxTitle {
		return validation("title", fmt.Sprintf("send a title of 1 to %d characters", MaxTitle),
			"the title has %d characters; it must have 1 to %d", n, MaxTitle)
	}

	visible := false

	for _, r := range title {
		switch {
		case unicode.IsControl(r):
			return validation("title", "send a title with no control character", "the title has a control character")
		case isBidiControl(r):
			return validation("title", "send a title with no text direction character",
				"the title has a text direction character")
		case !unicode.IsSpace(r) && !unicode.Is(unicode.Cf, r):
			visible = true
		}
	}

	if !visible {
		return validation("title", "send a title with visible text", "the title has no visible character")
	}

	return nil
}

func isBidiControl(r rune) bool {
	return r == 0x061C || r == 0x200E || r == 0x200F ||
		(r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

// CheckText is the rule of the Markdown of a brief or of a text item.
func CheckText(text string) *RuleError {
	if len(text) > MaxText {
		return &RuleError{
			Code: "E_LIMIT_EXCEEDED", Path: "text", Hint: "send at most 256 KiB",
			Message: fmt.Sprintf("the text has %d bytes; the most is %d", len(text), MaxText),
		}
	}

	if !utf8.ValidString(text) {
		return validation("text", "send UTF-8 text", "the text is not valid UTF-8")
	}

	return nil
}

// CheckAdd is the rule of ProjectItemAdd: the kind must be one that can be added by hand. A brief is made by
// BriefSet, so that a project never has two.
func CheckAdd(kind Kind) *RuleError {
	switch {
	case kind == Brief:
		return validation("kind", "use brief:set to write the brief", "a project has one brief, written with BriefSet")
	case kind.Reserved():
		return validation("kind", "use text, timeline or output",
			"the kind %q is not available yet", string(kind))
	case !kind.Allowed():
		return validation("kind", "use text, timeline or output", "the kind %q is not known", string(kind))
	}

	return nil
}

// CheckItemID reports whether id has the form of an item identifier.
func CheckItemID(id string) bool { return timeline.ValidID(PrefixItem, id) }

// CheckTrash is the rule of trashing the item id: the last active timeline of a project stays, because every
// timeline operation of a project that has one timeline addresses it (SDD-003 section 1.7).
func CheckTrash(items []Item, id string) *RuleError {
	var target *Item

	active := 0

	for i := range items {
		it := &items[i]
		if it.ID == id {
			target = it
		}

		if it.Kind == Timeline && !it.Trashed {
			active++
		}
	}

	if target != nil && target.Kind == Timeline && !target.Trashed && active == 1 {
		return &RuleError{
			Code: "E_INVALID_STATE", Path: "item_id", Hint: "add another timeline first",
			Message: "a project keeps at least one timeline",
		}
	}

	// A project has one brief, and BriefSet writes it; a trashed brief would stand beside the next one.
	if target != nil && target.Kind == Brief && !target.Trashed {
		return &RuleError{
			Code: "E_INVALID_STATE", Path: "item_id", Hint: "clear the brief with brief:set and an empty text",
			Message: "the brief is not moved to the trash",
		}
	}

	return nil
}

// CheckRoom is the rule of the size of a production: one more active item of kind k must leave the project with
// at most MaxItems active items and MaxTimelines active timelines.
func CheckRoom(items []Item, k Kind) *RuleError {
	active, timelines := 0, 0

	for i := range items {
		if items[i].Trashed {
			continue
		}

		active++

		if items[i].Kind == Timeline {
			timelines++
		}
	}

	if active+1 > MaxItems {
		return &RuleError{
			Code: "E_LIMIT_EXCEEDED", Path: "kind", Hint: "trash an item you no longer need",
			Message: fmt.Sprintf("the project has %d items; the most is %d", active, MaxItems),
		}
	}

	if k == Timeline && timelines+1 > MaxTimelines {
		return &RuleError{
			Code: "E_LIMIT_EXCEEDED", Path: "kind", Hint: "trash a timeline you no longer need",
			Message: fmt.Sprintf("the project has %d timelines; the most is %d", timelines, MaxTimelines),
		}
	}

	return nil
}

// Position is where an item goes in the order: pos is a 0-based index of the whole list, and a value past the
// end means the end. A negative value is a violation.
func Position(n, pos int) (int, *RuleError) {
	if pos < 0 {
		return 0, validation("position", "send 0 or more", "the position %d is below 0", pos)
	}

	return min(pos, n), nil
}

// Order returns ids with id moved to index pos (clamped to the end), the others in their order. It does not
// change its argument. ok is false when id is not in ids.
func Order(ids []string, id string, pos int) (out []string, ok bool) {
	rest := make([]string, 0, len(ids))

	for _, x := range ids {
		if x == id {
			ok = true
			continue
		}

		rest = append(rest, x)
	}

	if !ok {
		return nil, false
	}

	pos = max(0, min(pos, len(rest)))
	out = make([]string, 0, len(ids))
	out = append(out, rest[:pos]...)
	out = append(out, id)
	out = append(out, rest[pos:]...)

	return out, true
}

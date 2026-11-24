package project

import "strings"

// Intent is the kind of a project, what it is meant to become (epic 78, author decision D-134): only a text, a
// video, an image, or not decided yet. The steps of a project follow it, and a project is never forced to hold
// media. It is named Intent here because Kind is the kind of an item.
type Intent string

// The kinds of a project (SDD-000 section 1).
const (
	KindUndecided Intent = "undecided"
	KindText      Intent = "text"
	KindVideo     Intent = "video"
	KindImage     Intent = "image"
)

// The kinds of a text item the rules of a workflow tell apart (SDD-019).
const (
	TextKindScript  = "script"
	TextKindCopy    = "copy"
	TextKindCaption = "caption"
	TextKindPost    = "post"
)

// Intents lists the kinds a person may choose for a project, in the order a surface shows them.
func Intents() []Intent { return []Intent{KindText, KindVideo, KindImage} }

// Valid reports whether k is one of the four kinds.
func (k Intent) Valid() bool {
	switch k {
	case KindUndecided, KindText, KindVideo, KindImage:
		return true
	}

	return false
}

// ParseKind reads the kind a request carries: an empty value is undecided, and anything that is not a kind is
// E_VALIDATION on the path "kind".
func ParseKind(s string) (Intent, *RuleError) {
	if s == "" {
		return KindUndecided, nil
	}

	if k := Intent(s); k.Valid() {
		return k, nil
	}

	return KindUndecided, validation("kind", "use text, video or image, or leave kind out",
		"the kind %q is not known", s)
}

// EffectiveKind is the kind a project reads as (SDD-005 section 5.12.4). A stored kind other than undecided
// wins. Otherwise it comes from what the project holds, so that a project made before the kind existed is never
// rewritten: an active timeline makes it a video, an active image an image, an active text that is not a script a
// text, and a project with only a brief or scripts, or nothing, stays undecided. A trashed item holds nothing.
func EffectiveKind(stored Intent, items []Item) Intent {
	if stored.Valid() && stored != KindUndecided {
		return stored
	}

	return derivedKind(holdingsOf(items))
}

func derivedKind(h holdings) Intent {
	switch {
	case h.timelines > 0:
		return KindVideo
	case h.images > 0:
		return KindImage
	case h.texts > 0:
		return KindText
	}

	return KindUndecided
}

// KindOfItem is the kind an item decides for a project that has none: a text makes it a text, a timeline a video and
// an image an image. A script decides nothing, since it may be the words of a video as well as a text (VV-394,
// D-139), and neither do a brief, an output and a publication.
func KindOfItem(it Item) Intent {
	switch it.Kind {
	case Text:
		if it.TextKind == TextKindScript {
			return KindUndecided
		}

		return KindText
	case Timeline:
		return KindVideo
	case Image:
		return KindImage
	case Brief, Output, Publication:
	}

	return KindUndecided
}

// KindToStore is the kind a project that has none stored takes when the item added joins it: what it already reads
// as, so that a project that reads as a text and gets a timeline stays a text with its steps, and otherwise what the
// new item decides. It is undecided when neither decides anything.
func KindToStore(stored Intent, before []Item, added Item) Intent {
	if stored.Valid() && stored != KindUndecided {
		return stored
	}

	if k := EffectiveKind(KindUndecided, before); k != KindUndecided {
		return k
	}

	return KindOfItem(added)
}

// holdings counts what a project holds, the facts both the kind and the workflow read from its items.
type holdings struct {
	timelines, images, texts, outputs int
	hasBrief                          bool
	writing                           bool // an active text that is not a caption has some text
	hasPost                           bool // an active text of the kind post
	publications, pending, published  int
}

func hasText(s string) bool { return strings.TrimSpace(s) != "" }

func holdingsOf(items []Item) holdings {
	var h holdings

	for i := range items {
		it := &items[i]
		if it.Trashed {
			continue
		}

		switch it.Kind {
		case Brief:
			h.hasBrief = h.hasBrief || hasText(it.Text)
		case Text:
			if KindOfItem(*it) == KindText {
				h.texts++ // the texts that decide the kind: a script does not
			}

			h.hasPost = h.hasPost || it.TextKind == TextKindPost

			if it.TextKind != TextKindCaption && hasText(it.Text) {
				h.writing = true
			}
		case Timeline:
			h.timelines++
		case Image:
			h.images++
		case Output:
			h.outputs++
		case Publication:
			h.publications++

			switch it.Status {
			case StatusPendingApproval:
				h.pending++
			case StatusPublished:
				h.published++
			}
		}
	}

	return h
}

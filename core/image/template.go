package image

import (
	"maps"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// SlotValue is what fills a slot of a template: a text for a text slot, the content hash of an image for an
// image slot.
type SlotValue struct {
	Text  *string `json:"text,omitempty"`
	Asset string  `json:"asset,omitempty"`
}

// The reasons of E_SLOT for a template that was filled badly (SDD-021 section 7).
const (
	SlotUnknown      = "unknown_slot"
	SlotWrongKind    = "wrong_kind"
	SlotInvalidValue = "invalid_value"
	SlotMissing      = "missing"
)

func slotError(reason, slot, format string, a ...any) *command.Error {
	return command.Errorf(command.CodeSlot, format, a...).
		WithDetail("reason", reason).WithDetail("slot", slot).WithPath("values." + slot)
}

// empty reports whether a slot layer has no content: a text that is blank or an image that has no file.
func empty(l *Layer) bool {
	switch l.Kind {
	case KindText:
		for _, r := range l.Text {
			if r != ' ' && r != '\n' && r != '\t' && r != '\r' {
				return false
			}
		}

		return true
	case KindImage:
		return l.Asset == ""
	}

	return false
}

// FromTemplate validates the values and returns a new document made from the template: the id given, the
// template flag cleared, the layers kept with their ids (SDD-021 section 7). A slot that is empty in the
// template is required; one that has content keeps it unless a value replaces it. Nothing is returned when any
// value is wrong or any required slot has none.
func FromTemplate(t *Document, values map[string]SlotValue, id string, env Env) (*Document, error) {
	if !t.Template {
		return nil, command.Errorf(command.CodeInvalidState, "Image %s is not a template.", t.ID).
			WithHint("Create the image with template true, or give no template to make a blank one.")
	}

	slots := map[string]*Layer{}
	for _, l := range Slots(t) {
		slots[l.Slot] = l
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}

	slices.Sort(names)

	out := Clone(t)
	out.ID, out.Template = id, false

	for _, name := range names {
		src, ok := slots[name]
		if !ok {
			return nil, slotError(SlotUnknown, name, "The template has no slot named %q.", name).
				WithHint("The slots are: %s.", slotList(slots)).WithDetail("slots", slices.Sorted(maps.Keys(slots)))
		}

		l, _ := Find(out, src.ID)
		if err := fill(out, l, name, values[name], env); err != nil {
			return nil, err
		}
	}

	var missing []string

	for _, l := range Slots(out) {
		if empty(l) {
			missing = append(missing, l.Slot)
		}
	}

	if len(missing) > 0 {
		slices.Sort(missing)

		return nil, command.Errorf(command.CodeSlot, "The template needs a value for: %s.", strings.Join(missing, ", ")).
			WithHint("Give a value for each of them in values.").
			WithDetail("reason", SlotMissing).
			WithDetail("missing", missing).
			WithPath("values")
	}

	pruneAssets(out)

	if vs := Validate(out); len(vs) > 0 {
		return nil, firstViolation("the template", out)
	}

	return out, nil
}

func fill(d *Document, l *Layer, name string, v SlotValue, env Env) *command.Error {
	switch l.Kind {
	case KindText:
		if v.Text == nil || v.Asset != "" {
			return slotError(SlotWrongKind, name, "The slot %q is a text slot: give {\"text\": \"...\"}.", name).
				WithHint("A text slot takes text and no asset.")
		}

		if len([]rune(*v.Text)) > MaxText {
			return slotError(
				SlotInvalidValue,
				name,
				"The text of slot %q is longer than %d characters.",
				name,
				MaxText,
			).
				WithHint("Shorten the text.")
		}

		l.Text = *v.Text
		remeasure(&env, d, l)
	case KindImage:
		if v.Asset == "" || v.Text != nil {
			return slotError(
				SlotWrongKind,
				name,
				"The slot %q is an image slot: give {\"asset\": \"sha256:...\"}.",
				name,
			).
				WithHint("An image slot takes an asset and no text.")
		}

		info, ok := lookup(&env, v.Asset)
		if !ok || !hashPattern.MatchString(v.Asset) {
			return slotError(SlotInvalidValue, name, "The file %s is not a ready image of the workspace.", v.Asset).
				WithHint("Import the image with media:import and use the hash it returns.")
		}

		if _, known := d.Assets[v.Asset]; !known && len(d.Assets) >= MaxAssets {
			return slotError(SlotInvalidValue, name, "The document already uses %d images.", MaxAssets).
				WithHint("Use an image the template already has.")
		}

		if d.Assets == nil {
			d.Assets = map[string]AssetRef{}
		}

		d.Assets[v.Asset] = AssetRef{Hash: v.Asset, Name: info.Name, Width: info.Width, Height: info.Height}
		l.Asset, l.Crop = v.Asset, coverCrop(info.Width, info.Height, l.W, l.H)
	}

	return nil
}

// coverCrop is the part of a source of sw by sh that covers a box of w by h with the same aspect, centred.
func coverCrop(sw, sh, w, h int) *Crop {
	cw, ch := sw, sh

	if sw*h >= sh*w { // the source is wider than the box
		cw = min(max(roundDiv(sh*w, h), 1), sw)
	} else {
		ch = min(max(roundDiv(sw*h, w), 1), sh)
	}

	return &Crop{X: floorDiv(sw-cw, 2), Y: floorDiv(sh-ch, 2), W: cw, H: ch}
}

func slotList(slots map[string]*Layer) string {
	return strings.Join(slices.Sorted(maps.Keys(slots)), ", ")
}

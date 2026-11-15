// The coverage declaration of SDD-012 section 9.3: for each command of the
// catalogue that is not internal, the entry points that exist in code. The
// test in coverage.test.ts checks it against the committed registry snapshot,
// so a command added to the catalogue breaks the build until it has a key here.

export interface EntryPoint {
	kind: "gesture" | "inspector" | "menu" | "toolbar" | "key" | "palette";
	// An id of ACTIONS.
	action: string;
	phase: "P" | "A" | "B";
}

// The commands an action may send, when it is not one fixed list.
export const ANY_COMMAND = "*";

// The actions of the registry and the operations each one sends. `asset.add`
// is internal (the service writes it when a file is imported), so it has no
// entry here.
export const ACTIONS: Readonly<Record<string, readonly string[]>> = {
	"library.add_to_timeline": ["clip.add", "audio.add", "track.add"],
	"library.drag": ["clip.add", "audio.add"],
	"library.remove": ["asset.remove"],
	"timeline.split": ["item.split"],
	"timeline.trim_start": ["item.trim"],
	"timeline.trim_end": ["item.trim"],
	"caption.add": ["caption.add", "track.add"],
	"inspector.rename": ["item.rename"],
	"timeline.rename": ["item.rename"],
	"timeline.delete": ["item.delete"],
	"timeline.ripple_delete": ["item.delete"],
	"timeline.duplicate": ["item.duplicate"],
	"timeline.close_gap": ["timeline.close_gap"],
	"inspector.move": ["item.move"],
	"inspector.trim_end": ["item.trim"],
	// The palette runs any command of the registry with operations built from
	// its schema form, so it has no builder to check (rule 2 of section 9.3).
	"palette.run": [ANY_COMMAND],
};

// Every command has the palette entry (SDD-012 section 9.2: the palette is
// generated from the registry). A richer entry point is declared in the task
// that builds it, with the phase in which it exists.
const palette: EntryPoint = {
	kind: "palette",
	action: "palette.run",
	phase: "A",
};

export const COVERAGE: Readonly<Record<string, readonly EntryPoint[]>> = {
	"asset.remove": [
		{ kind: "menu", action: "library.remove", phase: "A" },
		palette,
	],
	"audio.add": [
		{ kind: "gesture", action: "library.add_to_timeline", phase: "A" },
		{ kind: "gesture", action: "library.drag", phase: "A" },
		palette,
	],
	"caption.add": [
		{ kind: "toolbar", action: "caption.add", phase: "P" },
		palette,
	],
	"caption.edit_word": [palette],
	"caption.from_transcript": [palette],
	"caption.merge": [palette],
	"caption.retime_from_clip": [palette],
	"caption.set_breaks": [palette],
	"caption.set_style": [palette],
	"caption.set_words": [palette],
	"caption.split": [palette],
	"clip.add": [
		{ kind: "gesture", action: "library.add_to_timeline", phase: "P" },
		{ kind: "gesture", action: "library.drag", phase: "A" },
		palette,
	],
	"clip.remove_silences": [palette],
	"clip.set_color": [palette],
	"clip.set_framing": [palette],
	"clip.set_speed": [palette],
	"item.crossfade": [palette],
	"item.delete": [
		{ kind: "key", action: "timeline.delete", phase: "A" },
		{ kind: "key", action: "timeline.ripple_delete", phase: "A" },
		{ kind: "toolbar", action: "timeline.delete", phase: "A" },
		{ kind: "menu", action: "timeline.delete", phase: "A" },
		{ kind: "menu", action: "timeline.ripple_delete", phase: "A" },
		palette,
	],
	"item.duplicate": [
		{ kind: "key", action: "timeline.duplicate", phase: "A" },
		{ kind: "menu", action: "timeline.duplicate", phase: "A" },
		palette,
	],
	"item.move": [
		{ kind: "inspector", action: "inspector.move", phase: "A" },
		palette,
	],
	"item.rename": [
		{ kind: "inspector", action: "inspector.rename", phase: "A" },
		{ kind: "key", action: "timeline.rename", phase: "A" },
		{ kind: "menu", action: "timeline.rename", phase: "A" },
		palette,
	],
	"item.set_audio": [palette],
	"item.set_transform": [palette],
	"item.set_transition": [palette],
	"item.split": [
		{ kind: "toolbar", action: "timeline.split", phase: "P" },
		{ kind: "key", action: "timeline.split", phase: "P" },
		palette,
	],
	"item.trim": [
		{ kind: "toolbar", action: "timeline.trim_start", phase: "P" },
		{ kind: "toolbar", action: "timeline.trim_end", phase: "P" },
		{ kind: "key", action: "timeline.trim_start", phase: "P" },
		{ kind: "key", action: "timeline.trim_end", phase: "P" },
		{ kind: "inspector", action: "inspector.trim_end", phase: "A" },
		palette,
	],
	"keyframe.clear": [palette],
	"keyframe.remove": [palette],
	"keyframe.set": [palette],
	"project.rename": [palette],
	"settings.set": [palette],
	"settings.set_fps": [palette],
	"shape.add": [palette],
	"shape.set": [palette],
	"slot.fill": [palette],
	"slot.mark": [palette],
	"slot.unmark": [palette],
	"template.set_info": [palette],
	"text.add": [palette],
	"text.apply_preset": [palette],
	"text.set": [palette],
	"timeline.close_gap": [
		{ kind: "menu", action: "timeline.close_gap", phase: "A" },
		palette,
	],
	"track.add": [
		{ kind: "gesture", action: "library.add_to_timeline", phase: "P" },
		{ kind: "toolbar", action: "caption.add", phase: "P" },
		palette,
	],
	"track.move": [palette],
	"track.remove": [palette],
	"track.set_caption_style": [palette],
	"track.update": [palette],
};

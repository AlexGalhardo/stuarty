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
	"timeline.marker": ["marker.add", "marker.remove"],
	"inspector.marker": ["marker.set", "marker.remove"],
	"timeline.trim_start": ["item.trim"],
	"timeline.trim_end": ["item.trim"],
	"timeline.slide": ["item.move"],
	"timeline.roll": ["item.trim"],
	"timeline.slip": ["item.slip"],
	"caption.add": ["caption.add", "track.add"],
	"inspector.rename": ["item.rename"],
	"timeline.rename": ["item.rename"],
	"timeline.delete": ["item.delete"],
	"timeline.ripple_delete": ["item.delete"],
	"timeline.duplicate": ["item.duplicate"],
	"timeline.repeat": ["item.repeat"],
	"inspector.shadow": ["clip.set_shadow"],
	"inspector.outline": ["clip.set_outline"],
	"timeline.toggle_enabled": ["item.set_enabled"],
	"inspector.enabled": ["item.set_enabled"],
	"inspector.flip": ["clip.set_flip"],
	"inspector.blend": ["clip.set_blend"],
	"inspector.mask": ["clip.set_mask"],
	"inspector.chroma_key": ["clip.set_chroma_key"],
	"timeline.close_gap": ["timeline.close_gap"],
	"inspector.move": ["item.move"],
	"inspector.trim_end": ["item.trim"],
	"canvas.transform": ["item.set_transform", "keyframe.set"],
	"inspector.transform": ["item.set_transform", "keyframe.set"],
	"notice.use_keyframes": ["keyframe.set"],
	"inspector.keyframe_add": ["keyframe.set"],
	"inspector.keyframe_easing": ["keyframe.set"],
	"inspector.keyframe_value": ["keyframe.set"],
	"inspector.keyframe_remove": ["keyframe.remove"],
	"inspector.keyframe_clear": ["keyframe.clear"],
	"timeline.delete_keyframe": ["keyframe.remove"],
	"inspector.transition": ["item.set_transition"],
	"timeline.crossfade": ["item.crossfade"],
	"inspector.animation": ["text.apply_preset"],
	"timeline.edit_word": ["caption.edit_word"],
	"inspector.word_text": ["caption.edit_word"],
	"timeline.word_boundary": ["caption.set_words"],
	"inspector.words": ["caption.set_words"],
	"inspector.breaks": ["caption.set_breaks"],
	"timeline.split_caption": ["caption.split"],
	"inspector.split_word": ["caption.split"],
	"timeline.merge_captions": ["caption.merge"],
	"inspector.caption_style": ["caption.set_style"],
	"inspector.auto_captions": ["caption.from_transcript", "track.add"],
	"inspector.resync_captions": ["caption.retime_from_clip"],
	"lint.resync_captions": ["caption.retime_from_clip"],
	"text.add": ["text.add", "track.add"],
	"shape.add": ["shape.add", "track.add"],
	"inspector.shape": ["shape.set"],
	"inspector.slot_mark": ["slot.mark"],
	"inspector.slot_unmark": ["slot.unmark"],
	"slots.fill": ["slot.fill"],
	"inspector.template_info": ["template.set_info"],
	"canvas.edit_text": ["text.set"],
	"inspector.text": ["text.set"],
	// The palette runs any command of the registry with operations built from
	// its schema form, so it has no builder to check (rule 2 of section 9.3).
	"inspector.audio": ["item.set_audio"],
	"inspector.framing": ["clip.set_framing"],
	"inspector.speed": ["clip.set_speed"],
	"inspector.speed_curve": ["clip.set_speed"],
	"inspector.reverse": ["clip.set_speed"],
	"inspector.effects": ["clip.set_effects"],
	"timeline.toggle_link": ["item.link", "item.unlink"],
	"timeline.link": ["item.link"],
	"timeline.unlink": ["item.unlink"],
	"timeline.extract_audio": ["item.extract_audio", "track.add"],
	"inspector.color": ["clip.set_color"],
	"inspector.import_subtitles": ["caption.import_subtitles"],
	"inspector.color_keyframe": ["keyframe.set", "keyframe.remove"],
	"inspector.silences": ["clip.remove_silences"],
	"inspector.filter": ["clip.set_filter"],
	"inspector.blur": ["clip.set_blur"],
	"inspector.duck": ["audio.duck"],
	"inspector.mark_beats": ["audio.mark_beats"],
	"inspector.font_add": ["font.add"],
	"inspector.font_remove": ["font.remove"],
	"topbar.rename": ["project.rename"],
	"inspector.settings": ["settings.set"],
	"inspector.fps": ["settings.set_fps"],
	"inspector.aspect": ["settings.reframe"],
	"timeline.add_track": ["track.add"],
	"timeline.track_update": ["track.update"],
	"timeline.toggle_magnet": ["track.update"],
	"timeline.freeze_frame": ["item.freeze_frame"],
	"timeline.paste_attributes": ["item.paste_attributes"],
	"inspector.track_update": ["track.update"],
	"timeline.track_move": ["track.move"],
	"timeline.track_remove": ["track.remove"],
	"inspector.track_caption_style": ["track.set_caption_style"],
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
	"audio.duck": [
		{ kind: "inspector", action: "inspector.duck", phase: "B" },
		palette,
	],
	"audio.mark_beats": [
		{ kind: "inspector", action: "inspector.mark_beats", phase: "B" },
		palette,
	],
	"caption.add": [
		{ kind: "toolbar", action: "caption.add", phase: "P" },
		palette,
	],
	"caption.edit_word": [
		{ kind: "gesture", action: "timeline.edit_word", phase: "A" },
		{ kind: "inspector", action: "inspector.word_text", phase: "A" },
		palette,
	],
	"caption.from_transcript": [
		{ kind: "inspector", action: "inspector.auto_captions", phase: "A" },
		palette,
	],
	"caption.merge": [
		{ kind: "menu", action: "timeline.merge_captions", phase: "A" },
		palette,
	],
	"caption.retime_from_clip": [
		{ kind: "inspector", action: "inspector.resync_captions", phase: "A" },
		{ kind: "menu", action: "lint.resync_captions", phase: "A" },
		palette,
	],
	"caption.set_breaks": [
		{ kind: "inspector", action: "inspector.breaks", phase: "A" },
		palette,
	],
	"caption.set_style": [
		{ kind: "inspector", action: "inspector.caption_style", phase: "A" },
		palette,
	],
	"caption.set_words": [
		{ kind: "gesture", action: "timeline.word_boundary", phase: "A" },
		{ kind: "inspector", action: "inspector.words", phase: "A" },
		palette,
	],
	"caption.split": [
		{ kind: "menu", action: "timeline.split_caption", phase: "A" },
		{ kind: "inspector", action: "inspector.split_word", phase: "A" },
		palette,
	],
	"clip.add": [
		{ kind: "gesture", action: "library.add_to_timeline", phase: "P" },
		{ kind: "gesture", action: "library.drag", phase: "A" },
		palette,
	],
	"clip.remove_silences": [
		{ kind: "inspector", action: "inspector.silences", phase: "A" },
		palette,
	],
	"clip.set_blur": [
		{ kind: "inspector", action: "inspector.blur", phase: "B" },
		palette,
	],
	"clip.set_chroma_key": [
		{ kind: "inspector", action: "inspector.chroma_key", phase: "B" },
		palette,
	],
	"clip.set_blend": [
		{ kind: "inspector", action: "inspector.blend", phase: "B" },
		palette,
	],
	"clip.set_mask": [
		{ kind: "inspector", action: "inspector.mask", phase: "B" },
		palette,
	],
	"clip.set_flip": [
		{ kind: "inspector", action: "inspector.flip", phase: "B" },
		palette,
	],
	"clip.set_effects": [
		{ kind: "inspector", action: "inspector.effects", phase: "B" },
		palette,
	],
	"item.set_enabled": [
		{ kind: "key", action: "timeline.toggle_enabled", phase: "B" },
		{ kind: "menu", action: "timeline.toggle_enabled", phase: "B" },
		{ kind: "inspector", action: "inspector.enabled", phase: "B" },
		palette,
	],
	"item.link": [
		{ kind: "key", action: "timeline.toggle_link", phase: "B" },
		{ kind: "menu", action: "timeline.link", phase: "B" },
		palette,
	],
	"item.unlink": [
		{ kind: "key", action: "timeline.toggle_link", phase: "B" },
		{ kind: "menu", action: "timeline.unlink", phase: "B" },
		palette,
	],
	"item.extract_audio": [
		{ kind: "key", action: "timeline.extract_audio", phase: "B" },
		{ kind: "menu", action: "timeline.extract_audio", phase: "B" },
		palette,
	],
	"item.paste_attributes": [
		{ kind: "key", action: "timeline.paste_attributes", phase: "B" },
		{ kind: "menu", action: "timeline.paste_attributes", phase: "B" },
		palette,
	],
	"item.freeze_frame": [
		{ kind: "menu", action: "timeline.freeze_frame", phase: "B" },
		{ kind: "toolbar", action: "timeline.freeze_frame", phase: "B" },
		palette,
	],
	"caption.import_subtitles": [
		{ kind: "inspector", action: "inspector.import_subtitles", phase: "B" },
		palette,
	],
	"marker.add": [
		{ kind: "key", action: "timeline.marker", phase: "B" },
		palette,
	],
	"marker.remove": [
		{ kind: "key", action: "timeline.marker", phase: "B" },
		{ kind: "inspector", action: "inspector.marker", phase: "B" },
		palette,
	],
	"marker.set": [
		{ kind: "inspector", action: "inspector.marker", phase: "B" },
		palette,
	],
	"clip.set_color": [
		{ kind: "inspector", action: "inspector.color", phase: "A" },
		palette,
	],
	"clip.set_filter": [
		{ kind: "inspector", action: "inspector.filter", phase: "B" },
		palette,
	],
	"clip.set_framing": [
		{ kind: "inspector", action: "inspector.framing", phase: "A" },
		palette,
	],
	"clip.set_speed": [
		{ kind: "inspector", action: "inspector.speed", phase: "A" },
		{ kind: "inspector", action: "inspector.speed_curve", phase: "B" },
		{ kind: "inspector", action: "inspector.reverse", phase: "B" },
		palette,
	],
	"font.add": [
		{ kind: "inspector", action: "inspector.font_add", phase: "B" },
		palette,
	],
	"font.remove": [
		{ kind: "inspector", action: "inspector.font_remove", phase: "B" },
		palette,
	],
	"item.crossfade": [
		{ kind: "menu", action: "timeline.crossfade", phase: "A" },
		palette,
	],
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
	"item.repeat": [
		{ kind: "menu", action: "timeline.repeat", phase: "B" },
		palette,
	],
	"clip.set_shadow": [
		{ kind: "inspector", action: "inspector.shadow", phase: "B" },
		palette,
	],
	"clip.set_outline": [
		{ kind: "inspector", action: "inspector.outline", phase: "B" },
		palette,
	],
	"item.move": [
		{ kind: "inspector", action: "inspector.move", phase: "A" },
		{ kind: "key", action: "timeline.slide", phase: "A" },
		palette,
	],
	"item.rename": [
		{ kind: "inspector", action: "inspector.rename", phase: "A" },
		{ kind: "key", action: "timeline.rename", phase: "A" },
		{ kind: "menu", action: "timeline.rename", phase: "A" },
		palette,
	],
	"item.set_audio": [
		{ kind: "inspector", action: "inspector.audio", phase: "A" },
		palette,
	],
	"item.set_transform": [
		{ kind: "gesture", action: "canvas.transform", phase: "A" },
		{ kind: "inspector", action: "inspector.transform", phase: "A" },
		palette,
	],
	"item.set_transition": [
		{ kind: "inspector", action: "inspector.transition", phase: "A" },
		palette,
	],
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
		{ kind: "key", action: "timeline.roll", phase: "A" },
		palette,
	],
	"keyframe.clear": [
		{ kind: "inspector", action: "inspector.keyframe_clear", phase: "A" },
		palette,
	],
	"keyframe.remove": [
		{ kind: "inspector", action: "inspector.keyframe_remove", phase: "A" },
		{ kind: "inspector", action: "inspector.color_keyframe", phase: "A" },
		{ kind: "key", action: "timeline.delete_keyframe", phase: "A" },
		palette,
	],
	"keyframe.set": [
		{ kind: "gesture", action: "canvas.transform", phase: "A" },
		{ kind: "inspector", action: "inspector.transform", phase: "A" },
		{ kind: "menu", action: "notice.use_keyframes", phase: "A" },
		{ kind: "inspector", action: "inspector.keyframe_add", phase: "A" },
		{ kind: "inspector", action: "inspector.color_keyframe", phase: "A" },
		{ kind: "inspector", action: "inspector.keyframe_easing", phase: "A" },
		{ kind: "inspector", action: "inspector.keyframe_value", phase: "A" },
		palette,
	],
	"project.rename": [
		{ kind: "toolbar", action: "topbar.rename", phase: "A" },
		palette,
	],
	"item.slip": [{ kind: "key", action: "timeline.slip", phase: "A" }, palette],
	"settings.reframe": [
		{ kind: "inspector", action: "inspector.aspect", phase: "A" },
		palette,
	],
	"settings.set": [
		{ kind: "inspector", action: "inspector.settings", phase: "A" },
		palette,
	],
	"settings.set_fps": [
		{ kind: "inspector", action: "inspector.fps", phase: "A" },
		palette,
	],
	"shape.add": [{ kind: "toolbar", action: "shape.add", phase: "A" }, palette],
	"shape.set": [
		{ kind: "inspector", action: "inspector.shape", phase: "A" },
		palette,
	],
	"slot.fill": [
		{ kind: "inspector", action: "slots.fill", phase: "A" },
		palette,
	],
	"slot.mark": [
		{ kind: "inspector", action: "inspector.slot_mark", phase: "A" },
		palette,
	],
	"slot.unmark": [
		{ kind: "inspector", action: "inspector.slot_unmark", phase: "A" },
		palette,
	],
	"template.set_info": [
		{ kind: "inspector", action: "inspector.template_info", phase: "A" },
		palette,
	],
	"text.add": [
		{ kind: "toolbar", action: "text.add", phase: "A" },
		{ kind: "key", action: "text.add", phase: "A" },
		palette,
	],
	"text.apply_preset": [
		{ kind: "inspector", action: "inspector.animation", phase: "A" },
		palette,
	],
	"text.set": [
		{ kind: "gesture", action: "canvas.edit_text", phase: "A" },
		{ kind: "inspector", action: "inspector.text", phase: "A" },
		palette,
	],
	"timeline.close_gap": [
		{ kind: "menu", action: "timeline.close_gap", phase: "A" },
		palette,
	],
	"track.add": [
		{ kind: "gesture", action: "library.add_to_timeline", phase: "P" },
		{ kind: "toolbar", action: "caption.add", phase: "P" },
		{ kind: "menu", action: "timeline.add_track", phase: "A" },
		palette,
	],
	"track.move": [
		{ kind: "gesture", action: "timeline.track_move", phase: "A" },
		{ kind: "menu", action: "timeline.track_move", phase: "A" },
		palette,
	],
	"track.remove": [
		{ kind: "menu", action: "timeline.track_remove", phase: "A" },
		palette,
	],
	"track.set_caption_style": [
		{ kind: "inspector", action: "inspector.track_caption_style", phase: "A" },
		palette,
	],
	"track.update": [
		{ kind: "gesture", action: "timeline.track_update", phase: "A" },
		{ kind: "inspector", action: "inspector.track_update", phase: "A" },
		{ kind: "key", action: "timeline.toggle_magnet", phase: "B" },
		{ kind: "toolbar", action: "timeline.toggle_magnet", phase: "B" },
		palette,
	],
};

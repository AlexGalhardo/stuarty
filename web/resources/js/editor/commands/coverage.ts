// The coverage declaration of SDD-012 section 9.3: for each command of the
// catalogue that is not internal, the entry points that exist in code. The
// test in coverage.test.ts checks it against the committed registry snapshot,
// so a command added to the catalogue breaks the build until it has a key here.

export interface EntryPoint {
	kind: "gesture" | "inspector" | "menu" | "toolbar" | "key" | "palette";
	// An id of ACTIONS.
	action: string;
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
	"inspector.keyframe_easing": ["keyframe.set_easing"],
	"inspector.keyframe_curve": ["keyframe.set_easing"],
	"inspector.keyframe_value": ["keyframe.set"],
	"inspector.keyframe_remove": ["keyframe.remove"],
	"inspector.keyframe_clear": ["keyframe.clear"],
	"timeline.delete_keyframe": ["keyframe.remove"],
	"inspector.transition": ["item.set_transition"],
	"timeline.crossfade": ["item.crossfade"],
	"inspector.animation": ["text.apply_preset"],
	"inspector.motion": ["animation.apply_preset"],
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
	"inspector.audio_processors": [
		"audio.processor_add",
		"audio.processor_set",
		"audio.processor_move",
		"audio.processor_remove",
		"audio.apply_preset",
	],
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
// that builds it.
const palette: EntryPoint = {
	kind: "palette",
	action: "palette.run",
};

export const COVERAGE: Readonly<Record<string, readonly EntryPoint[]>> = {
	"animation.apply_preset": [
		{ kind: "inspector", action: "inspector.motion" },
		palette,
	],
	"animation.list_presets": [palette],
	"asset.remove": [{ kind: "menu", action: "library.remove" }, palette],
	"audio.add": [
		{ kind: "gesture", action: "library.add_to_timeline" },
		{ kind: "gesture", action: "library.drag" },
		palette,
	],
	"audio.apply_preset": [
		{ kind: "inspector", action: "inspector.audio_processors" },
		palette,
	],
	"audio.duck": [{ kind: "inspector", action: "inspector.duck" }, palette],
	"audio.processor_add": [
		{ kind: "inspector", action: "inspector.audio_processors" },
		palette,
	],
	"audio.processor_move": [
		{ kind: "inspector", action: "inspector.audio_processors" },
		palette,
	],
	"audio.processor_remove": [
		{ kind: "inspector", action: "inspector.audio_processors" },
		palette,
	],
	"audio.processor_set": [
		{ kind: "inspector", action: "inspector.audio_processors" },
		palette,
	],
	"audio.mark_beats": [
		{ kind: "inspector", action: "inspector.mark_beats" },
		palette,
	],
	"caption.add": [{ kind: "toolbar", action: "caption.add" }, palette],
	"caption.edit_word": [
		{ kind: "gesture", action: "timeline.edit_word" },
		{ kind: "inspector", action: "inspector.word_text" },
		palette,
	],
	"caption.from_transcript": [
		{ kind: "inspector", action: "inspector.auto_captions" },
		palette,
	],
	"caption.merge": [
		{ kind: "menu", action: "timeline.merge_captions" },
		palette,
	],
	"caption.retime_from_clip": [
		{ kind: "inspector", action: "inspector.resync_captions" },
		{ kind: "menu", action: "lint.resync_captions" },
		palette,
	],
	"caption.set_breaks": [
		{ kind: "inspector", action: "inspector.breaks" },
		palette,
	],
	"caption.set_style": [
		{ kind: "inspector", action: "inspector.caption_style" },
		palette,
	],
	"caption.set_words": [
		{ kind: "gesture", action: "timeline.word_boundary" },
		{ kind: "inspector", action: "inspector.words" },
		palette,
	],
	"caption.split": [
		{ kind: "menu", action: "timeline.split_caption" },
		{ kind: "inspector", action: "inspector.split_word" },
		palette,
	],
	"clip.add": [
		{ kind: "gesture", action: "library.add_to_timeline" },
		{ kind: "gesture", action: "library.drag" },
		palette,
	],
	"clip.remove_silences": [
		{ kind: "inspector", action: "inspector.silences" },
		palette,
	],
	"clip.replace_media": [palette],
	"clip.set_blur": [{ kind: "inspector", action: "inspector.blur" }, palette],
	"clip.set_chroma_key": [
		{ kind: "inspector", action: "inspector.chroma_key" },
		palette,
	],
	"clip.set_blend": [{ kind: "inspector", action: "inspector.blend" }, palette],
	"clip.set_mask": [{ kind: "inspector", action: "inspector.mask" }, palette],
	"clip.set_flip": [{ kind: "inspector", action: "inspector.flip" }, palette],
	"clip.set_effects": [
		{ kind: "inspector", action: "inspector.effects" },
		palette,
	],
	"item.set_enabled": [
		{ kind: "key", action: "timeline.toggle_enabled" },
		{ kind: "menu", action: "timeline.toggle_enabled" },
		{ kind: "inspector", action: "inspector.enabled" },
		palette,
	],
	"item.link": [
		{ kind: "key", action: "timeline.toggle_link" },
		{ kind: "menu", action: "timeline.link" },
		palette,
	],
	"item.unlink": [
		{ kind: "key", action: "timeline.toggle_link" },
		{ kind: "menu", action: "timeline.unlink" },
		palette,
	],
	"item.extract_audio": [
		{ kind: "key", action: "timeline.extract_audio" },
		{ kind: "menu", action: "timeline.extract_audio" },
		palette,
	],
	"item.paste_attributes": [
		{ kind: "key", action: "timeline.paste_attributes" },
		{ kind: "menu", action: "timeline.paste_attributes" },
		palette,
	],
	"item.freeze_frame": [
		{ kind: "menu", action: "timeline.freeze_frame" },
		{ kind: "toolbar", action: "timeline.freeze_frame" },
		palette,
	],
	"caption.import_subtitles": [
		{ kind: "inspector", action: "inspector.import_subtitles" },
		palette,
	],
	"marker.add": [{ kind: "key", action: "timeline.marker" }, palette],
	"marker.remove": [
		{ kind: "key", action: "timeline.marker" },
		{ kind: "inspector", action: "inspector.marker" },
		palette,
	],
	"marker.set": [{ kind: "inspector", action: "inspector.marker" }, palette],
	"clip.set_color": [{ kind: "inspector", action: "inspector.color" }, palette],
	"clip.set_filter": [
		{ kind: "inspector", action: "inspector.filter" },
		palette,
	],
	"clip.set_framing": [
		{ kind: "inspector", action: "inspector.framing" },
		palette,
	],
	"clip.set_speed": [
		{ kind: "inspector", action: "inspector.speed" },
		{ kind: "inspector", action: "inspector.speed_curve" },
		{ kind: "inspector", action: "inspector.reverse" },
		palette,
	],
	"font.add": [{ kind: "inspector", action: "inspector.font_add" }, palette],
	"font.remove": [
		{ kind: "inspector", action: "inspector.font_remove" },
		palette,
	],
	"item.crossfade": [{ kind: "menu", action: "timeline.crossfade" }, palette],
	"item.delete": [
		{ kind: "key", action: "timeline.delete" },
		{ kind: "key", action: "timeline.ripple_delete" },
		{ kind: "toolbar", action: "timeline.delete" },
		{ kind: "menu", action: "timeline.delete" },
		{ kind: "menu", action: "timeline.ripple_delete" },
		palette,
	],
	"item.duplicate": [
		{ kind: "key", action: "timeline.duplicate" },
		{ kind: "menu", action: "timeline.duplicate" },
		palette,
	],
	"item.repeat": [{ kind: "menu", action: "timeline.repeat" }, palette],
	"clip.set_shadow": [
		{ kind: "inspector", action: "inspector.shadow" },
		palette,
	],
	"clip.set_outline": [
		{ kind: "inspector", action: "inspector.outline" },
		palette,
	],
	"item.move": [
		{ kind: "inspector", action: "inspector.move" },
		{ kind: "key", action: "timeline.slide" },
		palette,
	],
	"item.rename": [
		{ kind: "inspector", action: "inspector.rename" },
		{ kind: "key", action: "timeline.rename" },
		{ kind: "menu", action: "timeline.rename" },
		palette,
	],
	"item.set_audio": [{ kind: "inspector", action: "inspector.audio" }, palette],
	"item.set_transform": [
		{ kind: "gesture", action: "canvas.transform" },
		{ kind: "inspector", action: "inspector.transform" },
		palette,
	],
	"item.set_transition": [
		{ kind: "inspector", action: "inspector.transition" },
		palette,
	],
	"item.split": [
		{ kind: "toolbar", action: "timeline.split" },
		{ kind: "key", action: "timeline.split" },
		palette,
	],
	"item.trim": [
		{ kind: "toolbar", action: "timeline.trim_start" },
		{ kind: "toolbar", action: "timeline.trim_end" },
		{ kind: "key", action: "timeline.trim_start" },
		{ kind: "key", action: "timeline.trim_end" },
		{ kind: "inspector", action: "inspector.trim_end" },
		{ kind: "key", action: "timeline.roll" },
		palette,
	],
	"keyframe.clear": [
		{ kind: "inspector", action: "inspector.keyframe_clear" },
		palette,
	],
	"keyframe.remove": [
		{ kind: "inspector", action: "inspector.keyframe_remove" },
		{ kind: "inspector", action: "inspector.color_keyframe" },
		{ kind: "key", action: "timeline.delete_keyframe" },
		palette,
	],
	"keyframe.set": [
		{ kind: "gesture", action: "canvas.transform" },
		{ kind: "inspector", action: "inspector.transform" },
		{ kind: "menu", action: "notice.use_keyframes" },
		{ kind: "inspector", action: "inspector.keyframe_add" },
		{ kind: "inspector", action: "inspector.color_keyframe" },
		{ kind: "inspector", action: "inspector.keyframe_value" },
		palette,
	],
	"keyframe.set_easing": [
		{ kind: "inspector", action: "inspector.keyframe_easing" },
		{ kind: "inspector", action: "inspector.keyframe_curve" },
		palette,
	],
	"project.rename": [{ kind: "toolbar", action: "topbar.rename" }, palette],
	"item.slip": [{ kind: "key", action: "timeline.slip" }, palette],
	"settings.reframe": [
		{ kind: "inspector", action: "inspector.aspect" },
		palette,
	],
	"settings.set": [
		{ kind: "inspector", action: "inspector.settings" },
		palette,
	],
	"settings.set_fps": [{ kind: "inspector", action: "inspector.fps" }, palette],
	"shape.add": [{ kind: "toolbar", action: "shape.add" }, palette],
	"shape.set": [{ kind: "inspector", action: "inspector.shape" }, palette],
	"slot.fill": [{ kind: "inspector", action: "slots.fill" }, palette],
	"slot.mark": [{ kind: "inspector", action: "inspector.slot_mark" }, palette],
	"slot.unmark": [
		{ kind: "inspector", action: "inspector.slot_unmark" },
		palette,
	],
	"template.set_info": [
		{ kind: "inspector", action: "inspector.template_info" },
		palette,
	],
	"text.add": [
		{ kind: "toolbar", action: "text.add" },
		{ kind: "key", action: "text.add" },
		palette,
	],
	"text.apply_preset": [
		{ kind: "inspector", action: "inspector.animation" },
		palette,
	],
	"text.set": [
		{ kind: "gesture", action: "canvas.edit_text" },
		{ kind: "inspector", action: "inspector.text" },
		palette,
	],
	"timeline.close_gap": [
		{ kind: "menu", action: "timeline.close_gap" },
		palette,
	],
	"track.add": [
		{ kind: "gesture", action: "library.add_to_timeline" },
		{ kind: "toolbar", action: "caption.add" },
		{ kind: "menu", action: "timeline.add_track" },
		palette,
	],
	"track.move": [
		{ kind: "gesture", action: "timeline.track_move" },
		{ kind: "menu", action: "timeline.track_move" },
		palette,
	],
	"track.remove": [{ kind: "menu", action: "timeline.track_remove" }, palette],
	"track.set_caption_style": [
		{ kind: "inspector", action: "inspector.track_caption_style" },
		palette,
	],
	"track.update": [
		{ kind: "gesture", action: "timeline.track_update" },
		{ kind: "inspector", action: "inspector.track_update" },
		{ kind: "key", action: "timeline.toggle_magnet" },
		{ kind: "toolbar", action: "timeline.toggle_magnet" },
		palette,
	],
};

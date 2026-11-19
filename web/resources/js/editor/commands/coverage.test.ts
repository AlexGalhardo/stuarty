import { describe, expect, test } from "vitest";

import registry from "@/generated/commands.json";
import {
	addAudioOperations,
	addClipOperations,
	audioOperations,
	blendOperations,
	blurOperations,
	captionBreaksOperations,
	captionEditWordOperations,
	captionFromTranscriptOperations,
	captionImportSubtitlesOperations,
	captionMergeOperations,
	captionOperations,
	captionRetimeOperations,
	captionSplitOperations,
	captionStyleOperations,
	captionWordsOperations,
	chromaKeyOperations,
	closeGapOperations,
	colorOperations,
	crossfadeOperations,
	deleteOperations,
	dropAssetOperations,
	duckOperations,
	duplicateOperations,
	effectsOperations,
	enabledOperations,
	extractAudioOperations,
	filterOperations,
	flipOperations,
	fontAddOperations,
	fontRemoveOperations,
	framingOperations,
	freezeFrameOperations,
	keyframeClearOperations,
	keyframeEasingOperations,
	keyframeFallbackOperations,
	keyframeRemoveOperations,
	keyframeSetOperations,
	linkOperations,
	markBeatsOperations,
	markerAddOperations,
	markerRemoveOperations,
	markerSetOperations,
	maskOperations,
	motionPresetOperations,
	moveOperations,
	outlineOperations,
	pasteAttributesOperations,
	presetOperations,
	projectRenameOperations,
	reframeOperations,
	removeAssetOperations,
	renameOperations,
	repeatOperations,
	reverseOperations,
	rollOperations,
	settingsFpsOperations,
	settingsSetOperations,
	shadowOperations,
	shapeAddOperations,
	shapeSetOperations,
	silenceOperations,
	slideOperations,
	slipOperations,
	slotFillOperations,
	slotMarkOperations,
	slotUnmarkOperations,
	speedCurveOperations,
	speedOperations,
	splitOperations,
	templateInfoOperations,
	textAddOperations,
	textSetOperations,
	textStyleOperations,
	trackAddOperations,
	trackCaptionStyleOperations,
	trackMoveOperations,
	trackRemoveOperations,
	trackUpdateOperations,
	transformOperations,
	transitionOperations,
	trimEndOperations,
	trimOperations,
	unlinkOperations,
} from "./builders";
import { ACTIONS, ANY_COMMAND, COVERAGE, type EntryPoint } from "./coverage";

interface RegistryEntry {
	name: string;
	internal: boolean;
}

const commands = registry as RegistryEntry[];
const byName = new Map(commands.map((c) => [c.name, c]));

type Coverage = Readonly<Record<string, readonly EntryPoint[]>>;

// The rules of SDD-012 section 9.3, as a function so that the negative
// fixtures below can run them on a damaged declaration.
function coverageProblems(
	list: readonly RegistryEntry[],
	coverage: Coverage,
): string[] {
	const problems: string[] = [];
	const names = new Set(list.map((c) => c.name));
	for (const c of list) {
		if (c.internal) continue;
		const entries = coverage[c.name] ?? [];
		if (entries.length === 0) {
			problems.push(`${c.name} has no entry point`);
			continue;
		}
		if (!entries.some((e) => e.kind === "palette")) {
			problems.push(`${c.name} has no palette entry`);
		}
	}
	for (const name of Object.keys(coverage)) {
		if (!names.has(name)) problems.push(`${name} is not in the registry`);
		for (const entry of coverage[name]) {
			const sends = ACTIONS[entry.action];
			if (sends === undefined)
				problems.push(`${entry.action} is not an action`);
			else if (!sends.includes(ANY_COMMAND) && !sends.includes(name)) {
				problems.push(`${entry.action} does not send ${name}`);
			}
		}
	}
	return problems;
}

describe("command coverage (SDD-012 section 9.3)", () => {
	test("the declaration of the committed registry is complete and consistent", () => {
		expect(coverageProblems(commands, COVERAGE)).toEqual([]);
	});

	test("every non-internal command has a key", () => {
		const keys = commands.filter((c) => !c.internal).map((c) => c.name);
		expect(Object.keys(COVERAGE).sort()).toEqual(keys.sort());
		expect(keys).toHaveLength(77);
	});

	test("every command has a palette entry, and the commands with an inspector control keep it", () => {
		expect(coverageProblems(commands, COVERAGE)).toEqual([]);
		for (const name of [
			"audio.duck",
			"audio.mark_beats",
			"clip.set_filter",
			"clip.set_blur",
			"font.add",
			"font.remove",
		]) {
			expect(
				COVERAGE[name]?.some((e) => e.kind === "inspector"),
				name,
			).toBe(true);
		}
	});

	test("every key of COVERAGE is a non-internal command of the registry", () => {
		for (const name of Object.keys(COVERAGE)) {
			expect(byName.has(name), `${name} is not in the registry`).toBe(true);
			expect(byName.get(name)?.internal, name).toBe(false);
		}
	});

	test("the commands of the first UI keep a direct entry point", () => {
		for (const name of [
			"clip.add",
			"track.add",
			"item.split",
			"item.trim",
			"caption.add",
		]) {
			expect(
				(COVERAGE[name] ?? []).some((e) => e.kind !== "palette"),
				name,
			).toBe(true);
		}
	});

	test("negative: deleting one key of COVERAGE fails with the command name", () => {
		const { "item.move": _removed, ...rest } = COVERAGE;
		expect(coverageProblems(commands, rest)).toContain(
			"item.move has no entry point",
		);
	});

	test("negative: a command added to the catalogue with no key fails", () => {
		const grown = [...commands, { name: "clip.set_example", internal: false }];
		expect(coverageProblems(grown, COVERAGE)).toContain(
			"clip.set_example has no entry point",
		);
	});

	test("negative: a command with no palette entry fails", () => {
		const bare = { ...COVERAGE, "text.add": [] as EntryPoint[] };
		bare["text.add"] = [{ kind: "gesture", action: "timeline.split" }];
		const problems = coverageProblems(commands, bare);
		expect(problems).toContain("text.add has no palette entry");
	});

	test("negative: a stale key and an unknown action fail", () => {
		const stale = {
			...COVERAGE,
			"gone.command": [
				{ kind: "palette", action: "palette.run" } as EntryPoint,
			],
			"item.move": [{ kind: "palette", action: "no.such" } as EntryPoint],
		};
		const problems = coverageProblems(commands, stale);
		expect(problems).toContain("gone.command is not in the registry");
		expect(problems).toContain("no.such is not an action");
	});

	test("the builders emit the operations the actions declare", () => {
		const items = [{ id: "itm_a", start: 0, duration: 100 }];
		const newTrackId = (): string => "trk_a";
		const setInput = {
			itemId: "itm_a",
			property: "scale" as const,
			frame: 10,
			value: 1.5,
			easing: "ease_in" as const,
		};
		const emitted: Record<string, string[]> = {
			"library.add_to_timeline": addClipOperations({
				assetId: "ast_a",
				firstVisualTrack: null,
				newTrackId,
			})
				.concat(
					addAudioOperations({
						assetId: "ast_a",
						firstAudioTrack: null,
						newTrackId,
					}),
				)
				.map((o) => o.op),
			"library.drag": dropAssetOperations({
				assetId: "ast_a",
				assetKind: "audio",
				trackId: "trk_a",
				trackKind: "audio",
				start: 3,
			}).map((o) => o.op),
			"library.remove": removeAssetOperations("ast_a").map((o) => o.op),
			"timeline.split": splitOperations(items, 10).map((o) => o.op),
			"timeline.marker": markerAddOperations(10)
				.concat(markerRemoveOperations(10))
				.map((o) => o.op),
			"inspector.marker": markerSetOperations(10, { label: "a" })
				.concat(markerRemoveOperations(10, "itm_a"))
				.map((o) => o.op),
			"timeline.trim_start": trimOperations(items, "start", 10).map(
				(o) => o.op,
			),
			"timeline.trim_end": trimOperations(items, "end", 10).map((o) => o.op),
			"timeline.slide": slideOperations(items, 1).map((o) => o.op),
			"timeline.roll": rollOperations(items, 1).map((o) => o.op),
			"timeline.slip": slipOperations([{ id: "itm_a", sourceIn: 0 }], 1).map(
				(o) => o.op,
			),
			"inspector.rename": renameOperations("itm_a", "x").map((o) => o.op),
			"timeline.rename": renameOperations("itm_a", "x").map((o) => o.op),
			"timeline.delete": deleteOperations(["itm_a"]).map((o) => o.op),
			"timeline.ripple_delete": deleteOperations(["itm_a"], "all").map(
				(o) => o.op,
			),
			"timeline.duplicate": duplicateOperations(items).map((o) => o.op),
			"timeline.toggle_enabled": enabledOperations("itm_a", false).map(
				(o) => o.op,
			),
			"timeline.repeat": repeatOperations("itm_a", { copies: 2 }).map(
				(o) => o.op,
			),
			"inspector.shadow": shadowOperations("itm_a", null).map((o) => o.op),
			"inspector.outline": outlineOperations("itm_a", null).map((o) => o.op),
			"inspector.enabled": enabledOperations("itm_a", true).map((o) => o.op),
			"inspector.blend": blendOperations("itm_a", "screen").map((o) => o.op),
			"inspector.mask": maskOperations("itm_a", null).map((o) => o.op),
			"inspector.chroma_key": chromaKeyOperations("itm_a", null).map(
				(o) => o.op,
			),
			"inspector.flip": flipOperations("itm_a", { horizontal: true }).map(
				(o) => o.op,
			),
			"timeline.close_gap": closeGapOperations("trk_a", 5).map((o) => o.op),
			"inspector.move": moveOperations("itm_a", "1s").map((o) => o.op),
			"inspector.trim_end": trimEndOperations("itm_a", "2s").map((o) => o.op),
			"canvas.transform": transformOperations(
				[{ id: "itm_a", animated: ["scale"] }],
				{ x: 1, scale: 2 },
				10,
			).map((o) => o.op),
			"inspector.transform": transformOperations(
				[{ id: "itm_a", animated: ["opacity"] }],
				{ opacity: 0.5 },
				10,
			).map((o) => o.op),
			"notice.use_keyframes": keyframeFallbackOperations(
				transformOperations([{ id: "itm_a", animated: [] }], { x: 1 }, 10),
				10,
			).map((o) => o.op),
			"timeline.edit_word": captionEditWordOperations("itm_a", 0, "x").map(
				(o) => o.op,
			),
			"inspector.word_text": captionEditWordOperations("itm_a", 1, "").map(
				(o) => o.op,
			),
			"timeline.word_boundary": captionWordsOperations("itm_a", []).map(
				(o) => o.op,
			),
			"inspector.words": captionWordsOperations("itm_a", []).map((o) => o.op),
			"inspector.breaks": captionBreaksOperations("itm_a", [2]).map(
				(o) => o.op,
			),
			"timeline.split_caption": captionSplitOperations("itm_a", 1).map(
				(o) => o.op,
			),
			"inspector.split_word": captionSplitOperations("itm_a", 2).map(
				(o) => o.op,
			),
			"timeline.merge_captions": captionMergeOperations(["a", "b"]).map(
				(o) => o.op,
			),
			"inspector.caption_style": captionStyleOperations("itm_a", null).map(
				(o) => o.op,
			),
			"inspector.keyframe_add": keyframeSetOperations(setInput).map(
				(o) => o.op,
			),
			"inspector.keyframe_easing": keyframeEasingOperations({
				itemId: "itm_a",
				property: "scale",
				frame: 10,
				easing: "hold",
			}).map((o) => o.op),
			"inspector.keyframe_curve": keyframeEasingOperations({
				itemId: "itm_a",
				property: "scale",
				frame: 10,
				bezier: [0.1, 0.2, 0.3, 0.4],
			}).map((o) => o.op),
			"inspector.keyframe_value": keyframeSetOperations(setInput).map(
				(o) => o.op,
			),
			"inspector.keyframe_remove": keyframeRemoveOperations(
				"itm_a",
				"scale",
				10,
			).map((o) => o.op),
			"timeline.delete_keyframe": keyframeRemoveOperations(
				"itm_a",
				"x",
				10,
			).map((o) => o.op),
			"inspector.keyframe_clear": keyframeClearOperations("itm_a").map(
				(o) => o.op,
			),
			"inspector.transition": transitionOperations({
				itemId: "itm_a",
				edge: "in",
				kind: null,
			}).map((o) => o.op),
			"timeline.crossfade": crossfadeOperations(
				{ id: "a", start: 0, duration: 10 },
				{ id: "b", start: 10, duration: 10 },
			).map((o) => o.op),
			"inspector.animation": presetOperations({
				itemId: "itm_a",
				edge: "out",
				preset: "pop",
			}).map((o) => o.op),
			"inspector.motion": motionPresetOperations({
				itemId: "itm_a",
				preset: "slide",
				edge: "in",
				direction: "left",
			}).map((o) => o.op),
			"inspector.auto_captions": captionFromTranscriptOperations({
				clipId: "itm_a",
				trackId: null,
				newTrackId,
				trackName: "Captions",
				maxWords: 5,
				maxChars: 28,
				maxGap: "0.6s",
				replace: true,
			}).map((o) => o.op),
			"inspector.resync_captions": captionRetimeOperations("itm_a").map(
				(o) => o.op,
			),
			"lint.resync_captions": captionRetimeOperations("itm_a").map((o) => o.op),
			"text.add": textAddOperations({
				text: "a",
				playhead: 0,
			}).map((o) => o.op),
			"shape.add": shapeAddOperations({
				shape: "ellipse",
				playhead: 0,
			}).map((o) => o.op),
			"inspector.shape": shapeSetOperations("itm_a", { width: 10 }).map(
				(o) => o.op,
			),
			"inspector.slot_mark": slotMarkOperations("itm_a", {
				name: "title",
				description: "",
				required: true,
			}).map((o) => o.op),
			"inspector.slot_unmark": slotUnmarkOperations("itm_a").map((o) => o.op),
			"slots.fill": slotFillOperations("title", { text: "x" }).map((o) => o.op),
			"inspector.template_info": templateInfoOperations({
				description: "d",
			}).map((o) => o.op),
			"canvas.edit_text": textSetOperations("itm_a", "x").map((o) => o.op),
			"inspector.text": textStyleOperations("itm_a", { size: 90 }).map(
				(o) => o.op,
			),
			"inspector.audio": audioOperations("itm_a", { volume: 2 }).map(
				(o) => o.op,
			),
			"inspector.framing": framingOperations("itm_a", { fit: "none" }).map(
				(o) => o.op,
			),
			"inspector.speed": speedOperations("itm_a", 1.5, true).map((o) => o.op),
			"inspector.speed_curve": speedCurveOperations(
				"itm_a",
				[
					{ t: 0, speed: 1 },
					{ t: 1, speed: 2 },
				],
				false,
			).map((o) => o.op),
			"inspector.reverse": reverseOperations("itm_a", true).map((o) => o.op),
			"inspector.effects": effectsOperations("itm_a", []).map((o) => o.op),
			"timeline.toggle_link": linkOperations(["itm_a", "itm_b"])
				.concat(unlinkOperations(["itm_a"]))
				.map((o) => o.op),
			"timeline.link": linkOperations(["itm_a", "itm_b"]).map((o) => o.op),
			"timeline.unlink": unlinkOperations(["itm_a"]).map((o) => o.op),
			"timeline.extract_audio": extractAudioOperations({
				itemId: "itm_a",
				audioTrack: null,
				newTrackId: () => "trk_new",
			}).map((o) => o.op),
			"inspector.color": colorOperations("itm_a", { contrast: 1.2 }).map(
				(o) => o.op,
			),
			"inspector.color_keyframe": keyframeSetOperations({
				itemId: "itm_a",
				property: "hue",
				frame: 10,
				value: 20,
			})
				.concat(keyframeRemoveOperations("itm_a", "hue", 10))
				.map((o) => o.op),
			"inspector.import_subtitles": captionImportSubtitlesOperations({
				trackId: "trk_a",
				text: "1\n00:00:01,000 --> 00:00:02,000\nHi\n",
			}).map((o) => o.op),
			"inspector.silences": silenceOperations("itm_a", {
				minSilence: "0.4s",
				padding: "0.1s",
				ripple: true,
			}).map((o) => o.op),
			"inspector.filter": filterOperations("itm_a", { filter: "sepia" }).map(
				(o) => o.op,
			),
			"inspector.blur": blurOperations("itm_a", 12).map((o) => o.op),
			"inspector.duck": duckOperations("itm_a", {
				speechItemIds: ["itm_b"],
				gainDb: -12,
				attack: "0.2s",
				release: "0.5s",
			}).map((o) => o.op),
			"inspector.mark_beats": markBeatsOperations("itm_a", {
				every: 2,
				label: "",
				replace: true,
			}).map((o) => o.op),
			"inspector.font_add": fontAddOperations("Brand Sans", [
				{ weight: 400, hash: `sha256:${"a".repeat(64)}` },
			]).map((o) => o.op),
			"inspector.font_remove": fontRemoveOperations("Brand Sans").map(
				(o) => o.op,
			),
			"topbar.rename": projectRenameOperations("x").map((o) => o.op),
			"inspector.settings": settingsSetOperations({ width: 720 }).map(
				(o) => o.op,
			),
			"inspector.fps": settingsFpsOperations("30").map((o) => o.op),
			"inspector.aspect": reframeOperations("1:1").map((o) => o.op),
			"timeline.add_track": trackAddOperations("audio", { above: "trk_a" }).map(
				(o) => o.op,
			),
			"timeline.toggle_magnet": trackUpdateOperations("trk_a", {
				magnet: true,
			}).map((o) => o.op),
			"timeline.paste_attributes": pasteAttributesOperations("itm_a", [
				"itm_b",
			]).map((o) => o.op),
			"timeline.freeze_frame": freezeFrameOperations(
				[{ id: "itm_a", start: 0, duration: 30 }],
				10,
			).map((o) => o.op),
			"timeline.track_update": trackUpdateOperations("trk_a", {
				hidden: true,
			}).map((o) => o.op),
			"inspector.track_update": trackUpdateOperations("trk_a", {
				name: "x",
			}).map((o) => o.op),
			"timeline.track_move": trackMoveOperations("trk_a", {
				below: "trk_b",
			}).map((o) => o.op),
			"timeline.track_remove": trackRemoveOperations("trk_a", true).map(
				(o) => o.op,
			),
			"inspector.track_caption_style": trackCaptionStyleOperations(
				"trk_a",
				{ size: 90 },
				false,
			).map((o) => o.op),
			"caption.add": captionOperations({
				text: "a",
				duration: "1s",
				playhead: 0,
			}).map((o) => o.op),
		};
		for (const [action, ops] of Object.entries(emitted)) {
			expect(ops.length, action).toBeGreaterThan(0);
			for (const op of ops)
				expect(ACTIONS[action], `${action} sends ${op}`).toContain(op);
		}
		// Every action but the palette has a builder checked above.
		for (const action of Object.keys(ACTIONS)) {
			if (action !== "palette.run")
				expect(emitted[action], action).toBeDefined();
		}
	});
});

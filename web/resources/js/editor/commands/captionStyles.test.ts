import { describe, expect, test } from "vitest";

import { blurRegionOperations, captionApplyStyleOperations } from "./builders";
import { CAPTION_STYLES, captionStyleSample } from "./captionStyles";
import { applyVoicePresetOperations } from "./processorBuilders";
import { VOICE_PRESETS } from "./processorTable";

describe("the caption style gallery data (SDD-002 C.88)", () => {
	test("the registry carries the 14 styles, each with a bundled font", () => {
		expect(CAPTION_STYLES).toHaveLength(14);
		expect(new Set(CAPTION_STYLES.map((s) => s.id)).size).toBe(14);
		for (const style of CAPTION_STYLES) {
			expect(style.caption_style.font_family, style.id).toBe(style.font_family);
			expect(style.name.length, style.id).toBeGreaterThan(0);
		}
	});

	test("a sample keeps the colours, the edge and the box of the stored style", () => {
		const bold = CAPTION_STYLES.find((s) => s.id === "bold_pop");
		expect(bold).toBeDefined();
		const css = captionStyleSample(bold?.caption_style ?? {});
		expect(css.color).toBe("#FFFFFF");
		expect(css.textTransform).toBe("uppercase");
		expect(css.WebkitTextStroke).toContain("#000000");

		const box = CAPTION_STYLES.find((s) => s.id === "soft_box");
		const boxed = captionStyleSample(box?.caption_style ?? {});
		expect(boxed.backgroundColor).toBe("#111827CC");
	});
});

describe("the builders of epic 71", () => {
	test("caption.apply_style sends the id, and reset_overrides only when asked", () => {
		expect(captionApplyStyleOperations("trk_a", "bold_pop", false)).toEqual([
			{
				op: "caption.apply_style",
				args: { track_id: "trk_a", style: "bold_pop" },
			},
		]);
		expect(
			captionApplyStyleOperations("trk_a", "bold_pop", true)[0]?.args,
		).toEqual({ track_id: "trk_a", style: "bold_pop", reset_overrides: true });
	});

	test("clip.blur_region leaves out the time and the radius the person did not give", () => {
		const bare = blurRegionOperations("itm_a", {
			x: 0.1,
			y: 0.6,
			width: 0.8,
			height: 0.1,
		});
		expect(bare[0]?.args).toEqual({
			item_id: "itm_a",
			x: 0.1,
			y: 0.6,
			width: 0.8,
			height: 0.1,
		});
		const full = blurRegionOperations("itm_a", {
			x: 0,
			y: 0,
			width: 1,
			height: 1,
			start: "1s",
			duration: "2s",
			radius: 40,
		});
		expect(full[0]?.args).toMatchObject({
			start: "1s",
			duration: "2s",
			radius: 40,
		});
	});

	test("the voice presets come from the registry and apply to an item or a track", () => {
		expect(VOICE_PRESETS).toEqual(["studio", "broadcast", "room_rescue"]);
		expect(
			applyVoicePresetOperations({ trackId: "trk_a" }, "broadcast"),
		).toEqual([
			{
				op: "audio.apply_voice_preset",
				args: { track_id: "trk_a", preset: "broadcast" },
			},
		]);
	});
});

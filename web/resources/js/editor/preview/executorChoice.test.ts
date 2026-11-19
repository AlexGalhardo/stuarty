import { describe, expect, it } from "vitest";

import {
	chooseExecutor,
	type FeatureProbe,
	missingFeatures,
	probeFeatures,
	readSetting,
} from "./executorChoice";

const everything: FeatureProbe = {
	Worker: true,
	WebAssembly: true,
	OffscreenCanvas: true,
	createImageBitmap: true,
	VideoDecoder: true,
	VideoFrame: true,
	"crypto.subtle": true,
};

const noop = (): void => undefined;

describe("executor choice", () => {
	it("reads the query before the stored value and ignores anything else", () => {
		expect(readSetting("?executor=rust", "pixi")).toBe("rust");
		expect(readSetting("", "pixi")).toBe("pixi");
		expect(readSetting("?executor=gpu", null)).toBe("auto");
		expect(readSetting("", null)).toBe("auto");
	});

	it("names every missing feature", () => {
		const probe = probeFeatures({
			Worker: noop,
			WebAssembly: {},
		});
		expect(missingFeatures(probe)).toEqual([
			"OffscreenCanvas",
			"createImageBitmap",
			"VideoDecoder",
			"VideoFrame",
			"crypto.subtle",
		]);
		expect(missingFeatures(everything)).toEqual([]);
	});

	it("uses the renderer of the export when forced and every feature is there", () => {
		expect(chooseExecutor("rust", [])).toEqual({ kind: "rust", reason: null });
	});

	it("falls back to PixiJS and says which feature is missing", () => {
		expect(chooseExecutor("rust", ["VideoDecoder", "OffscreenCanvas"])).toEqual(
			{
				kind: "pixi",
				reason: "the browser lacks VideoDecoder, OffscreenCanvas",
			},
		);
	});

	it("keeps PixiJS when forced, and follows the default when there is no setting", () => {
		expect(chooseExecutor("pixi", [])).toEqual({
			kind: "pixi",
			reason: "forced by the setting",
		});
		expect(chooseExecutor("auto", [], "rust").kind).toBe("rust");
		expect(chooseExecutor("auto", ["Worker"], "rust").kind).toBe("pixi");
		expect(chooseExecutor("auto", [], "pixi")).toEqual({
			kind: "pixi",
			reason: null,
		});
	});
});

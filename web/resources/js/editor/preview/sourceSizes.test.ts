import { describe, expect, it } from "vitest";

import type { MediaInspectResult } from "../api/types";
import { inspectSourceSize, type SourceSize, SourceSizes } from "./sourceSizes";

function rig(open: Record<string, SourceSize> = {}) {
	const asked: string[] = [];
	const answers = new Map<string, (size: SourceSize | null) => void>();
	let changed = 0;
	const sizes = new SourceSizes(
		(asset) => open[asset] ?? null,
		(timeline, asset) => {
			asked.push(`${timeline}/${asset}`);
			return new Promise((resolve) =>
				answers.set(`${timeline}/${asset}`, resolve),
			);
		},
		() => {
			changed++;
		},
	);
	return {
		sizes,
		asked,
		answer: async (key: string, size: SourceSize | null) => {
			answers.get(key)?.(size);
			await Promise.resolve();
			await Promise.resolve();
		},
		changed: () => changed,
	};
}

describe("SourceSizes", () => {
	it("answers from the open document and asks nothing", () => {
		const r = rig({ ast_a: { width: 1920, height: 1080 } });
		expect(r.sizes.of("ast_a", "prj_open", "prj_open")).toEqual({
			width: 1920,
			height: 1080,
		});
		// An asset of a nested timeline that the open document names too is the
		// same file (a clash is refused when a composition is expanded).
		expect(r.sizes.of("ast_a", "prj_nested", "prj_open")).toEqual({
			width: 1920,
			height: 1080,
		});
		expect(r.asked).toEqual([]);
	});

	it("does not ask for an asset of the open timeline it does not know", () => {
		const r = rig();
		expect(r.sizes.of("ast_x", "prj_open", "prj_open")).toBeNull();
		expect(r.asked).toEqual([]);
	});

	it("asks the timeline of a nested asset once, then remembers it", async () => {
		const r = rig();
		expect(r.sizes.of("ast_n", "prj_nested", "prj_open")).toBeUndefined();
		expect(r.sizes.of("ast_n", "prj_nested", "prj_open")).toBeUndefined();
		expect(r.asked).toEqual(["prj_nested/ast_n"]);
		await r.answer("prj_nested/ast_n", { width: 1280, height: 720 });
		expect(r.changed()).toBe(1);
		expect(r.sizes.of("ast_n", "prj_nested", "prj_open")).toEqual({
			width: 1280,
			height: 720,
		});
		expect(r.asked).toHaveLength(1);
	});

	it("takes the poster as the source when the timeline gives no size", async () => {
		const r = rig();
		r.sizes.of("ast_n", "prj_nested", "prj_open");
		await r.answer("prj_nested/ast_n", null);
		expect(r.sizes.of("ast_n", "prj_nested", "prj_open")).toBeNull();
		expect(r.asked).toHaveLength(1);
	});
});

describe("inspectSourceSize", () => {
	const result = (probe?: Record<string, unknown>): MediaInspectResult =>
		({
			asset_id: "ast_n",
			kind: "image",
			state: "ready",
			progress: 1,
			available: [],
			probe,
			next_cursor: null,
			text: "",
		}) satisfies MediaInspectResult;

	it("reads the width and the height of the probe", async () => {
		const seen: string[] = [];
		const size = await inspectSourceSize(
			{
				inspectMedia: async (timeline, asset) => {
					seen.push(`${timeline}/${asset}`);
					return result({ kind: "image", width: 1280, height: 720 });
				},
			},
			"prj_nested",
			"ast_n",
		);
		expect(size).toEqual({ width: 1280, height: 720 });
		expect(seen).toEqual(["prj_nested/ast_n"]);
	});

	it("is null without a probe or with no size in it", async () => {
		for (const probe of [
			undefined,
			{ kind: "audio" },
			{ width: 0, height: 4 },
		]) {
			expect(
				await inspectSourceSize(
					{ inspectMedia: async () => result(probe) },
					"prj_nested",
					"ast_n",
				),
			).toBeNull();
		}
	});
});

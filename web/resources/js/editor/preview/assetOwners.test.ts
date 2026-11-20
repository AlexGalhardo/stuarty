import { describe, expect, it } from "vitest";

import type { PlanWindow } from "../api/types";
import { AssetOwners } from "./assetOwners";
import { layersAt } from "./planValue";

function window(extra: Partial<PlanWindow>): PlanWindow {
	return {
		layers: [],
		audio: [],
		...extra,
	} as unknown as PlanWindow;
}

const base = {
	first: 0,
	last: 9,
	cx: 0,
	cy: 0,
	w: 1,
	h: 1,
	rotation: 0,
	scale: 1,
	opacity: 1,
};

describe("AssetOwners", () => {
	it("answers the timeline a plan names, and the open one otherwise", () => {
		const owners = new AssetOwners();
		owners.note(
			window({
				layers: [
					{
						...base,
						item: "k/a",
						kind: "media",
						asset: "ast_nested",
						timeline: "prj_child",
					},
					{ ...base, item: "b", kind: "media", asset: "ast_own" },
				],
				audio: [
					{
						item: "k/c",
						asset: "ast_sound",
						timeline: "prj_child",
						first: 0,
						last: 9,
					},
				],
			} as unknown as Partial<PlanWindow>),
		);

		expect(owners.of("ast_nested", "prj_open")).toBe("prj_child");
		expect(owners.of("ast_sound", "prj_open")).toBe("prj_child");
		expect(owners.of("ast_own", "prj_open")).toBe("prj_open");
		expect(owners.of("ast_unknown", "prj_open")).toBe("prj_open");
	});
});

describe("layersAt", () => {
	it("leaves a group layer to the server", () => {
		const plan = window({
			layers: [
				{ ...base, item: "k", kind: "group" },
				{ ...base, item: "t", kind: "bitmap" },
			],
		} as unknown as Partial<PlanWindow>);

		expect(layersAt(plan, 3).map((l) => l.item)).toEqual(["t"]);
	});
});

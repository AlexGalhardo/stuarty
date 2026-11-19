import { describe, expect, test } from "vitest";

import type { Layer } from "../api/types";
import { slotsOf } from "./NewFromTemplate";

const layer = (extra: Partial<Layer> & { id: string }): Layer => ({
	kind: "text",
	name: extra.id,
	visible: true,
	opacity: 100,
	x: 0,
	y: 0,
	w: 10,
	h: 10,
	...extra,
});

describe("slotsOf", () => {
	test("a slot with no content is required and one with content is optional", () => {
		const slots = slotsOf([
			layer({ id: "t", slot: "title", text: "" }),
			layer({ id: "s", slot: "subtitle", text: "Hello" }),
			layer({ id: "p", kind: "image", slot: "photo" }),
			layer({ id: "q", kind: "image", slot: "logo", asset: "sha256:ab" }),
			layer({ id: "n", text: "no slot" }),
		]);
		expect(slots.map((s) => [s.name, s.kind, s.required])).toEqual([
			["title", "text", true],
			["subtitle", "text", false],
			["photo", "image", true],
			["logo", "image", false],
		]);
	});

	test("slots inside a group are found in layer order", () => {
		const slots = slotsOf([
			layer({
				id: "g",
				kind: "group",
				children: [layer({ id: "t", slot: "inner", text: "" })],
			}),
			layer({ id: "u", slot: "outer", text: "" }),
		]);
		expect(slots.map((s) => s.name)).toEqual(["inner", "outer"]);
	});
});

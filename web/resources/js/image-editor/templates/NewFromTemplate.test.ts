import { describe, expect, test } from "vitest";

import type { Finding, Layer } from "../api/types";
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

const emptySlot = (item: string): Finding => ({
	code: "L_IMAGE_EMPTY_SLOT",
	severity: "error",
	item,
	message: "The slot has no content.",
	hint: "",
});

describe("slotsOf", () => {
	test("a slot is required when the core's lint says it is empty, and optional otherwise", () => {
		const slots = slotsOf(
			[
				layer({ id: "t", slot: "title", text: "" }),
				layer({ id: "s", slot: "subtitle", text: "Hello" }),
				layer({ id: "p", kind: "image", slot: "photo" }),
				layer({ id: "q", kind: "image", slot: "logo", asset: "sha256:ab" }),
				layer({ id: "n", text: "no slot" }),
			],
			[
				emptySlot("t"),
				emptySlot("p"),
				{ ...emptySlot("s"), code: "L_IMAGE_TEXT_SMALL", severity: "warning" },
			],
		);
		expect(slots.map((s) => [s.name, s.kind, s.required])).toEqual([
			["title", "text", true],
			["subtitle", "text", false],
			["photo", "image", true],
			["logo", "image", false],
		]);
	});

	test("the screen does not decide on its own: an empty text with no finding is not marked required", () => {
		const slots = slotsOf([layer({ id: "t", slot: "title", text: "" })], []);
		expect(slots.map((s) => s.required)).toEqual([false]);
	});

	test("slots inside a group are found in layer order", () => {
		const slots = slotsOf(
			[
				layer({
					id: "g",
					kind: "group",
					children: [layer({ id: "t", slot: "inner", text: "" })],
				}),
				layer({ id: "u", slot: "outer", text: "" }),
			],
			[emptySlot("t"), emptySlot("u")],
		);
		expect(slots.map((s) => [s.name, s.required])).toEqual([
			["inner", true],
			["outer", true],
		]);
	});
});

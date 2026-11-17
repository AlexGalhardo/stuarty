import { describe, expect, test } from "vitest";

import { resetOf } from "./TransformSection";

describe("resetOf", () => {
	test("X and Y have none: the registry gives them no clearing value", () => {
		expect(resetOf("x", ["540"], false)).toBeNull();
		expect(resetOf("y", ["960"], false)).toBeNull();
	});

	test("uses the values the core documents as clearing the property", () => {
		expect(resetOf("scale", ["2"], false)).toEqual({
			value: 1,
			disabled: false,
		});
		expect(resetOf("rotation", ["15"], false)?.value).toBe(0);
		expect(resetOf("opacity", ["0.5"], false)?.value).toBe(1);
	});

	test("is off at the default for every item, and for a keyframed property", () => {
		expect(resetOf("scale", ["1", "1"], false)?.disabled).toBe(true);
		expect(resetOf("scale", ["1", "2"], false)?.disabled).toBe(false);
		expect(resetOf("scale", ["2"], true)?.disabled).toBe(true);
	});
});

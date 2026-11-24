import { describe, expect, test } from "vitest";

import type { MotionCheck } from "../api/types";
import {
	fieldKey,
	originOf,
	rateLabel,
	sizeLabel,
	verdictOf,
} from "./motionModel";

describe("motion panel wording", () => {
	test("reads a frame rate as a person does", () => {
		expect(rateLabel({ num: 30, den: 1 })).toBe("30");
		expect(rateLabel({ num: 30000, den: 1001 })).toBe("29.97");
		expect(rateLabel({ num: 30, den: 0 })).toBe("30/0");
		expect(sizeLabel(1080, 1920)).toBe("1080 x 1920");
	});

	test("names the five fields of the check and leaves an unknown one as the service said it", () => {
		const mismatch = { expected: "", found: "", hint: "" };
		for (const field of ["size", "frame_rate", "duration", "alpha", "kind"]) {
			expect(fieldKey({ field, ...mismatch })).toBe(
				`ui.editor.handoff.field.${field}`,
			);
		}
		expect(fieldKey({ field: "colour", ...mismatch })).toBeNull();
	});

	test("takes the verdict from the service, never from the mismatches", () => {
		const check = { ok: true, mismatches: [{ field: "size" }] } as MotionCheck;
		expect(verdictOf(check)).toBe("fits");
		expect(verdictOf({ ...check, ok: false })).toBe("mismatch");
	});

	test("leaves out the members of the origin that were left blank", () => {
		expect(
			originOf({
				source: " hyperframes/intro ",
				engine: "",
				engineVersion: " ",
				renderKey: "lower-third",
			}),
		).toEqual({
			kind: "rendered_externally",
			source: "hyperframes/intro",
			render_key: "lower-third",
		});
		expect(
			originOf({
				source: "x",
				engine: "HyperFrames",
				engineVersion: "1.0",
				renderKey: "",
			}),
		).toEqual({
			kind: "rendered_externally",
			source: "x",
			engine: "HyperFrames",
			engine_version: "1.0",
		});
	});
});

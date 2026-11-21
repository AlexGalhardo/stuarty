import { describe, expect, test } from "vitest";

import type { ProjectContextInput } from "../../editor/api/types";
import { emptyContext, validate } from "./projectSchema";

const valid = (): ProjectContextInput => ({
	name: "Launch teaser",
	description: "d".repeat(64),
	goal: {
		specific: "s".repeat(16),
		measurable: "m".repeat(16),
		achievable: "a".repeat(16),
		relevant: "r".repeat(16),
		time_bound: "t".repeat(16),
		due: "2999-12-31",
	},
});

const options = { today: "2026-11-20", checkDue: true };

describe("the schema built from the one the core exports", () => {
	test("accepts a valid context", () => {
		expect(validate(valid(), options)).toEqual({});
	});

	test("names every invalid field of an empty form", () => {
		expect(Object.keys(validate(emptyContext(), options))).toEqual([
			"name",
			"description",
			"goal.specific",
			"goal.measurable",
			"goal.achievable",
			"goal.relevant",
			"goal.time_bound",
			"goal.due",
		]);
	});

	test.each([
		["name", 3, "ui.projects.errors.too_short"],
		["name", 33, "ui.projects.errors.too_long"],
	])("the %s boundary at %i characters", (_field, length, key) => {
		const v = valid();
		v.name = "n".repeat(length);
		expect(validate(v, options).name?.key).toBe(key);
	});

	test("the name pattern: two spaces, a space at the end, punctuation", () => {
		for (const name of ["Launch  teaser", "Launch teaser ", "Launch, teaser"]) {
			const v = valid();
			v.name = name;
			expect(validate(v, options).name?.key).toBe(
				"ui.projects.errors.name_pattern",
			);
		}
		const v = valid();
		v.name = "Vídeo ação";
		expect(validate(v, options)).toEqual({});
	});

	test("description 63 and 1025 are refused, 64 and 1024 are not", () => {
		for (const [n, ok] of [
			[63, false],
			[64, true],
			[1024, true],
			[1025, false],
		] as const) {
			const v = valid();
			v.description = "d".repeat(n);
			expect("description" in validate(v, options)).toBe(!ok);
		}
	});

	test("a goal part: 15 and 281 are refused, 16 and 280 are not", () => {
		for (const [n, ok] of [
			[15, false],
			[16, true],
			[280, true],
			[281, false],
		] as const) {
			const v = valid();
			v.goal.specific = "s".repeat(n);
			expect("goal.specific" in validate(v, options)).toBe(!ok);
		}
	});

	test("the due date: a real date, and not before today when it is new", () => {
		const v = valid();
		v.goal.due = "2026-13-40";
		expect(validate(v, options)["goal.due"]?.key).toBe(
			"ui.projects.errors.date",
		);
		v.goal.due = "2026-11-19";
		expect(validate(v, options)["goal.due"]?.key).toBe(
			"ui.projects.errors.date_past",
		);
		expect(validate(v, { ...options, checkDue: false })).toEqual({});
	});
});

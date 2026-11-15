import { readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, test } from "vitest";

import { catalogFixtureProblems, RULES } from "./i18n-catalog";

const dir = join(dirname(fileURLToPath(import.meta.url)), "i18n-fixtures");
const allowed = new Set(["brand"]);
const cases = readdirSync(dir);

describe("check-i18n fixtures", () => {
	test("every rule has a failing and a passing fixture (I6)", () => {
		for (const rule of RULES) {
			expect(cases).toContain(`${rule.toLowerCase()}-fail`);
			expect(cases).toContain(`${rule.toLowerCase()}-pass`);
		}
	});

	for (const rule of ["I1", "I2", "I5", "I7"]) {
		const name = rule.toLowerCase();
		test(`${rule}: the failing fixture reports it`, () => {
			const rules = catalogFixtureProblems(join(dir, `${name}-fail`), allowed).map((p) => p.rule);
			expect(rules).toContain(rule);
		});
		test(`${rule}: the passing fixture reports nothing`, () => {
			expect(catalogFixtureProblems(join(dir, `${name}-pass`), allowed)).toEqual([]);
		});
	}
});

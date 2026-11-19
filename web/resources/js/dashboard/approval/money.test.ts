import { describe, expect, test } from "vitest";

import { formatMoney, microsToInput, parseMicros } from "./money";

describe("amounts of the approval queue are micros, formatted for a person", () => {
	test("formats in the locale and the currency", () => {
		expect(formatMoney({ micros: 1_500_000, currency: "USD" }, "en")).toBe(
			"$1.50",
		);
		expect(
			formatMoney({ micros: 1_500_000, currency: "BRL" }, "pt-BR"),
		).toMatch(/R\$\s?1,50/);
	});

	test("keeps the cents of a small amount", () => {
		expect(formatMoney({ micros: 40_000, currency: "USD" }, "en")).toBe(
			"$0.04",
		);
	});

	test("a currency Intl does not know is shown by its code", () => {
		expect(formatMoney({ micros: 2_000_000, currency: "ZZZ!" }, "en")).toBe(
			"2 ZZZ!",
		);
	});

	test("reads what a person typed without floating point", () => {
		expect(parseMicros("5")).toBe(5_000_000);
		expect(parseMicros("0.1")).toBe(100_000);
		expect(parseMicros("1,25")).toBe(1_250_000);
		expect(parseMicros(" 2.000001 ")).toBe(2_000_001);
		expect(parseMicros("0")).toBe(0);
	});

	test("refuses what is not an amount", () => {
		for (const bad of ["", "abc", "-1", "1.1234567", "1e3", "1.2.3", "$5"]) {
			expect(parseMicros(bad), bad).toBeNull();
		}
	});

	test("writes micros back as a form field", () => {
		expect(microsToInput(1_500_000)).toBe("1.5");
		expect(microsToInput(5_000_000)).toBe("5");
		expect(microsToInput(100_000)).toBe("0.1");
		expect(parseMicros(microsToInput(2_000_001))).toBe(2_000_001);
	});
});

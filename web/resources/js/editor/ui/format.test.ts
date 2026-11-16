import { describe, expect, test } from "vitest";

import { capitalizeFirst, formatDecimal } from "./format";

describe("number formats follow the locale", () => {
	test("pt-BR uses the decimal comma, en the point", () => {
		expect(formatDecimal(18.44, "en")).toBe("18.4");
		expect(formatDecimal(18.44, "pt-BR")).toBe("18,4");
	});
});

describe("capitalizeFirst", () => {
	test("a sentence that starts with a lower-case actor gets a capital", () => {
		expect(capitalizeFirst("alguém alterou o projeto")).toBe(
			"Alguém alterou o projeto",
		);
		expect(capitalizeFirst("")).toBe("");
	});
});

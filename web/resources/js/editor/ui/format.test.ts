import { describe, expect, test } from "vitest";

import { formatDecimal, formatSeconds } from "./format";

describe("number formats follow the locale", () => {
	test("pt-BR uses the decimal comma, en the point", () => {
		expect(formatDecimal(18.44, "en")).toBe("18.4");
		expect(formatDecimal(18.44, "pt-BR")).toBe("18,4");
		expect(formatSeconds(72.3, "en")).toBe("1:12.3");
		expect(formatSeconds(72.3, "pt-BR")).toBe("1:12,3");
	});
});

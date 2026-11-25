import { describe, expect, test } from "vitest";

import { localizeSize } from "./localizeSize";

describe("localizeSize", () => {
	test("uses the decimal comma of pt-BR", () => {
		expect(localizeSize("52.1 KB", "pt-BR")).toBe("52,1 KB");
		expect(localizeSize("1.2 GB", "pt-BR")).toBe("1,2 GB");
	});

	test("keeps the point in English and leaves whole numbers alone", () => {
		expect(localizeSize("52.1 KB", "en")).toBe("52.1 KB");
		expect(localizeSize("512 B", "pt-BR")).toBe("512 B");
	});

	test("an unknown language leaves the text as it is", () => {
		expect(localizeSize("52.1 KB", "not a locale")).toBe("52.1 KB");
	});
});

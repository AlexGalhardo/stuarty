import { describe, expect, test } from "vitest";

import { importedLabel } from "./FontsSection";

describe("the imported font picker label", () => {
	test("shows the start of the hash and the size in KB", () => {
		expect(
			importedLabel({ hash: `sha256:${"ab".repeat(32)}`, bytes: 20480 }),
		).toBe("sha256:abababababab… (20 KB)");
	});

	test("never shows 0 KB for a small file", () => {
		expect(importedLabel({ hash: `sha256:${"c".repeat(64)}`, bytes: 10 })).toBe(
			"sha256:cccccccccccc… (1 KB)",
		);
	});
});

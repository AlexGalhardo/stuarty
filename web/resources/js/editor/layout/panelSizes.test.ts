import { describe, expect, test } from "vitest";

import { panelSizes } from "./EditorShell";

describe("panelSizes", () => {
	test("the library is twice its former width from 1280 px up", () => {
		expect(panelSizes(1600).library).toBe(640);
		expect(panelSizes(1920).library).toBe(640);
		expect(panelSizes(1280).library).toBe(480);
		expect(panelSizes(1440).library).toBe(480);
	});

	test("the preview keeps its room on a narrow window", () => {
		for (const width of [1024, 1100, 1200]) {
			const sizes = panelSizes(width);
			expect(width - sizes.library - sizes.inspector).toBeGreaterThanOrEqual(
				360,
			);
		}
	});
});

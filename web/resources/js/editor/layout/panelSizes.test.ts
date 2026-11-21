import { describe, expect, test } from "vitest";

import { panelSizes } from "./EditorShell";

describe("panelSizes", () => {
	test("the library is twice its former width from 1280 px up", () => {
		expect(panelSizes(1600).library).toBe(640);
		expect(panelSizes(1920).library).toBe(640);
		expect(panelSizes(1280).library).toBe(480);
		expect(panelSizes(1440).library).toBe(480);
	});

	test("the history sidebar is a rail collapsed and 320 px open", () => {
		expect(panelSizes(1600).history).toBe(40);
		expect(panelSizes(1600, true).history).toBe(320);
	});

	test("the preview keeps its room, the library gives way to the history", () => {
		for (const open of [false, true]) {
			for (const width of [1024, 1100, 1200, 1280, 1600, 1920]) {
				const sizes = panelSizes(width, open);
				const preview = width - sizes.library - sizes.history - sizes.inspector;
				expect(preview).toBeGreaterThanOrEqual(360);
			}
		}
	});
});

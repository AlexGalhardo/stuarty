import { describe, expect, test } from "vitest";

import { panelSizes } from "./EditorShell";

describe("panelSizes", () => {
	test("the rail and the panel are 44 and 240 px and the inspector 340 px from 1280 px up (final design 05)", () => {
		for (const width of [1280, 1440, 1600, 1920]) {
			expect(panelSizes(width).library).toBe(284);
			expect(panelSizes(width).inspector).toBe(340);
		}
	});

	test("the edit by text screen gives the transcript the room the preview and the inspector spare", () => {
		expect(panelSizes(1440, false, true).library).toBe(740);
	});

	test("the history sidebar takes no room collapsed and 320 px open", () => {
		expect(panelSizes(1600).history).toBe(0);
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

	test("with the history open neither sidebar is a sliver at the narrowest width", () => {
		for (const width of [1024, 1100, 1200, 1279]) {
			const sizes = panelSizes(width, true);
			expect(sizes.library).toBeGreaterThanOrEqual(192);
			expect(sizes.history).toBeGreaterThanOrEqual(192);
		}
	});
});

import { describe, expect, test } from "vitest";

import { activeSection, isSectionKey, pathOf, SECTIONS } from "./sections";

describe("SECTIONS", () => {
	test("are the eleven sections of RF-311 in order", () => {
		expect(SECTIONS.map((s) => s.key)).toEqual([
			"projects",
			"edit-video",
			"edit-image",
			"write",
			"visual-assets",
			"video-assets",
			"publish",
			"skills",
			"prompts",
			"memory",
			"connections",
		]);
	});

	test("each has its own path and icon", () => {
		expect(new Set(SECTIONS.map((s) => s.path)).size).toBe(11);
		for (const s of SECTIONS) {
			expect(s.path).toBe(`/${s.key}`);
			expect(s.icon).toBeTruthy();
		}
	});
});

describe("activeSection", () => {
	test("is the section whose path is the url or holds it", () => {
		expect(activeSection("/projects")).toBe("projects");
		expect(activeSection("/projects/trash")).toBe("projects");
		expect(activeSection("/video-assets?page=2")).toBe("video-assets");
		expect(activeSection("/edit-video#top")).toBe("edit-video");
	});

	test("is not fooled by a longer name that starts the same", () => {
		expect(activeSection("/writer")).toBeNull();
		expect(activeSection("/projects-old")).toBeNull();
	});

	test("the home and settings belong to no section", () => {
		expect(activeSection("/")).toBeNull();
		expect(activeSection("/settings")).toBeNull();
		expect(activeSection("/settings/keys")).toBeNull();
	});
});

describe("pathOf and isSectionKey", () => {
	test("pathOf drops the query and the fragment", () => {
		expect(pathOf("/a/b?x=1#y")).toBe("/a/b");
		expect(pathOf("?x=1")).toBe("/");
	});

	test("isSectionKey narrows", () => {
		expect(isSectionKey("memory")).toBe(true);
		expect(isSectionKey("settings")).toBe(false);
		expect(isSectionKey(undefined)).toBe(false);
	});
});

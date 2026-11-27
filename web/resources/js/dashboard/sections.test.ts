import { describe, expect, test } from "vitest";

import {
	activeNavItem,
	activeSection,
	isSectionKey,
	NAV_GROUPS,
	pathOf,
	SECTION_KEYS,
	SECTIONS,
} from "./sections";

describe("SECTIONS", () => {
	test("list the dashboard sections in order", () => {
		expect(SECTIONS.map((s) => s.key)).toEqual([
			"projects",
			"edit-video",
			"edit-image",
			"write",
			"record",
			"visual-assets",
			"video-assets",
			"sound-assets",
			"publish",
			"skills",
			"prompts",
			"memory",
			"connections",
			"webhooks",
		]);
	});

	test("each has its own path and icon", () => {
		expect(new Set(SECTIONS.map((s) => s.path)).size).toBe(14);
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

describe("NAV_GROUPS", () => {
	test("hold every key of SECTION_KEYS once, in the five groups of the shell", () => {
		expect(NAV_GROUPS.map((g) => g.id)).toEqual([
			"create",
			"assets",
			"agent",
			"publish",
			"connect",
		]);
		const keys = NAV_GROUPS.flatMap((g) => g.items)
			.map((i) => i.section)
			.filter((k) => k !== undefined);
		expect([...keys].sort()).toEqual([...SECTION_KEYS].sort());
	});

	test("link only to routes that exist, each once", () => {
		const hrefs = NAV_GROUPS.flatMap((g) => g.items.map((i) => i.href));
		expect(new Set(hrefs).size).toBe(hrefs.length);
		expect(hrefs).toContain("/approvals");
		expect(hrefs).toContain("/settings/brand-kit");
		expect(hrefs).toContain("/record");
		expect(hrefs).not.toContain("/local-models");
	});

	test("activeNavItem is the longest path that holds the url", () => {
		expect(activeNavItem("/approvals")).toBe("approvals");
		expect(activeNavItem("/record")).toBe("record");
		expect(activeNavItem("/projects/prj_x")).toBe("projects");
		expect(activeNavItem("/settings/brand-kit")).toBe("brand-kit");
		expect(activeNavItem("/settings/keys")).toBe("keys");
		expect(activeNavItem("/settings")).toBeNull();
		expect(activeNavItem("/")).toBeNull();
	});
});

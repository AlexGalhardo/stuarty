import { describe, expect, test } from "vitest";

import { breadcrumbOf } from "./breadcrumb";

const labels = (url: string): string[] =>
	breadcrumbOf(url).map((crumb) => crumb.label);

describe("breadcrumbOf", () => {
	test("the home is the studio, then Home", () => {
		expect(labels("/")).toEqual([
			"ui.shell.breadcrumb.studio",
			"ui.nav.dashboard_home",
		]);
	});

	test("a section is the studio, then the section", () => {
		expect(labels("/publish")).toEqual([
			"ui.shell.breadcrumb.studio",
			"ui.nav.section.publish",
		]);
	});

	test("a page below an item belongs to the item", () => {
		expect(labels("/projects/prj_1234567890?tab=x")).toEqual([
			"ui.shell.breadcrumb.studio",
			"ui.nav.section.projects",
		]);
	});

	test("the links of the sidebar that are not sections have their own step", () => {
		expect(labels("/approvals")).toContain("ui.nav.approvals");
		expect(labels("/settings/brand-kit")).toContain("ui.nav.brand_kit");
	});

	test("settings pages without an item are Settings", () => {
		expect(labels("/settings/account")).toEqual([
			"ui.shell.breadcrumb.studio",
			"ui.nav.settings",
		]);
	});

	test("exactly one step is the current page", () => {
		for (const url of ["/", "/write", "/settings", "/nowhere"]) {
			expect(breadcrumbOf(url).filter((crumb) => crumb.current)).toHaveLength(
				1,
			);
		}
	});

	test("a path the shell does not know is only the studio, invented names are not drawn", () => {
		expect(breadcrumbOf("/nowhere")).toEqual([
			{ label: "ui.shell.breadcrumb.studio", href: "/", current: true },
		]);
	});
});

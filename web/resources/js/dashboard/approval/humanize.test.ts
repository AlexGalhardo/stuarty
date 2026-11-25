import { describe, expect, test } from "vitest";

import { headline, placeOf } from "./humanize";

const t = (key: string, params?: Record<string, string | number>): string =>
	`${key}|${Object.entries(params ?? {})
		.map(([k, v]) => `${k}=${v}`)
		.join(",")}`;

describe("placeOf", () => {
	const connection = (id: string) => (id === "con_a" ? "Blog" : null);
	const project = (id: string) => (id === "prj_a" ? "Course" : null);

	test("names the connection and the project", () => {
		expect(placeOf("con_a / prj_a", connection, project)).toBe("Blog / Course");
	});

	test("drops an id with no known name, so a raw id never shows", () => {
		expect(placeOf("con_x / prj_a", connection, project)).toBe("Course");
		expect(placeOf("con_x", connection, project)).toBe("");
		expect(placeOf("", connection, project)).toBe("");
	});
});

describe("headline", () => {
	test("a publication names the item and the place", () => {
		expect(headline(t, "publish.upload", "Post", "Blog")).toBe(
			"ui.approvals.headline.publish|item=“Post”,destination=Blog",
		);
	});

	test("a missing item or place reads as a generic word", () => {
		expect(headline(t, "publish.upload", undefined, null)).toContain(
			"ui.approvals.headline.this_item",
		);
	});

	test("a generation and an unknown kind have their own words", () => {
		expect(headline(t, "generate.image", undefined, "OpenRouter")).toContain(
			"headline.generate",
		);
		expect(headline(t, "other.thing", undefined, null)).toContain(
			"headline.other|kind=other.thing",
		);
	});
});

import { describe, expect, test } from "vitest";

import { createPlan, exampleKey } from "./kinds";

const pick = { textType: "post", format: "9:16" } as const;

describe("createPlan", () => {
	test("a text starts its first text with the type chosen and asks for no timeline", () => {
		const plan = createPlan(
			{ ...pick, choice: "text", textType: "script" },
			"My post",
		);
		expect(plan.kind).toBe("text");
		expect(plan.item).toEqual({
			kind: "text",
			title: "My post",
			text: "",
			text_kind: "script",
		});
		expect(JSON.stringify(plan)).not.toContain("preset");
	});

	test("a video starts its first timeline with the format chosen", () => {
		expect(
			createPlan({ ...pick, choice: "video", format: "1:1" }, "Reel"),
		).toEqual({
			kind: "video",
			item: { kind: "timeline", title: "Reel", preset: "1:1" },
		});
	});

	test("an image and a project for later create no item and no format", () => {
		expect(createPlan({ ...pick, choice: "image" }, "Cover")).toEqual({
			kind: "image",
		});
		expect(createPlan({ ...pick, choice: "undecided" }, "Maybe")).toEqual({});
	});
});

describe("exampleKey", () => {
	test("a text, a video and an image have examples of their own for the long fields", () => {
		expect(exampleKey("text", "description")).toBe(
			"ui.projects.example_kind.text.description",
		);
		expect(exampleKey("video", "specific")).toBe(
			"ui.projects.example_kind.video.specific",
		);
		expect(exampleKey("image", "achievable")).toBe(
			"ui.projects.example_kind.image.achievable",
		);
	});

	test("the other fields, and a project not decided yet, read the neutral example", () => {
		expect(exampleKey("text", "name")).toBe("ui.projects.example.name");
		expect(exampleKey("undecided", "description")).toBe(
			"ui.projects.example.description",
		);
		expect(exampleKey(undefined, "description")).toBe(
			"ui.projects.example.description",
		);
	});
});

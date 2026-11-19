import { describe, expect, test } from "vitest";

import { frameUrl } from "../api/imageApi";
import { frameWidth } from "./Frame";

describe("frameWidth", () => {
	test("asks for the canvas width itself when the screen needs at least that", () => {
		expect(frameWidth(1280, 1280)).toBe(0);
		expect(frameWidth(3000, 1280)).toBe(0);
	});

	test("asks for the smallest step that is as wide as the screen needs", () => {
		expect(frameWidth(300, 1280)).toBe(512);
		expect(frameWidth(512, 1280)).toBe(512);
		expect(frameWidth(1100, 1280)).toBe(0);
		expect(frameWidth(1000, 4096)).toBe(1024);
	});
});

describe("frameUrl", () => {
	test("names the document, the width and the revision as a cache key", () => {
		expect(frameUrl("img_abc", 7, 512)).toBe(
			"/api/v1/images/img_abc/frame?width=512&rev=7",
		);
	});
});

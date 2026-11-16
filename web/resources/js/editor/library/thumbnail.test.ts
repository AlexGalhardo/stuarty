import { describe, expect, test } from "vitest";

import { hasThumbnail } from "./thumbnail";

describe("hasThumbnail", () => {
	test("only a ready video or image has one", () => {
		expect(hasThumbnail({ kind: "video", state: "ready" })).toBe(true);
		expect(hasThumbnail({ kind: "image", state: "ready" })).toBe(true);
		expect(hasThumbnail({ kind: "audio", state: "ready" })).toBe(false);
		expect(hasThumbnail({ kind: "video", state: "ingesting" })).toBe(false);
	});
});

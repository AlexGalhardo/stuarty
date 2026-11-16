import { describe, expect, test } from "vitest";

import { formatSeconds, formatTime } from "./timeFormat";

const FPS = { num: 30, den: 1 };

describe("one time format for the whole editor", () => {
	test("a frame and an asset duration in seconds read the same", () => {
		expect(formatTime(375, FPS, "timecode")).toBe("00:12.500");
		expect(formatSeconds(12.5, "timecode")).toBe("00:12.500");
	});

	test("the mode switches every place together", () => {
		expect(formatTime(375, FPS, "frames")).toBe("375f");
		expect(formatTime(375, FPS, "seconds")).toBe("12.500s");
		expect(formatSeconds(12.5, "seconds")).toBe("12.500s");
	});
});

import { describe, expect, test } from "vitest";

import {
	displayDuration,
	displayEnd,
	displayInPoint,
	displayStart,
	formatSeconds,
	formatTime,
	localizeDecimalInput,
	normalizeTimeInput,
	speedText,
	timeToFrame,
} from "./units";

const ok = (value: string) => ({ ok: true, value });
const no = { ok: false };

describe("normalizeTimeInput", () => {
	test("rejects empty, words, four parts, negatives and spaces", () => {
		for (const raw of [
			"",
			"  ",
			"abc",
			"1:2:3:4",
			"-1",
			"-1s",
			"1 2",
			"12.5.1",
		])
			expect(normalizeTimeInput(raw, "s")).toEqual(no);
	});

	test("a bare number gets the default unit (T8)", () => {
		expect(normalizeTimeInput("12.5", "s")).toEqual(ok("12.5s"));
		expect(normalizeTimeInput("12", "f")).toEqual(ok("12f"));
		expect(normalizeTimeInput(" 3 ", "s")).toEqual(ok("3s"));
	});

	test("a decimal with the unit f is rejected, as SDD-012 12.2 says", () => {
		expect(normalizeTimeInput("12.5", "f")).toEqual(no);
		expect(normalizeTimeInput("12.5f", "s")).toEqual(no);
	});

	test("the three forms of SDD-002 A.4 pass through unchanged", () => {
		for (const raw of [
			"12.5s",
			"0.04s",
			"375f",
			"00:12.500",
			"01:02.5",
			"00:00:12.500",
		])
			expect(normalizeTimeInput(raw, "s")).toEqual(ok(raw));
	});

	test("timecode parts are bounded and the unit must be s or f", () => {
		expect(normalizeTimeInput("00:75.000", "s")).toEqual(no);
		expect(normalizeTimeInput("01:75:00", "s")).toEqual(no);
		expect(normalizeTimeInput("500ms", "s")).toEqual(no);
		expect(normalizeTimeInput("12.5S", "s")).toEqual(no);
	});
});

describe("localizeDecimalInput", () => {
	test("only pt-BR turns the first comma into a point", () => {
		expect(localizeDecimalInput("12,5", "pt-BR")).toBe("12.5");
		expect(localizeDecimalInput("00:12,500", "pt-BR")).toBe("00:12.500");
		expect(localizeDecimalInput("12,5", "en")).toBe("12,5");
		expect(normalizeTimeInput(localizeDecimalInput("12,5s", "pt-BR"))).toEqual(
			ok("12.5s"),
		);
		expect(normalizeTimeInput(localizeDecimalInput("12,5", "en"))).toEqual(no);
	});
});

describe("display", () => {
	const fps = { num: 30, den: 1 };
	test("formatTime in the three modes", () => {
		expect(formatTime(375, fps, "frames")).toBe("375f");
		expect(formatTime(375, fps, "seconds")).toBe("12.500s");
		expect(formatTime(375, fps, "timecode")).toBe("00:12.500");
		expect(formatTime(30 * 3725, fps, "timecode")).toBe("01:02:05.000");
		expect(formatTime(1, { num: 30000, den: 1001 }, "seconds")).toBe("0.033s");
	});

	test("formatSeconds never goes through frames", () => {
		expect(formatSeconds(12.5, "seconds")).toBe("12.500s");
		expect(formatSeconds(62.5, "timecode")).toBe("01:02.500");
		expect(formatSeconds(12.5, "frames")).toBe("12.500s");
	});
});

describe("item times", () => {
	const fps = { num: 30, den: 1 };
	const item = { start: 30, duration: 180, source_in: 36 };
	test("start, end, duration and in point", () => {
		expect(displayStart(item, fps, "timecode")).toBe("00:01.000");
		expect(displayEnd(item, fps, "timecode")).toBe("00:07.000");
		expect(displayDuration(item, fps, "seconds")).toBe("6.000s");
		expect(displayInPoint(item, fps, "frames")).toBe("36f");
		expect(displayInPoint({ start: 0, duration: 1 }, fps, "frames")).toBe("0f");
	});
});

describe("speedText", () => {
	test("shows the number of the field, one at normal speed", () => {
		expect(speedText(undefined)).toBe("1");
		expect(speedText({ num: 3, den: 2 })).toBe("1.5");
		expect(speedText({ num: 1, den: 3 })).toBe("0.333");
		expect(speedText({ num: 4, den: 1 })).toBe("4");
	});
});

describe("timeToFrame (VV-127)", () => {
	const fps = { num: 30, den: 1 };

	test("reads seconds, frames and timecodes", () => {
		expect(timeToFrame("67s", fps)).toBe(2010);
		expect(timeToFrame("67", fps)).toBe(2010);
		expect(timeToFrame("12.5s", fps)).toBe(375);
		expect(timeToFrame("90f", fps)).toBe(90);
		expect(timeToFrame("01:07.000", fps)).toBe(2010);
		expect(timeToFrame("1:00:00", fps)).toBe(108000);
	});

	test("rounds on a fractional rate and refuses what is not a time", () => {
		expect(timeToFrame("1s", { num: 30000, den: 1001 })).toBe(30);
		expect(timeToFrame("abc", fps)).toBeNull();
		expect(timeToFrame("", fps)).toBeNull();
	});
});

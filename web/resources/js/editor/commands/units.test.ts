import { describe, expect, test } from "vitest";

import {
	formatSeconds,
	formatTime,
	localizeDecimalInput,
	normalizeTimeInput,
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

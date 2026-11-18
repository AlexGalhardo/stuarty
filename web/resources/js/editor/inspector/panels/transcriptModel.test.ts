import { describe, expect, test } from "vitest";

import type { Track } from "@/generated/timeline";
import {
	defaultCaptionTrack,
	installRequest,
	NEW_TRACK,
	parseInstallCommand,
	unavailableFrom,
} from "./transcriptModel";

const track = (id: string, extra: Partial<Track> = {}): Track => ({
	id,
	kind: "visual",
	order: id,
	...extra,
});

describe("defaultCaptionTrack", () => {
	test("is the first visual track with a caption style", () => {
		const tracks = {
			a: track("a"),
			b: track("b", { caption_style: {} }),
			c: track("c", { caption_style: {} }),
		};
		expect(defaultCaptionTrack(["a", "b", "c"], tracks)).toBe("b");
	});

	test("is a new track when none has a caption style", () => {
		expect(defaultCaptionTrack(["a"], { a: track("a") })).toBe(NEW_TRACK);
		expect(defaultCaptionTrack([], {})).toBe(NEW_TRACK);
	});

	test("an audio track never qualifies", () => {
		const audio = track("a", { kind: "audio", caption_style: {} });
		expect(defaultCaptionTrack(["a"], { a: audio })).toBe(NEW_TRACK);
	});
});

describe("installRequest", () => {
	test("prefers the tool and model the service names", () => {
		const base = {
			code: "E_TRANSCRIBE_UNAVAILABLE",
			message: "m",
			missing: [],
		};
		expect(
			installRequest({
				...base,
				tool: "whisper",
				model: "base",
				installCommand: "vukem-video tools:install other --model x",
			}),
		).toEqual({ name: "whisper", model: "base" });
		expect(
			installRequest({
				...base,
				installCommand: "vukem-video tools:install whisper --model small",
			}),
		).toEqual({ name: "whisper", model: "small" });
	});
});

describe("parseInstallCommand", () => {
	test("reads the tool and the model", () => {
		expect(
			parseInstallCommand("vukem-video tools:install whisper --model small"),
		).toEqual({ name: "whisper", model: "small" });
		expect(parseInstallCommand("vukem-video tools:install whisper")).toEqual({
			name: "whisper",
		});
	});

	test("refuses any other text", () => {
		expect(parseInstallCommand(undefined)).toBeNull();
		expect(parseInstallCommand("rm -rf / --model x")).toBeNull();
		expect(
			parseInstallCommand("vukem-video tools:install whisper; echo hi"),
		).toBeNull();
	});
});

describe("unavailableFrom", () => {
	test("reads what is missing, its size and the command", () => {
		const out = unavailableFrom({
			code: "E_TRANSCRIBE_UNAVAILABLE",
			message: "needs a helper",
			hint: "install it",
			details: {
				missing: [{ name: "whisper-cli", url_host: "example.com", bytes: 10 }],
				download_bytes: 10,
				install_command: "vukem-video tools:install whisper",
			},
		});
		expect(out.missing).toEqual([
			{ name: "whisper-cli", host: "example.com", bytes: 10 },
		]);
		expect(out.downloadBytes).toBe(10);
		expect(out.installCommand).toBe("vukem-video tools:install whisper");
	});

	test("a code with no details has nothing to install", () => {
		const out = unavailableFrom({
			code: "E_TRANSCRIBE_UNAVAILABLE",
			message: "m",
		});
		expect(out.missing).toEqual([]);
		expect(out.installCommand).toBeUndefined();
	});
});

import { describe, expect, test } from "vitest";

import type { InstalledTool } from "../../editor/api/types";
import { installCommand, transcriptionState } from "./installState";

const tool = (extra: Partial<InstalledTool>): InstalledTool => ({
	name: "whisper",
	kind: "helper",
	version: "1",
	sha256: "x",
	path: "p",
	...extra,
});

describe("the transcription state of the settings page (VV-077)", () => {
	test("nothing installed", () => {
		expect(transcriptionState([])).toEqual({ helper: false, models: [] });
	});

	test("the helper and its models, sorted", () => {
		expect(
			transcriptionState([
				tool({}),
				tool({ kind: "model", model: "small" }),
				tool({ kind: "model", model: "base" }),
			]),
		).toEqual({ helper: true, models: ["base", "small"] });
	});

	test("the terminal command names the chosen model", () => {
		expect(installCommand("base")).toBe(
			"vukem-video tools:install whisper --model base",
		);
	});
});

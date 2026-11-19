import { describe, expect, test } from "vitest";

import registry from "@/generated/commands.json";
import {
	ANIMATION_PRESETS,
	BLEND_MODES,
	FILTERS,
	FITS,
	KEYFRAME_COLOUR_PROPERTIES,
	KEYFRAME_EASINGS,
	KEYFRAME_FRAMING_PROPERTIES,
	KEYFRAME_PROPERTIES,
	SHAPES,
	TRANSITION_KINDS,
	VOICE_PRESETS,
} from "./builders";

interface Schema {
	enum?: unknown[];
	properties?: Record<string, Schema>;
}

// The registry (generated from Go) is the single source of the enum values the
// editor copies in builders.ts. FRAME_RATES and SIZE_PRESETS have no enum in the
// registry (free values; the presets live in the UI), so they are not checked.
function registryEnum(command: string, arg: string): string[] {
	const entry = registry.find((c) => c.name === command);
	if (!entry) throw new Error(`command ${command} is not in the registry`);
	const schema = (entry.args_schema as unknown as Schema).properties?.[arg];
	if (!schema?.enum) throw new Error(`${command}.${arg} has no enum`);
	// A nullable argument lists null to mean "remove"; the lists hold real values.
	return schema.enum.filter((v): v is string => typeof v === "string");
}

const CASES: [string, readonly string[], string, string][] = [
	["KEYFRAME_EASINGS", KEYFRAME_EASINGS, "keyframe.set", "easing"],
	[
		"KEYFRAME_PROPERTIES",
		[
			...KEYFRAME_PROPERTIES,
			...KEYFRAME_COLOUR_PROPERTIES,
			...KEYFRAME_FRAMING_PROPERTIES,
		],
		"keyframe.set",
		"property",
	],
	["TRANSITION_KINDS", TRANSITION_KINDS, "item.set_transition", "kind"],
	["ANIMATION_PRESETS", ANIMATION_PRESETS, "text.apply_preset", "preset"],
	["FILTERS", FILTERS, "clip.set_filter", "filter"],
	["FITS", FITS, "clip.set_framing", "fit"],
	["SHAPES", SHAPES, "shape.add", "shape"],
	["BLEND_MODES", BLEND_MODES, "clip.set_blend", "mode"],
	["VOICE_PRESETS", VOICE_PRESETS, "item.set_audio", "voice"],
];

describe("editor enum lists match the command registry", () => {
	test.each(CASES)("%s", (_name, list, command, arg) => {
		expect(new Set(list)).toEqual(new Set(registryEnum(command, arg)));
	});
});

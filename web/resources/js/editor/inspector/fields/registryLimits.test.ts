import { describe, expect, test } from "vitest";

import { registryLimits, registryRange } from "./registryLimits";

describe("registryLimits", () => {
	test("reads the minimum and maximum of an argument from the registry", () => {
		expect(registryLimits("item.set_audio", "volume")).toEqual({
			min: 0,
			max: 4,
		});
		expect(registryLimits("item.set_transform", "opacity")).toEqual({
			min: 0,
			max: 1,
		});
		expect(registryLimits("clip.set_framing", "crop", "left")).toEqual({
			min: 0,
			max: 1,
		});
		expect(registryLimits("clip.set_color", "brightness")).toEqual({
			min: -1,
			max: 1,
		});
	});

	test("an argument with one limit, no limit or an unknown command has the rest open", () => {
		expect(registryLimits("item.set_transform", "scale")).toEqual({ max: 20 });
		expect(registryLimits("item.set_transform", "x")).toEqual({});
		expect(registryLimits("no.such_command", "x")).toEqual({});
	});
});

describe("registryRange", () => {
	test("the sliders of the inspector have both limits in the registry", () => {
		for (const [command, arg] of [
			["clip.set_filter", "amount"],
			["clip.set_blur", "radius"],
			["clip.set_color", "brightness"],
			["clip.set_color", "contrast"],
			["clip.set_color", "saturation"],
		] as const) {
			expect(registryRange(command, arg), `${command} ${arg}`).toBeDefined();
		}
		expect(() => registryRange("item.set_transform", "x")).toThrow();
	});
});

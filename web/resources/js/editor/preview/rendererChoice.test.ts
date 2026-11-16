import { describe, expect, test } from "vitest";

import { chooseRenderer } from "./rendererChoice";

describe("chooseRenderer", () => {
	test("uses WebGPU when the browser gives an adapter", async () => {
		const gpu = { requestAdapter: async () => ({}) };
		expect(await chooseRenderer(null, gpu)).toBe("webgpu");
	});

	test("falls back to WebGL without navigator.gpu, without an adapter or on an error", async () => {
		expect(await chooseRenderer(null, undefined)).toBe("webgl");
		expect(
			await chooseRenderer(null, { requestAdapter: async () => null }),
		).toBe("webgl");
		const failing = {
			requestAdapter: async () => {
				throw new Error("blocked");
			},
		};
		expect(await chooseRenderer(null, failing)).toBe("webgl");
	});

	test("a forced renderer wins over the detection and an unknown value is ignored", async () => {
		const gpu = { requestAdapter: async () => ({}) };
		expect(await chooseRenderer("webgl", gpu)).toBe("webgl");
		expect(await chooseRenderer("webgpu", undefined)).toBe("webgpu");
		expect(await chooseRenderer("metal", undefined)).toBe("webgl");
	});
});

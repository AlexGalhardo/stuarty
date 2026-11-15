import { describe, expect, test } from "vitest";

import { exportCapabilities } from "./exportCapabilities";

function doc(args: unknown): unknown {
	return {
		paths: {
			"/api/v1/projects/{id}/jobs": {
				post: {
					requestBody: {
						content: {
							"application/json": {
								schema: { $ref: "#/components/schemas/jobStartIn" },
							},
						},
					},
				},
			},
		},
		components: {
			schemas: {
				jobStartIn: { type: "object", properties: { args } },
			},
		},
	};
}

describe("exportCapabilities", () => {
	test("reads the enums of the args members", () => {
		const caps = exportCapabilities(
			doc({
				type: "object",
				properties: {
					format: { enum: ["mp4", "gif"] },
					fps: { enum: [30, 60] },
					size: { enum: ["1080x1920", "2160x3840"] },
				},
			}),
		);
		expect(caps).toEqual({
			formats: ["mp4", "gif"],
			fps: [30, 60],
			sizes: ["1080x1920", "2160x3840"],
		});
	});

	test("a member without an enum is offered with the defaults", () => {
		const caps = exportCapabilities(
			doc({
				type: "object",
				properties: { format: { type: "string" }, fps: { type: "integer" } },
			}),
		);
		expect(caps.formats).toEqual(["mp4", "gif", "wav", "aac"]);
		expect(caps.fps).toEqual([60]);
		expect(caps.sizes).toEqual([]);
	});

	test("a free args map offers nothing", () => {
		expect(
			exportCapabilities(doc({ type: "object", additionalProperties: true })),
		).toEqual({ formats: [], fps: [], sizes: [] });
	});

	test("anything that is not an OpenAPI document offers nothing", () => {
		for (const value of [null, 3, "x", {}, { paths: [] }]) {
			expect(exportCapabilities(value).formats).toEqual([]);
		}
	});
});

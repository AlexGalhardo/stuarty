import { describe, expect, test } from "vitest";

import registry from "@/generated/commands.json";
import {
	BACKGROUND_KINDS,
	backgroundOperations,
	CAMERA_SHAPES,
	CORNERS,
	LAYOUTS,
	layoutOperations,
} from "./builders";

type Registered = {
	name: string;
	args_schema: { properties: Record<string, { enum?: string[] }> };
};

const schemaOf = (name: string): Registered["args_schema"]["properties"] => {
	const found = (registry as unknown as Registered[]).find(
		(c) => c.name === name,
	);
	if (!found) throw new Error(`no command ${name}`);
	return found.args_schema.properties;
};

describe("the layout and the background of a recording (SDD-002 C.96 and C.97)", () => {
	test("the choices of the inspector are the enums of the commands", () => {
		const layout = schemaOf("recording.apply_layout");
		expect([...LAYOUTS]).toEqual(layout.layout?.enum);
		expect([...CORNERS]).toEqual(layout.corner?.enum);
		expect([...CAMERA_SHAPES]).toEqual(layout.camera_shape?.enum);
		expect([...BACKGROUND_KINDS].sort()).toEqual(
			[...(schemaOf("recording.set_background").kind?.enum ?? [])].sort(),
		);
	});

	test("a layout is one operation with the options it applies to", () => {
		expect(
			layoutOperations("itm_s", {
				layout: "pip",
				cameraItemId: "itm_c",
				corner: "top_left",
				cameraShape: "rect",
				paddingPercent: 5,
			}),
		).toEqual([
			{
				op: "recording.apply_layout",
				args: {
					layout: "pip",
					screen_item_id: "itm_s",
					camera_item_id: "itm_c",
					corner: "top_left",
					camera_shape: "rect",
					padding: 0.05,
				},
			},
		]);
	});

	test("an option that does not apply to the layout is left out", () => {
		const only = layoutOperations("itm_s", {
			layout: "screen_only",
			cameraItemId: "itm_c",
			corner: "top_left",
			cameraShape: "circle",
		});
		expect(only[0]?.args).toEqual({
			layout: "screen_only",
			screen_item_id: "itm_s",
		});

		const split = layoutOperations("itm_s", {
			layout: "split",
			cameraItemId: "itm_c",
			corner: "top_left",
			cameraShape: "circle",
		});
		expect(split[0]?.args).toEqual({
			layout: "split",
			screen_item_id: "itm_s",
			camera_item_id: "itm_c",
		});

		const focus = layoutOperations("itm_s", {
			layout: "camera_focus",
			cameraItemId: "itm_c",
			corner: "top_right",
			cameraShape: "circle",
		});
		expect(focus[0]?.args).toMatchObject({ corner: "top_right" });
		expect(focus[0]?.args).not.toHaveProperty("camera_shape");
	});

	test("a background sends the colour or the asset of its kind only", () => {
		expect(
			backgroundOperations("itm_s", {
				kind: "color",
				color: "#112233",
				assetId: "ast_x",
			})[0]?.args,
		).toEqual({ item_id: "itm_s", kind: "color", color: "#112233" });
		expect(
			backgroundOperations("itm_s", {
				kind: "image",
				color: "#112233",
				assetId: "ast_x",
			})[0]?.args,
		).toEqual({ item_id: "itm_s", kind: "image", asset_id: "ast_x" });
		expect(
			backgroundOperations("itm_s", { kind: "none", color: "#112233" })[0]
				?.args,
		).toEqual({ item_id: "itm_s", kind: "none" });
	});
});

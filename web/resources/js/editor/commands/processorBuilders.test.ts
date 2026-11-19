import { describe, expect, test } from "vitest";

import {
	applyPresetOperations,
	processorAddOperations,
	processorMoveOperations,
	processorRemoveOperations,
	processorSetOperations,
} from "./processorBuilders";

describe("audio processor operations", () => {
	test("an item and a track name their own member", () => {
		expect(processorAddOperations({ itemId: "itm_a" }, "gate")).toEqual([
			{ op: "audio.processor_add", args: { item_id: "itm_a", kind: "gate" } },
		]);
		expect(processorAddOperations({ trackId: "trk_a" }, "limiter")).toEqual([
			{
				op: "audio.processor_add",
				args: { track_id: "trk_a", kind: "limiter" },
			},
		]);
	});

	test("a set sends only what changed", () => {
		expect(
			processorSetOperations({ itemId: "itm_a" }, 2, {
				params: { ratio: 4 },
			})[0]?.args,
		).toEqual({ item_id: "itm_a", index: 2, params: { ratio: 4 } });
		expect(
			processorSetOperations({ itemId: "itm_a" }, 0, { enabled: false })[0]
				?.args,
		).toEqual({ item_id: "itm_a", index: 0, enabled: false });
	});

	test("move, remove and preset", () => {
		expect(processorMoveOperations({ trackId: "trk_a" }, 1, 0)[0]).toEqual({
			op: "audio.processor_move",
			args: { track_id: "trk_a", index: 1, to: 0 },
		});
		expect(processorRemoveOperations({ itemId: "itm_a" }, 3)[0]).toEqual({
			op: "audio.processor_remove",
			args: { item_id: "itm_a", index: 3 },
		});
		expect(applyPresetOperations({ itemId: "itm_a" }, "voice")[0]).toEqual({
			op: "audio.apply_preset",
			args: { item_id: "itm_a", preset: "voice" },
		});
	});
});

import { describe, expect, test } from "vitest";

import { createUiStore } from "./uiStore";

const ref = { item: "itm_a", property: "scale", t: 30 };

describe("keyframe selection", () => {
	test("selecting a keyframe selects its item too", () => {
		const { store, writer } = createUiStore();
		writer.selectKeyframe(ref);
		expect(store.getState().selection).toEqual({
			items: ["itm_a"],
			track: null,
			anchor: "itm_a",
			keyframe: ref,
		});
	});

	test("selecting items or a track drops the keyframe", () => {
		const { store, writer } = createUiStore();
		writer.selectKeyframe(ref);
		writer.select(["itm_a"]);
		expect(store.getState().selection.keyframe).toBeNull();
		writer.selectKeyframe(ref);
		writer.selectTrack("trk_a");
		expect(store.getState().selection.keyframe).toBeNull();
	});

	test("null clears only the keyframe", () => {
		const { store, writer } = createUiStore();
		writer.selectKeyframe(ref);
		writer.selectKeyframe(null);
		expect(store.getState().selection.items).toEqual(["itm_a"]);
		expect(store.getState().selection.keyframe).toBeNull();
	});

	test("an item the read model removed takes its keyframe with it", () => {
		const { store, writer } = createUiStore();
		writer.selectKeyframe(ref);
		writer.dropSelection(() => false);
		expect(store.getState().selection.keyframe).toBeNull();
		writer.selectKeyframe(ref);
		writer.dropSelection(() => true);
		expect(store.getState().selection.keyframe).toEqual(ref);
	});
});

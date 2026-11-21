import { describe, expect, test, vi } from "vitest";

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

describe("row heights", () => {
	test("are clamped to the bounds and written to vv.layout", () => {
		const saved = new Map<string, string>();
		vi.stubGlobal("localStorage", {
			getItem: (key: string) => saved.get(key) ?? null,
			setItem: (key: string, value: string) => void saved.set(key, value),
		});
		const { store, writer } = createUiStore();
		expect(store.getState().rowPx).toEqual({ visual: 64, audio: 48 });
		writer.setRowPx("visual", 5000);
		writer.setRowPx("audio", 1);
		expect(store.getState().rowPx).toEqual({ visual: 160, audio: 32 });
		expect(JSON.parse(saved.get("vv.layout") ?? "null")).toEqual({
			rows: { visual: 160, audio: 32 },
			historyOpen: false,
		});
		expect(createUiStore().store.getState().rowPx).toEqual({
			visual: 160,
			audio: 32,
		});
		vi.unstubAllGlobals();
	});
});

describe("preview audio and size", () => {
	test("volume stays between 0 and 1 and mute toggles", () => {
		const { store, writer } = createUiStore();
		writer.setPreviewVolume(3);
		expect(store.getState().previewVolume).toBe(1);
		writer.setPreviewVolume(-1);
		expect(store.getState().previewVolume).toBe(0);
		writer.togglePreviewMuted();
		expect(store.getState().previewMuted).toBe(true);
		writer.setPreviewSize("50");
		expect(store.getState().previewSize).toBe("50");
	});
});

describe("in and out points", () => {
	test("an in point after the out point drops the out point, and the reverse", () => {
		const { store, writer } = createUiStore();
		writer.setRangeIn(10);
		writer.setRangeOut(50);
		expect(store.getState().range).toEqual({ in: 10, out: 50 });
		writer.setRangeIn(60);
		expect(store.getState().range).toEqual({ in: 60, out: null });
		writer.setRangeOut(20);
		expect(store.getState().range).toEqual({ in: null, out: 20 });
	});

	test("clearing leaves no points", () => {
		const { store, writer } = createUiStore();
		writer.setRangeIn(10);
		writer.clearRange();
		expect(store.getState().range).toEqual({ in: null, out: null });
	});
});

describe("the history sidebar", () => {
	test("toggles and keeps the library region apart", () => {
		vi.stubGlobal("localStorage", {
			getItem: () => null,
			setItem: () => undefined,
		});
		const { store, writer } = createUiStore();
		expect(store.getState().panels.history).toBe(false);
		writer.toggleHistory();
		expect(store.getState().panels).toEqual({
			left: true,
			inspector: true,
			history: true,
		});
		writer.toggleLeftRegion();
		expect(store.getState().panels.left).toBe(false);
		expect(store.getState().panels.history).toBe(true);
		writer.toggleHistory();
		expect(store.getState().panels.history).toBe(false);
	});
});

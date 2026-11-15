import { afterEach, describe, expect, it, vi } from "vitest";
import {
	canDropPaths,
	canPickFiles,
	cleanPaths,
	DESKTOP_READY_EVENT,
	getDesktopBridge,
	subscribeDesktopBridge,
} from "./desktopBridge";

afterEach(() => {
	vi.unstubAllGlobals();
});

describe("desktopBridge", () => {
	it("is absent in a plain browser", () => {
		vi.stubGlobal("window", new EventTarget());
		const bridge = getDesktopBridge();
		expect(bridge).toBeNull();
		expect(canPickFiles(bridge)).toBe(false);
		expect(canDropPaths(bridge)).toBe(false);
	});

	it("is absent when there is no window", () => {
		vi.stubGlobal("window", undefined);
		expect(getDesktopBridge()).toBeNull();
	});

	it("detects each member on its own", () => {
		const window = Object.assign(new EventTarget(), {
			vukemDesktop: { pickFiles: async () => [] },
		});
		vi.stubGlobal("window", window);
		const bridge = getDesktopBridge();
		expect(canPickFiles(bridge)).toBe(true);
		expect(canDropPaths(bridge)).toBe(false);
	});

	it("ignores a value that is not an object", () => {
		vi.stubGlobal(
			"window",
			Object.assign(new EventTarget(), { vukemDesktop: 1 }),
		);
		expect(getDesktopBridge()).toBeNull();
	});

	it("keeps only non-empty string paths", () => {
		expect(cleanPaths(["/a.mp4", "", "  ", 3, null, "C:\b.mov"])).toEqual([
			"/a.mp4",
			"C:\b.mov",
		]);
		expect(cleanPaths(undefined)).toEqual([]);
		expect(cleanPaths("/a.mp4")).toEqual([]);
	});

	it("tells a subscriber when the shell defines the bridge, until it leaves", () => {
		const target = new EventTarget();
		vi.stubGlobal("window", target);
		const listener = vi.fn();
		const off = subscribeDesktopBridge(listener);
		target.dispatchEvent(new Event(DESKTOP_READY_EVENT));
		expect(listener).toHaveBeenCalledTimes(1);
		off();
		target.dispatchEvent(new Event(DESKTOP_READY_EVENT));
		expect(listener).toHaveBeenCalledTimes(1);
	});
});

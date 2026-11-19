import { afterEach, describe, expect, it, vi } from "vitest";
import {
	canDropPaths,
	canPickFiles,
	canPickTemplate,
	canShowInFolder,
	cleanPaths,
	DESKTOP_READY_EVENT,
	getDesktopBridge,
	showInFolder,
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

	it("shows a file in its folder only through a bridge that offers it", async () => {
		const calls: string[] = [];
		const bridge = {
			showInFolder: async (path: string) => {
				calls.push(path);
				return [];
			},
		};
		expect(canShowInFolder(bridge)).toBe(true);
		expect(canShowInFolder({ pickFiles: async () => [] })).toBe(false);
		expect(canShowInFolder(null)).toBe(false);
		await showInFolder(bridge, "/out/a.mp4");
		await showInFolder(null, "/out/b.mp4");
		expect(calls).toEqual(["/out/a.mp4"]);
	});

	it("detects the template picker on its own", () => {
		expect(canPickTemplate({ pickTemplate: async () => [] })).toBe(true);
		expect(canPickTemplate({ pickFiles: async () => [] })).toBe(false);
		expect(canPickTemplate(null)).toBe(false);
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

import { afterEach, describe, expect, it, vi } from "vitest";
import {
	canBackground,
	canDropPaths,
	canPickFiles,
	canPickTemplate,
	canRevealAsset,
	canShowInFolder,
	cleanPaths,
	DESKTOP_READY_EVENT,
	getDesktopBridge,
	parseBackground,
	readBackground,
	revealAsset,
	showInFolder,
	subscribeDesktopBridge,
	writeBackground,
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

	it("reveals a library file only through a bridge that offers revealAsset, and never through showInFolder", async () => {
		const calls: string[] = [];
		const shown: string[] = [];
		const bridge = {
			revealAsset: async (path: string) => {
				calls.push(path);
				return [];
			},
			showInFolder: async (path: string) => {
				shown.push(path);
				return [];
			},
		};
		expect(canRevealAsset(bridge)).toBe(true);
		expect(canRevealAsset({ showInFolder: async () => [] })).toBe(false);
		expect(canRevealAsset(null)).toBe(false);
		await revealAsset(bridge, "/ws/store/aa/aabb");
		await revealAsset(null, "/ws/store/aa/ccdd");
		expect(calls).toEqual(["/ws/store/aa/aabb"]);
		expect(shown).toEqual([]);
	});

	it("passes the refusal of the shell on, so the page can say it", async () => {
		const bridge = {
			revealAsset: async () => {
				throw new Error("no file manager was found on this computer");
			},
		};
		await expect(revealAsset(bridge, "/ws/store/aa/aabb")).rejects.toThrow(
			"no file manager",
		);
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

describe("the background mode of the shell", () => {
	const held = { keep_running: true, start_at_sign_in: false, asked: true };

	it("is offered only when the shell has both members", () => {
		expect(canBackground(null)).toBe(false);
		expect(canBackground({ getBackground: async () => held })).toBe(false);
		expect(
			canBackground({
				getBackground: async () => held,
				setBackground: async () => held,
			}),
		).toBe(true);
	});

	it("takes only what the shell sends as settings", () => {
		expect(parseBackground(held)).toEqual(held);
		expect(parseBackground({ ...held, error: "denied" })).toEqual({
			...held,
			error: "denied",
		});
		expect(parseBackground({ ...held, error: "" })).toEqual(held);
		expect(parseBackground({ keep_running: true })).toBeNull();
		expect(parseBackground({ ...held, asked: "yes" })).toBeNull();
		expect(parseBackground(null)).toBeNull();
		expect(parseBackground("on")).toBeNull();
	});

	it("reads and writes through the bridge, and does nothing without one", async () => {
		const sent: unknown[] = [];
		const bridge = {
			getBackground: async () => held,
			setBackground: async (s: unknown) => {
				sent.push(s);
				return s;
			},
		};
		expect(await readBackground(bridge)).toEqual(held);
		expect(await writeBackground(bridge, held)).toEqual(held);
		expect(sent).toEqual([held]);
		expect(await readBackground(null)).toBeNull();
		expect(await writeBackground({}, held)).toBeNull();
	});
});

import { describe, expect, test, vi } from "vitest";

import { LOCK_WATCH_MS, type LockWatchDeps, startLockWatch } from "./lockWatch";

function harness(state: string | null, visible = true) {
	let tick: () => void = () => {};
	let show: () => void = () => {};
	const onLocked = vi.fn();
	const stopTimer = vi.fn();
	const readState = vi.fn(async () => state);
	const deps: LockWatchDeps = {
		readState,
		onLocked,
		isVisible: () => visible,
		repeat: (fn, ms) => {
			expect(ms).toBe(LOCK_WATCH_MS);
			tick = fn;
			return stopTimer;
		},
		onVisible: (fn) => {
			show = fn;
			return () => {};
		},
	};
	const stop = startLockWatch(deps);
	const settle = async (fn: () => void): Promise<void> => {
		fn();
		await Promise.resolve();
		await Promise.resolve();
	};
	return {
		onLocked,
		readState,
		stopTimer,
		stop,
		tick: () => settle(tick),
		show: () => settle(show),
	};
}

describe("the watch of the idle lock", () => {
	test("a page leaves for the sign-in once the studio is locked, and only once", async () => {
		const h = harness("locked");
		await h.tick();
		await h.tick();
		expect(h.onLocked).toHaveBeenCalledTimes(1);
	});

	test("an unlocked studio and a server that does not answer leave the page alone", async () => {
		for (const state of ["unlocked", "none", null]) {
			const h = harness(state);
			await h.tick();
			expect(h.onLocked).not.toHaveBeenCalled();
		}
	});

	test("a hidden tab asks nothing", async () => {
		const h = harness("locked", false);
		await h.tick();
		expect(h.readState).not.toHaveBeenCalled();
		expect(h.onLocked).not.toHaveBeenCalled();
	});

	test("coming back to the tab checks at once", async () => {
		const h = harness("locked");
		await h.show();
		expect(h.onLocked).toHaveBeenCalledTimes(1);
	});

	test("after the page closes nothing runs", async () => {
		const h = harness("locked");
		h.stop();
		expect(h.stopTimer).toHaveBeenCalledTimes(1);
		await h.tick();
		expect(h.readState).not.toHaveBeenCalled();
		expect(h.onLocked).not.toHaveBeenCalled();
	});
});

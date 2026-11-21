import { afterEach, describe, expect, test, vi } from "vitest";

import {
	readLayout,
	readLocale,
	readTimeDisplay,
	readTimeUnit,
	writeLayout,
	writeLocale,
	writeTimeUnit,
} from "./prefs";

function storage(initial: Record<string, string> = {}) {
	const data = new Map(Object.entries(initial));
	return {
		data,
		getItem: (key: string) => data.get(key) ?? null,
		setItem: (key: string, value: string) => void data.set(key, value),
	};
}

afterEach(() => vi.unstubAllGlobals());

describe("time unit preference", () => {
	test("is seconds until the person chooses frames", () => {
		vi.stubGlobal("localStorage", storage());
		expect(readTimeUnit()).toBe("s");
		writeTimeUnit("f");
		expect(readTimeUnit()).toBe("f");
	});

	test("the time display defaults to timecode and reads the stored mode", () => {
		vi.stubGlobal("localStorage", storage({}));
		expect(readTimeDisplay()).toBe("timecode");
		vi.stubGlobal("localStorage", storage({ "vv.timeDisplay": "frames" }));
		expect(readTimeDisplay()).toBe("frames");
		vi.stubGlobal("localStorage", storage({ "vv.timeDisplay": "x" }));
		expect(readTimeDisplay()).toBe("timecode");
	});

	test("an unknown stored value reads as seconds", () => {
		vi.stubGlobal("localStorage", storage({ "vv.timeUnit": "x" }));
		expect(readTimeUnit()).toBe("s");
	});

	test("a blocked storage leaves seconds and does not throw", () => {
		vi.stubGlobal("localStorage", {
			getItem: () => {
				throw new Error("blocked");
			},
			setItem: () => {
				throw new Error("blocked");
			},
		});
		expect(readTimeUnit()).toBe("s");
		expect(() => writeTimeUnit("f")).not.toThrow();
	});
});

describe("layout preference", () => {
	test("reads the defaults, then what was written", () => {
		vi.stubGlobal("localStorage", storage());
		expect(readLayout().rows).toEqual({ visual: 64, audio: 48 });
		writeLayout({ rows: { visual: 96, audio: 40 } });
		expect(readLayout().rows).toEqual({ visual: 96, audio: 40 });
	});

	test("a value outside the bounds or of another type reads as the default", () => {
		vi.stubGlobal(
			"localStorage",
			storage({ "vv.layout": '{"rows":{"visual":9999,"audio":"x"}}' }),
		);
		expect(readLayout().rows).toEqual({ visual: 64, audio: 48 });
	});

	test("a blocked storage leaves the defaults and does not throw", () => {
		vi.stubGlobal("localStorage", {
			getItem: () => {
				throw new Error("blocked");
			},
			setItem: () => {
				throw new Error("blocked");
			},
		});
		expect(readLayout().rows).toEqual({ visual: 64, audio: 48 });
		expect(() =>
			writeLayout({ rows: { visual: 70, audio: 50 } }),
		).not.toThrow();
	});
});

describe("language preference", () => {
	test("is none until chosen, then the stored tag", () => {
		vi.stubGlobal("localStorage", storage());
		expect(readLocale()).toBeNull();
		writeLocale("pt-BR");
		expect(readLocale()).toBe("pt-BR");
	});

	test("a stored value that is not a language tag reads as none", () => {
		vi.stubGlobal("localStorage", storage({ "vv.lang": "<x>" }));
		expect(readLocale()).toBeNull();
	});
});

describe("history sidebar preference", () => {
	test("is closed until opened, then reads what was written, beside the rows", () => {
		vi.stubGlobal("localStorage", storage());
		expect(readLayout().historyOpen).toBe(false);
		writeLayout({ historyOpen: true });
		expect(readLayout().historyOpen).toBe(true);
		expect(readLayout().rows).toEqual({ visual: 64, audio: 48 });
		writeLayout({ rows: { visual: 70, audio: 50 } });
		expect(readLayout()).toEqual({
			rows: { visual: 70, audio: 50 },
			historyOpen: true,
		});
		writeLayout({ historyOpen: false });
		expect(readLayout().historyOpen).toBe(false);
	});

	test("anything but true reads as closed, and blocked storage does not throw", () => {
		vi.stubGlobal(
			"localStorage",
			storage({ "vv.layout": JSON.stringify({ historyOpen: "yes" }) }),
		);
		expect(readLayout().historyOpen).toBe(false);
		vi.stubGlobal("localStorage", {
			getItem: () => {
				throw new Error("blocked");
			},
			setItem: () => {
				throw new Error("blocked");
			},
		});
		expect(readLayout().historyOpen).toBe(false);
		expect(() => writeLayout({ historyOpen: true })).not.toThrow();
	});
});

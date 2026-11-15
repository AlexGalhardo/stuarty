import { afterEach, describe, expect, test, vi } from "vitest";

import { readLayout, readTimeUnit, writeLayout, writeTimeUnit } from "./prefs";

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

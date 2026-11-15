import { afterEach, describe, expect, test, vi } from "vitest";

import { readTimeUnit, writeTimeUnit } from "./prefs";

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

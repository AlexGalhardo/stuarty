import { describe, expect, test } from "vitest";

import {
	cleanPrefs,
	DEFAULT_PREFS,
	loadPrefs,
	PREFS_KEY,
	savePrefs,
} from "./prefs";

function storage(initial: Record<string, string> = {}) {
	const data = { ...initial };
	return {
		data,
		getItem: (key: string) => data[key] ?? null,
		setItem: (key: string, value: string) => {
			data[key] = value;
		},
	};
}

describe("the preferences of the teleprompter", () => {
	test("start from the defaults of the specification", () => {
		expect(loadPrefs(storage())).toEqual({
			speed: 60,
			size: 48,
			mirror: false,
		});
		expect(loadPrefs(null)).toEqual(DEFAULT_PREFS);
	});

	test("are kept in the browser under stuarty.teleprompter and read back", () => {
		const store = storage();
		savePrefs(store, { speed: 120, size: 72, mirror: true });
		expect(Object.keys(store.data)).toEqual(["stuarty.teleprompter"]);
		expect(loadPrefs(store)).toEqual({ speed: 120, size: 72, mirror: true });
	});

	test("are held to their ranges", () => {
		expect(cleanPrefs({ speed: 5000, size: 2, mirror: true })).toEqual({
			speed: 400,
			size: 24,
			mirror: true,
		});
		expect(cleanPrefs({ speed: -3, size: 1000 })).toEqual({
			speed: 10,
			size: 120,
			mirror: false,
		});
	});

	test("ignore what is not a preference", () => {
		expect(cleanPrefs({ speed: "fast", size: null, mirror: "yes" })).toEqual(
			DEFAULT_PREFS,
		);
		expect(cleanPrefs("text")).toEqual(DEFAULT_PREFS);
		expect(loadPrefs(storage({ [PREFS_KEY]: "{not json" }))).toEqual(
			DEFAULT_PREFS,
		);
	});

	test("survive a storage that refuses", () => {
		const refusing = {
			getItem: () => {
				throw new Error("denied");
			},
			setItem: () => {
				throw new Error("denied");
			},
		};
		expect(loadPrefs(refusing)).toEqual(DEFAULT_PREFS);
		expect(() => savePrefs(refusing, DEFAULT_PREFS)).not.toThrow();
	});
});

import { describe, expect, test } from "vitest";

import { breadcrumbOf } from "./breadcrumb";
import {
	fallbackBackHref,
	type HistoryStorage,
	MAX_NAV_HISTORY,
	peekBackTarget,
	popBackTarget,
	readNavHistory,
	recordNavPath,
} from "./useBackNavigation";

function memoryStorage(): HistoryStorage {
	const data = new Map<string, string>();
	return {
		getItem: (key) => data.get(key) ?? null,
		setItem: (key, value) => {
			data.set(key, value);
		},
	};
}

describe("useBackNavigation helpers", () => {
	test("records visited routes without consecutive duplicates and caps length", () => {
		const storage = memoryStorage();
		recordNavPath(storage, "/");
		recordNavPath(storage, "/projects");
		recordNavPath(storage, "/projects");
		recordNavPath(storage, "/settings");
		expect(readNavHistory(storage)).toEqual(["/", "/projects", "/settings"]);

		for (let i = 0; i < MAX_NAV_HISTORY + 10; i += 1) {
			recordNavPath(storage, `/projects/prj_${i}`);
		}
		expect(readNavHistory(storage)).toHaveLength(MAX_NAV_HISTORY);
	});

	test("peeks and pops back through visited pages cleanly", () => {
		const storage = memoryStorage();
		recordNavPath(storage, "/");
		recordNavPath(storage, "/projects");
		recordNavPath(storage, "/settings");

		expect(peekBackTarget(storage, "/settings", "/")).toBe("/projects");
		expect(popBackTarget(storage, "/settings", "/")).toBe("/projects");
		expect(readNavHistory(storage)).toEqual(["/", "/projects"]);

		expect(popBackTarget(storage, "/projects", "/")).toBe("/");
		expect(readNavHistory(storage)).toEqual(["/"]);

		expect(peekBackTarget(storage, "/", null)).toBeNull();
	});

	test("falls back to parent breadcrumb when opened directly on a deep URL", () => {
		const storage = memoryStorage();
		expect(fallbackBackHref("/", breadcrumbOf("/"))).toBeNull();
		expect(fallbackBackHref("/projects", breadcrumbOf("/projects"))).toBe("/");
		expect(
			fallbackBackHref(
				"/projects/prj_aaaaaaaaaa",
				breadcrumbOf("/projects/prj_aaaaaaaaaa"),
			),
		).toBe("/projects");
		expect(
			peekBackTarget(
				storage,
				"/projects/prj_aaaaaaaaaa",
				fallbackBackHref(
					"/projects/prj_aaaaaaaaaa",
					breadcrumbOf("/projects/prj_aaaaaaaaaa"),
				),
			),
		).toBe("/projects");
	});
});

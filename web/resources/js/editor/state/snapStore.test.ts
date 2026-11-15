import { describe, expect, test, vi } from "vitest";

import type { SnapPointsResult } from "../api/types";
import { createSnapStore } from "./snapStore";

function answer(revision: number, frame = 10): SnapPointsResult {
	return {
		revision,
		points: [{ frame, kind: "item_start", item: "itm_a" }],
		limits: { itm_a: { trim_start_min: 0, trim_end_max: 99 } },
	};
}

function setup(answers: (rev: number | undefined) => SnapPointsResult) {
	let revision: number | null = 3;
	const getSnapPoints = vi.fn(
		async (_id: string, query?: { revision?: number; words?: boolean }) =>
			answers(query?.revision),
	);
	const store = createSnapStore({
		client: { getSnapPoints },
		projectId: "prj_1",
		getRevision: () => revision,
	});
	return {
		store,
		getSnapPoints,
		setRevision(next: number) {
			revision = next;
		},
	};
}

describe("snapStore", () => {
	test("one request per revision, however many gestures begin", async () => {
		const s = setup((rev) => answer(rev ?? 0));
		await Promise.all([s.store.begin(false), s.store.begin(false)]);
		await s.store.begin(false);
		expect(s.getSnapPoints).toHaveBeenCalledTimes(1);
		expect(s.store.points()).toHaveLength(1);
		s.setRevision(4);
		expect(s.store.points()).toBeNull();
		await s.store.begin(false);
		expect(s.getSnapPoints).toHaveBeenCalledTimes(2);
		expect(s.store.points()).not.toBeNull();
	});

	test("words are fetched again only when the cache lacks them", async () => {
		const s = setup((rev) => answer(rev ?? 0));
		await s.store.begin(true);
		await s.store.begin(false);
		await s.store.begin(true);
		expect(s.getSnapPoints).toHaveBeenCalledTimes(1);
		expect(s.getSnapPoints.mock.calls[0]?.[1]).toEqual({
			revision: 3,
			words: true,
		});
	});

	test("an answer for another revision is fetched once more, then used only if it matches", async () => {
		const s = setup((rev) => answer(rev === 3 ? 2 : (rev ?? 0)));
		await s.store.begin(false);
		expect(s.getSnapPoints).toHaveBeenCalledTimes(2);
		expect(s.store.points()).toBeNull();
	});

	test("a failed request leaves no points and does not reject", async () => {
		const s = setup(() => {
			throw new Error("down");
		});
		await expect(s.store.begin(false)).resolves.toBeUndefined();
		expect(s.store.points()).toBeNull();
	});

	test("limits come from the cached revision", async () => {
		const s = setup((rev) => answer(rev ?? 0));
		await s.store.begin(false);
		expect(s.store.limits("itm_a")).toEqual({
			trim_start_min: 0,
			trim_end_max: 99,
		});
		expect(s.store.limits("itm_x")).toBeNull();
	});
});

import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { HistoryEntry, HistoryListResult } from "../api/types";
import { devFixture } from "../boot/devFixture";
import { flush } from "../fixtures/fixtureServer";
import { type Harness, startHarness } from "../fixtures/harness";
import { EditorProvider } from "../state/EditorContext";
import { PAGE_SIZE, REFETCH_DEBOUNCE_MS } from "../state/historyStore";
import HistoryPanel from "./HistoryPanel";

const entry = (
	revision: number,
	extra: Partial<HistoryEntry> = {},
): HistoryEntry => ({
	revision,
	actor: "local:ui",
	at: "2026-11-14T12:00:00Z",
	kind: "edit",
	changes: 1,
	ops: ["item.move"],
	undo_of: null,
	undone_by: null,
	description: `step ${revision}`,
	undoable_by_me: false,
	redoable_by_me: false,
	...extra,
});

function page(
	entries: HistoryEntry[],
	next: string | null = null,
): HistoryListResult {
	return {
		project_id: "prj_x",
		revision: entries[0]?.revision ?? 0,
		entries,
		next_cursor: next,
		text: "",
	};
}

async function withHistory(
	pages: HistoryListResult[],
): Promise<{ h: Harness; calls: () => number }> {
	const h = await startHarness();
	let served = 0;
	const id = encodeURIComponent(h.server.projectId);
	h.server.routes.set(`GET /projects/${id}/history`, () => ({
		status: 200,
		body: pages[Math.min(served++, pages.length - 1)],
	}));
	const calls = (): number =>
		h.server.requests.filter((r) => r.path.includes("/history")).length;
	return { h, calls };
}

describe("history controller", () => {
	test("a gap triggers exactly one getHistory call", async () => {
		const { h, calls } = await withHistory([page([entry(3), entry(2)])]);
		await h.runtime.historyControls.refresh();
		expect(calls()).toBe(1);
		h.runtime.historyControls.onGap();
		await flush();
		expect(calls()).toBe(2);
		await h.timers.advance(5000);
		expect(calls()).toBe(2);
	});

	test("a committed batch is prepended at once and refetched once, 250 ms later", async () => {
		const { h, calls } = await withHistory([
			page([entry(2), entry(1)]),
			page([
				entry(4, { actor: "local:claude-code", undoable_by_me: false }),
				entry(3),
				entry(2),
				entry(1),
			]),
		]);
		const controls = h.runtime.historyControls;
		await controls.refresh();
		const event = (revision: number) =>
			({
				name: "batch.committed",
				project_id: h.server.projectId,
				revision,
				base_revision: revision - 1,
				kind: "edit",
				actor: "local:claude-code",
				at: "2026-11-14T12:01:00Z",
				description: `step ${revision}`,
			}) as const;
		controls.onBatch(event(3));
		controls.onBatch(event(4));
		const provisional = controls.store.getState().rows;
		expect(provisional.map((r) => [r.revision, r.provisional])).toEqual([
			[4, true],
			[3, true],
			[2, false],
			[1, false],
		]);
		expect(provisional[0]?.description).toBe("step 4");
		expect(calls()).toBe(1);
		await h.timers.advance(REFETCH_DEBOUNCE_MS - 1);
		expect(calls()).toBe(1);
		await h.timers.advance(1);
		await flush();
		expect(calls()).toBe(2);
		const rows = controls.store.getState().rows;
		expect(rows.map((r) => r.provisional)).toEqual([
			false,
			false,
			false,
			false,
		]);
		expect(rows[0]?.actor).toBe("local:claude-code");
	});

	test("the buttons learn undoable_by_me and redoable_by_me from the first page only", async () => {
		const { h } = await withHistory([
			page([
				entry(3, { undoable_by_me: true, redoable_by_me: false }),
				entry(2),
			]),
		]);
		await h.runtime.historyControls.refresh();
		const state = h.runtime.history.getState();
		expect(state.undoable).toBe(true);
		expect(state.redoable).toBe(false);
	});

	test("a page where no step is flagged disables Undo and Redo", async () => {
		const { h } = await withHistory([page([entry(1)])]);
		await h.runtime.historyControls.refresh();
		expect(h.runtime.history.getState().undoable).toBe(false);
		expect(h.runtime.history.getState().redoable).toBe(false);
	});

	test("Load more appends the next page and a refetch keeps it", async () => {
		const first = Array.from({ length: PAGE_SIZE }, (_, i) => entry(100 - i));
		const { h } = await withHistory([
			page(first, "cursor-1"),
			page([entry(50), entry(49)], null),
			page(first, "cursor-1"),
		]);
		const controls = h.runtime.historyControls;
		await controls.refresh();
		await controls.loadMore();
		expect(controls.store.getState().rows).toHaveLength(PAGE_SIZE + 2);
		expect(controls.store.getState().nextCursor).toBeNull();
		await controls.refresh();
		expect(controls.store.getState().rows).toHaveLength(PAGE_SIZE + 2);
	});

	test("undone steps are marked from undone_by", async () => {
		const { h } = await withHistory([
			page([
				entry(2, { kind: "undo", undo_of: 1 }),
				entry(1, { undone_by: 2 }),
			]),
		]);
		await h.runtime.historyControls.refresh();
		expect(h.runtime.history.getState().rows.map((r) => r.undone)).toEqual([
			false,
			true,
		]);
	});
});

describe("HistoryPanel", () => {
	test("shows the actors, the undone mark and Load more", async () => {
		const { h } = await withHistory([
			page(
				[
					entry(3, { actor: "local:claude-code" }),
					entry(2, { kind: "undo", undo_of: 1 }),
					entry(1, { undone_by: 2 }),
				],
				"c",
			),
		]);
		await h.runtime.historyControls.refresh();
		const html = renderToString(
			<I18nProvider messages={devFixture(1).bootstrap.messages} locale="en">
				<EditorProvider runtime={h.runtime}>
					<HistoryPanel />
				</EditorProvider>
			</I18nProvider>,
		);
		expect(html).toContain("claude-code");
		expect(html).toContain("You");
		expect(html).toContain('data-undone="true"');
		expect(html).toContain("Undone");
		expect(html).toContain("Load more");
		expect(html).toContain("item.move");
		expect(html.match(/data-revision=/g)).toHaveLength(3);
	});
});

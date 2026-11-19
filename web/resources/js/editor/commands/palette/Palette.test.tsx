import { describe, expect, test, vi } from "vitest";

import type { Translate } from "@/lib/i18n";
import type { ApiClient } from "../../api/client";
import { createEditQueue, type EditQueue } from "../../api/edit";
import type { CommandSummary, EditResult } from "../../api/types";
import { isPaletteShortcut } from "../keymap";
import {
	addToBatch,
	type BatchEntry,
	removeFromBatch,
	sendBatch,
} from "./batch";
import { matchRange, paletteRows } from "./usePaletteSearch";

const summary = (
	name: string,
	extra: Partial<CommandSummary> = {},
): CommandSummary => ({
	name,
	scope: "edit",
	summary: `Does ${name}.`,
	...extra,
});

// A catalogue with one translated title; the rest fall back to the summary.
const titles: Record<string, string> = {
	"ui.editor.commands.item.move.title": "Move item",
};
const t: Translate = (key) => titles[key] ?? key;

const registry: CommandSummary[] = [
	summary("item.move", { summary: "Moves an item in time." }),
	summary("item.trim"),
	summary("asset.add", { internal: true, scope: "media" }),
];

describe("paletteRows", () => {
	test("hides internal commands and sorts by title", () => {
		const rows = paletteRows(t, "en", registry, "");
		expect(rows.map((r) => r.command.name)).toEqual(["item.trim", "item.move"]);
		expect(rows[1]?.title).toBe("Move item");
		expect(rows[0]?.translated).toBe(false);
	});

	test("a command added to the registry appears without code change", () => {
		const added = [
			...registry,
			summary("brand.new", { summary: "Brand new." }),
		];
		const names = paletteRows(t, "en", added, "").map((r) => r.command.name);
		expect(names).toContain("brand.new");
	});

	test("search matches title, name and summary, every word", () => {
		const names = (q: string): string[] =>
			paletteRows(t, "en", registry, q).map((r) => r.command.name);
		expect(names("move")).toEqual(["item.move"]);
		expect(names("ITEM.TRIM")).toEqual(["item.trim"]);
		expect(names("in time")).toEqual(["item.move"]);
		expect(names("item zzz")).toEqual([]);
		expect(names("asset.add")).toEqual([]);
	});

	test("matchRange locates the query in the title", () => {
		expect(matchRange("Move item", "ove")).toEqual([1, 4]);
		expect(matchRange("Move item", "")).toBeNull();
		expect(matchRange("Move item", "zzz")).toBeNull();
	});
});

describe("batch", () => {
	const op = (name: string) => ({ op: name, args: {} });

	test("entries keep their ids after a removal", () => {
		let batch: BatchEntry[] = [];
		batch = addToBatch(batch, op("a"));
		batch = addToBatch(batch, op("b"));
		batch = removeFromBatch(batch, 1);
		batch = addToBatch(batch, op("c"));
		expect(batch.map((e) => [e.id, e.operation.op])).toEqual([
			[2, "b"],
			[3, "c"],
		]);
	});

	test("Run batch with 3 operations calls edit exactly once", async () => {
		const edit = vi.fn(async () => ({
			ok: true as const,
			result: {} as EditResult,
		}));
		const edits = {
			edit,
			undo: vi.fn(),
			redo: vi.fn(),
		} as unknown as EditQueue;
		await sendBatch(edits, 7, [op("a"), op("b"), op("c")], false);
		expect(edit).toHaveBeenCalledTimes(1);
		const [operations, revision, options] = edit.mock.calls[0] as unknown as [
			unknown[],
			number,
			{ dryRun: boolean },
		];
		expect(operations).toHaveLength(3);
		expect(revision).toBe(7);
		expect(options.dryRun).toBe(false);
	});

	test("the queue sends one request, with dry_run only for a preview", async () => {
		const requests: Record<string, unknown>[] = [];
		const client = {
			edit: async (_id: string, request: Record<string, unknown>) => {
				requests.push(request);
				return { applied: false } as EditResult;
			},
		} as unknown as ApiClient;
		let keys = 0;
		const queue = createEditQueue({
			client,
			projectId: "prj_a",
			newKey: () => `key-${++keys}`,
			onResult: () => undefined,
			onNetworkFailure: () => undefined,
		});
		await sendBatch(queue, 4, [op("a"), op("b"), op("c")], true);
		await sendBatch(queue, 5, [op("a"), op("b"), op("c")], false);
		expect(requests).toHaveLength(2);
		expect(requests[0]).toMatchObject({
			base_revision: 4,
			idempotency_key: "key-1",
			dry_run: true,
		});
		expect(requests[1]).toMatchObject({
			base_revision: 5,
			idempotency_key: "key-2",
		});
		expect(requests[1]).not.toHaveProperty("dry_run");
		expect(requests[1]).toMatchObject({ operations: [{}, {}, {}] });
	});
});

describe("isPaletteShortcut", () => {
	const key = (init: Partial<KeyboardEvent>) =>
		isPaletteShortcut({
			key: "k",
			ctrlKey: false,
			metaKey: false,
			altKey: false,
			shiftKey: false,
			...init,
		});
	test("Ctrl+K and Cmd+K only", () => {
		expect(key({ ctrlKey: true })).toBe(true);
		expect(key({ metaKey: true, key: "K" })).toBe(true);
		expect(key({})).toBe(false);
		expect(key({ ctrlKey: true, shiftKey: true })).toBe(false);
		expect(key({ ctrlKey: true, key: "j" })).toBe(false);
	});
});

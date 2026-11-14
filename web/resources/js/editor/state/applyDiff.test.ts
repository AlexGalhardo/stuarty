import { describe, expect, test } from "bun:test";

import type { Change } from "../api/types";
import { fakeId, makeDocument } from "../fixtures/fixtureServer";
import { applyDiff } from "./applyDiff";
import { createDocumentStore } from "./documentStore";

const ITEM = fakeId("itm", 1);
const OTHER = fakeId("itm", 2);

describe("applyDiff", () => {
	test("updated sets the after-values and deletes a field set to null", () => {
		const doc = makeDocument();
		const patched = applyDiff(doc, [
			{
				kind: "item.updated",
				id: ITEM,
				changes: { start: [0, 30], text: [null, "Hi"] },
			},
			{
				kind: "item.updated",
				id: ITEM,
				changes: { text: ["Hi", null] },
			},
		]);
		if (patched === "unpatchable") throw new Error("unpatchable");
		expect(patched.items[ITEM].start).toBe(30);
		expect("text" in patched.items[ITEM]).toBe(false);
	});

	test("only the changed path gets new objects", () => {
		const doc = makeDocument();
		const patched = applyDiff(doc, [
			{ kind: "item.updated", id: ITEM, changes: { start: [0, 30] } },
		]);
		if (patched === "unpatchable") throw new Error("unpatchable");
		expect(patched.items[OTHER]).toBe(doc.items[OTHER]);
		expect(patched.items[ITEM]).not.toBe(doc.items[ITEM]);
		expect(patched.tracks).toBe(doc.tracks);
	});

	test("added inserts and removed deletes", () => {
		const doc = makeDocument();
		const object = {
			id: "itm_zzzzzzzzzz",
			type: "text",
			track: "x",
			start: 1,
			duration: 2,
		};
		const added = applyDiff(doc, [
			{ kind: "item.added", id: object.id, object },
		]);
		if (added === "unpatchable") throw new Error("unpatchable");
		expect(added.items[object.id]).toEqual(object as never);
		const removed = applyDiff(added, [
			{ kind: "item.removed", id: object.id, object },
		]);
		if (removed === "unpatchable") throw new Error("unpatchable");
		expect(Object.keys(removed.items)).toEqual(Object.keys(doc.items));
	});

	test("settings and project updates", () => {
		const doc = makeDocument();
		const patched = applyDiff(doc, [
			{
				kind: "settings.updated",
				id: doc.id,
				changes: { width: [1080, 720], height: [1920, 1280] },
			},
			{
				kind: "project.updated",
				id: doc.id,
				changes: { name: [doc.name, "v2"] },
			},
		]);
		if (patched === "unpatchable") throw new Error("unpatchable");
		expect(patched.settings.width).toBe(720);
		expect(patched.name).toBe("v2");
	});

	test("an absent field matches a null before-value", () => {
		const doc = makeDocument();
		const patched = applyDiff(doc, [
			{
				kind: "track.updated",
				id: fakeId("trk", 1),
				changes: { hidden: [null, true] },
			},
		]);
		expect(patched).not.toBe("unpatchable");
	});

	const unpatchable: [string, Change][] = [
		[
			"a before-value mismatch",
			{ kind: "item.updated", id: ITEM, changes: { start: [5, 6] } },
		],
		[
			"a missing id on updated",
			{
				kind: "item.updated",
				id: "itm_missing000",
				changes: { start: [0, 1] },
			},
		],
		[
			"a missing id on removed",
			{ kind: "item.removed", id: "itm_missing000", object: {} },
		],
		[
			"an existing id on added",
			{ kind: "item.added", id: ITEM, object: { id: ITEM } },
		],
		[
			"an unknown kind",
			{ kind: "keyframe.added", id: ITEM, object: {} } as unknown as Change,
		],
	];
	for (const [name, change] of unpatchable) {
		test(`is unpatchable on ${name}`, () => {
			expect(applyDiff(makeDocument(), [change])).toBe("unpatchable");
		});
	}
});

describe("documentStore", () => {
	function store(freeze = true) {
		return createDocumentStore({ projectId: "p", freeze, now: () => 0 });
	}

	test("applyRevisionDiff requires revision current + 1", () => {
		const { store: s, writer } = store();
		writer.replaceDocument(3, makeDocument());
		const diff: Change[] = [
			{ kind: "item.updated", id: ITEM, changes: { start: [0, 9] } },
		];
		expect(writer.applyRevisionDiff(5, diff)).toBe(false);
		expect(writer.applyRevisionDiff(4, diff)).toBe(true);
		expect(s.getState().revision).toBe(4);
	});

	test("the document is deep-frozen in checked builds (document-frozen)", () => {
		const { store: s, writer } = store();
		writer.replaceDocument(0, makeDocument());
		const doc = s.getState().document as unknown as {
			items: Record<string, { start: number }>;
		};
		expect(() => {
			doc.items[ITEM].start = 99;
		}).toThrow();
	});

	test("a replacement keeps the identity of unchanged items", () => {
		const { store: s, writer } = store();
		writer.replaceDocument(0, makeDocument());
		const before = s.getState().document?.items[OTHER];
		const next = makeDocument();
		next.items[ITEM].start = 7;
		writer.replaceDocument(1, next);
		const after = s.getState().document;
		expect(after?.items[OTHER]).toBe(before as never);
		expect(after?.items[ITEM].start).toBe(7);
	});
});

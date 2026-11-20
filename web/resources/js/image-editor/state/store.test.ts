import { describe, expect, test } from "vitest";

import { ApiError } from "../../editor/api/errors";
import type { ImageApi } from "../api/imageApi";
import type {
	ImageDocument,
	ImageEditResult,
	ImageHistoryEntry,
	Layer,
} from "../api/types";
import { changedLayers, createImageStore, layerIds } from "./store";

const layer = (id: string, extra: Partial<Layer> = {}): Layer => ({
	id,
	kind: "shape",
	name: id,
	visible: true,
	opacity: 100,
	x: 0,
	y: 0,
	w: 10,
	h: 10,
	...extra,
});

function doc(revision: number, layers: Layer[]): ImageDocument {
	return {
		image_id: "img_aaaaaaaaaa",
		revision,
		name: "T",
		width: 100,
		height: 100,
		background: "#ffffff",
		template: false,
		layers,
		findings: [],
		lint: { errors: 0, warnings: 0 },
		text: "",
	};
}

const edited = (revision: number): ImageEditResult => ({
	image_id: "img_aaaaaaaaaa",
	revision,
	base_revision: revision - 1,
	applied: true,
	actor: "local:person",
	created: [],
	findings: [],
	lint: { errors: 0, warnings: 0 },
	description: "",
});

// A server in memory: it keeps one document and answers like the real one (a stale base is a conflict).
function fakeApi(start: ImageDocument): {
	api: ImageApi;
	current: () => ImageDocument;
	set(next: ImageDocument): void;
	sent: { base: number; ops: string[] }[];
	entries: ImageHistoryEntry[];
} {
	let current = start;
	const sent: { base: number; ops: string[] }[] = [];
	const entries: ImageHistoryEntry[] = [];
	const api: ImageApi = {
		list: async () => ({ images: [], next_cursor: null }),
		create: async () => ({ image_id: "x", revision: 1, width: 1, height: 1 }),
		describe: async () => current,
		edit: async (_id, base, ops) => {
			if (base !== current.revision) {
				throw new ApiError(409, {
					code: "E_REVISION_CONFLICT",
					message: "stale",
				});
			}
			sent.push({ base, ops: ops.map((o) => o.op) });
			current = doc(current.revision + 1, [
				...current.layers,
				layer(`l${current.revision}`),
			]);
			return edited(current.revision);
		},
		undo: async () => {
			current = doc(current.revision + 1, current.layers.slice(0, -1));
			return edited(current.revision);
		},
		redo: async () => edited(current.revision),
		exportImage: async () => {
			throw new Error("not used");
		},
		history: async () => ({
			image_id: "img_aaaaaaaaaa",
			revision: current.revision,
			entries,
		}),
	};
	return {
		api,
		current: () => current,
		set: (n) => (current = n),
		sent,
		entries,
	};
}

describe("the image store", () => {
	test("a batch goes out against the revision on screen and the answer replaces the document", async () => {
		const server = fakeApi(doc(1, []));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		expect(store.getState().load).toBe("ready");

		const result = await store
			.getState()
			.send([{ op: "layer.add_fill", args: { color: "#000000" } }], () => [
				"l1",
			]);
		expect(result?.revision).toBe(2);
		expect(server.sent).toEqual([{ base: 1, ops: ["layer.add_fill"] }]);
		expect(store.getState().doc?.revision).toBe(2);
		expect(store.getState().selection).toEqual(["l1"]);
	});

	test("batches run one after the other, each against the revision the last one made", async () => {
		const server = fakeApi(doc(1, []));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		await Promise.all([
			store.getState().send([{ op: "layer.add_fill" }]),
			store.getState().send([{ op: "layer.add_fill" }]),
			store.getState().send([{ op: "layer.add_fill" }]),
		]);
		expect(server.sent.map((s) => s.base)).toEqual([1, 2, 3]);
		expect(store.getState().error).toBeNull();
	});

	test("a batch given as a function is planned against the document its turn finds", async () => {
		const server = fakeApi(doc(1, []));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		const seen: number[] = [];
		const plan = (d: ImageDocument) => {
			seen.push(d.revision);
			return [{ op: "layer.add_fill" }];
		};
		await Promise.all([
			store.getState().send(plan),
			store.getState().send(plan),
		]);
		expect(seen).toEqual([1, 2]);
		// a plan with nothing to do sends nothing
		expect(await store.getState().send(() => [])).toBeNull();
		expect(server.sent).toHaveLength(2);
	});

	test("a refused batch leaves the document as it was and keeps the error", async () => {
		const server = fakeApi(doc(1, [layer("a")]));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		server.set(doc(5, [layer("a"), layer("b")]));
		const result = await store.getState().send([{ op: "layer.remove" }]);
		expect(result).toBeNull();
		expect(store.getState().error?.code).toBe("E_REVISION_CONFLICT");
		// after the conflict the screen catches up with the server
		expect(store.getState().doc?.revision).toBe(5);
	});

	test("the selection drops the layers that no longer exist", async () => {
		const server = fakeApi(doc(1, [layer("a"), layer("b")]));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		store.getState().select(["a", "b"]);
		await store.getState().undo();
		expect(server.current().layers.map((l) => l.id)).toEqual(["a"]);
		expect(store.getState().selection).toEqual(["a"]);
	});

	test("a revision another actor made replaces the document, outlines what changed and says who", async () => {
		const server = fakeApi(doc(1, [layer("a")]));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		server.entries.unshift({
			revision: 2,
			kind: "edit",
			actor: "agent:claude",
			at: "2026-11-19T12:00:00Z",
			ops: ["layer.add_text"],
			changes: 1,
			mine: false,
		});
		server.set(doc(2, [layer("a"), layer("t", { kind: "text" })]));
		await store.getState().poll();
		expect(store.getState().doc?.revision).toBe(2);
		expect(store.getState().changedByOthers).toEqual(["t"]);
		expect(store.getState().notice).toEqual({
			kind: "agent",
			actor: "agent:claude",
		});
		store.getState().clearNotice();
		expect(store.getState().notice).toBeNull();
	});

	test("a poll that finds the same revision changes nothing", async () => {
		const server = fakeApi(doc(1, [layer("a")]));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		const before = store.getState().doc;
		await store.getState().poll();
		expect(store.getState().doc).toBe(before);
		expect(store.getState().notice).toBeNull();
	});

	test("a failed open is a failed state with the error", async () => {
		const server = fakeApi(doc(1, []));
		server.api.describe = async () => {
			throw new ApiError(404, {
				code: "E_NOT_FOUND",
				message: "no such image",
			});
		};
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		expect(store.getState().load).toBe("failed");
		expect(store.getState().error?.code).toBe("E_NOT_FOUND");
	});
});

describe("layer helpers", () => {
	test("ids walk the groups, parents first", () => {
		expect(
			layerIds([
				layer("g", { kind: "group", children: [layer("a"), layer("b")] }),
				layer("c"),
			]),
		).toEqual(["g", "a", "b", "c"]);
	});

	test("a layer changed or added is reported, a group is not changed by its children", () => {
		const before = [layer("g", { kind: "group", children: [layer("a")] })];
		const after = [
			layer("g", {
				kind: "group",
				children: [layer("a", { x: 5 }), layer("n")],
			}),
		];
		expect(changedLayers(before, after)).toEqual(["a", "n"]);
	});
});

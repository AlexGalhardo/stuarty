import { describe, expect, test, vi } from "vitest";

import { ApiError } from "../../editor/api/errors";
import type { ImageApi } from "../api/imageApi";
import type {
	ImageDocument,
	ImageEditResult,
	ImageHistoryEntry,
	ImageImportResult,
	ImageUpdatedEvent,
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

const importResult = (revision: number): ImageImportResult => ({
	state: "ready",
	image_id: "img_aaaaaaaaaa",
	revision,
	created: false,
	layer_id: "imported",
	name: "photo.png",
	applied: true,
	text: "",
});

const updated = (revision: number): ImageUpdatedEvent => ({
	name: "image.updated",
	image_id: "img_aaaaaaaaaa",
	revision,
	base_revision: revision - 1,
	kind: "edit",
	actor: "agent:claude",
	at: "2026-11-20T12:00:00.000Z",
	diff: [],
});

// A server in memory: it keeps one document and answers like the real one (a stale base is a conflict).
function fakeApi(start: ImageDocument): {
	api: ImageApi;
	current: () => ImageDocument;
	set(next: ImageDocument): void;
	sent: { base: number; ops: string[] }[];
	imports: { base?: number; path?: string; file?: string }[];
	entries: ImageHistoryEntry[];
} {
	let current = start;
	const sent: { base: number; ops: string[] }[] = [];
	const imports: { base?: number; path?: string; file?: string }[] = [];
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
		importImage: async (request) => {
			imports.push({ base: request.base_revision, path: request.path });
			current = doc(current.revision + 1, [
				...current.layers,
				layer("imported"),
			]);
			return importResult(current.revision);
		},
		importFile: async (file, request) => {
			imports.push({ base: request.base_revision, file: file.name });
			current = doc(current.revision + 1, [
				...current.layers,
				layer("imported"),
			]);
			return importResult(current.revision);
		},
		checkpoints: async () => ({
			image_id: "img_aaaaaaaaaa",
			revision: current.revision,
			checkpoints: [],
			text: "",
		}),
		createCheckpoint: async () => {
			throw new Error("not used");
		},
		removeCheckpoint: async () => {
			throw new Error("not used");
		},
		diff: async () => {
			throw new Error("not used");
		},
		revert: async () => {
			throw new Error("not used");
		},
		probe: async () => 200,
	};
	return {
		api,
		current: () => current,
		set: (n) => (current = n),
		sent,
		imports,
		entries,
	};
}

describe("the image store", () => {
	test("reload reads the document and the history again after a step the window did not send", async () => {
		const server = fakeApi(doc(1, []));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();

		// a revert made through the API moves the document under the window
		server.set(doc(5, [layer("back")]));
		await store.getState().reload();

		expect(store.getState().doc?.revision).toBe(5);
		expect(store.getState().doc?.layers.map((l) => l.id)).toEqual(["back"]);
	});

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

	test("an event of a newer revision fetches the document, outlines what changed and says who", async () => {
		const server = fakeApi(doc(1, [layer("a")]));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		server.entries.unshift({
			revision: 2,
			kind: "edit",
			actor: "agent:claude",
			at: "2026-11-20T12:00:00Z",
			ops: ["layer.add_text"],
			changes: 1,
			mine: false,
		});
		server.set(doc(2, [layer("a"), layer("t", { kind: "text" })]));
		await store.getState().receive(updated(2));
		expect(store.getState().doc?.revision).toBe(2);
		expect(store.getState().changedByOthers).toEqual(["t"]);
		expect(store.getState().notice).toEqual({
			kind: "agent",
			actor: "agent:claude",
		});
	});

	test("an event of the revision on screen, or an older one, is ignored: a commit of this window is applied once", async () => {
		const server = fakeApi(doc(1, []));
		let describes = 0;
		const inner = server.api.describe;
		server.api.describe = async (id, signal) => {
			describes++;
			return inner(id, signal);
		};
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		await store.getState().send([{ op: "layer.add_fill" }]);
		const before = describes;
		const doc2 = store.getState().doc;
		// the event of the revision this window just made, and an old one
		await store.getState().receive(updated(2));
		await store.getState().receive(updated(1));
		expect(describes).toBe(before);
		expect(store.getState().doc).toBe(doc2);
		expect(store.getState().notice).toBeNull();
	});

	test("an event that arrives while a command runs is looked at once the command ends", async () => {
		const server = fakeApi(doc(1, []));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		const edit = server.api.edit;
		server.api.edit = async (id, base, ops) => {
			const result = await edit(id, base, ops);
			// another actor commits revision 3 while this window's batch (revision 2) is being answered
			server.set(doc(3, [layer("x"), layer("y"), layer("z")]));
			await store.getState().receive(updated(3));
			return result;
		};
		await store.getState().send([{ op: "layer.add_fill" }]);
		await vi.waitFor(() => expect(store.getState().doc?.revision).toBe(3));
	});

	test("a file is put in this image against the revision on screen, by its path or as a file", async () => {
		const server = fakeApi(doc(1, [layer("a")]));
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		const byPath = await store
			.getState()
			.importFile({ path: "/home/ana/a.png" });
		expect(byPath?.revision).toBe(2);
		expect(store.getState().selection).toEqual(["imported"]);
		const file = new File([new Uint8Array([1, 2, 3])], "b.png");
		await store.getState().importFile({ file });
		expect(server.imports).toEqual([
			{ base: 1, path: "/home/ana/a.png" },
			{ base: 2, file: "b.png" },
		]);
		expect(store.getState().doc?.revision).toBe(3);
	});

	test("a file the server is still reading makes no layer and says so", async () => {
		const server = fakeApi(doc(1, [layer("a")]));
		server.api.importFile = async () => ({
			state: "pending",
			hash: "sha256:00",
			created: false,
			name: "big.png",
			applied: false,
			text: "file sha256:00 is still being ingested",
		});
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		const file = new File([new Uint8Array([1, 2, 3])], "big.png");
		expect(await store.getState().importFile({ file })).toBeNull();
		expect(store.getState().error?.code).toBe("E_ASSET_NOT_READY");
		expect(store.getState().doc?.revision).toBe(1);
		expect(store.getState().busy).toBe(false);
	});

	test("a refused import keeps the error and leaves the document as it was", async () => {
		const server = fakeApi(doc(1, [layer("a")]));
		server.api.importImage = async () => {
			throw new ApiError(422, {
				code: "E_UNSUPPORTED_MEDIA",
				message: "not an image",
			});
		};
		const store = createImageStore(server.api, "img_aaaaaaaaaa");
		await store.getState().open();
		expect(await store.getState().importFile({ path: "/x.mov" })).toBeNull();
		expect(store.getState().error?.code).toBe("E_UNSUPPORTED_MEDIA");
		expect(store.getState().doc?.revision).toBe(1);
		expect(store.getState().busy).toBe(false);
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

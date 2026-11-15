import { afterEach, describe, expect, test, vi } from "vitest";

import { createStore } from "../state/createStore";
import type { DocumentState } from "../state/documentStore";
import type { EditorRuntime } from "../state/runtime";
import { installDebugHooks } from "./debugHooks";

function fakeRuntime(): {
	runtime: EditorRuntime;
	store: ReturnType<typeof createStore<DocumentState>>;
} {
	const store = createStore<DocumentState>({
		projectId: "prj_a",
		status: "ready",
		connection: "open",
		revision: 3,
		document: { items: { itm_a: { start: 0 } } },
		endFrame: 0,
		lastEventAt: 0,
	} as unknown as DocumentState);
	return { runtime: { documents: store } as unknown as EditorRuntime, store };
}

afterEach(() => vi.unstubAllGlobals());

describe("debug hooks (SDD-012 section 20.4)", () => {
	test("read-only getters, a frozen clone and the revision callback", () => {
		const win: { __vv?: ReturnType<typeof read> } = {};
		vi.stubGlobal("window", win);
		const { runtime, store } = fakeRuntime();
		installDebugHooks(runtime, []);
		const vv = read(win);
		expect(vv.revision).toBe(3);
		expect(vv.connection).toBe("open");
		const doc = vv.document as { items: { itm_a: { start: number } } };
		expect(Object.isFrozen(doc.items.itm_a)).toBe(true);
		// The clone is not the store's document: writing it changes nothing.
		expect(doc).not.toBe(store.getState().document);
		expect(() => {
			(vv as unknown as { revision: number }).revision = 9;
		}).toThrow();

		const seen: number[] = [];
		const off = vv.onRevisionCommit((r) => seen.push(r));
		store.setState((s) => ({ ...s, revision: 4 }));
		store.setState((s) => ({ ...s, connection: "offline" }));
		off();
		store.setState((s) => ({ ...s, revision: 5 }));
		expect(seen).toEqual([4]);
	});

	test("a build that is not the end-to-end build has no dispatch", () => {
		const win: { __vv?: ReturnType<typeof read> } = {};
		vi.stubGlobal("window", win);
		installDebugHooks(fakeRuntime().runtime, []);
		expect(read(win).dispatch).toBeUndefined();
	});

	test("mismatches reports the revisions the verify check collected", () => {
		const win: { __vv?: ReturnType<typeof read> } = {};
		vi.stubGlobal("window", win);
		const found = [7];
		installDebugHooks(fakeRuntime().runtime, found);
		expect(read(win).mismatches).toEqual([7]);
	});
});

function read(win: { __vv?: Window["__vv"] }): NonNullable<Window["__vv"]> {
	if (!win.__vv) throw new Error("hooks not installed");
	return win.__vv;
}

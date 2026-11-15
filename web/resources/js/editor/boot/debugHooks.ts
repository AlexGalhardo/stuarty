import type { EditOutcome } from "../api/edit";
import type { ConnectionState } from "../api/events";
import type { EditorRuntime } from "../state/runtime";
import { type SnapResult, snapFrame } from "../timeline/snapping";

// The test hooks of SDD-012 section 20.4. `?debug=1` installs window.__vv with
// read-only getters: nothing here changes the project or a store, so it is
// safe in a production build. The one function that does change the project,
// dispatch, is written inside the VITE_E2E branch below, which a production
// build removes (scripts/check-hooks.ts greps the bundle for its marker).

type EditOperations = Parameters<EditorRuntime["edits"]["edit"]>[0];

export interface DebugHooks {
	readonly revision: number | null;
	// A frozen clone: a test cannot write the read model through it.
	readonly document: unknown;
	readonly connection: ConnectionState;
	readonly ghosts: readonly unknown[];
	// Revisions at which the document patched from events differed from the
	// fetched one (?verify=1). A test asserts it is empty.
	readonly mismatches: readonly number[];
	measures(): { name: string; duration: number }[];
	onRevisionCommit(callback: (revision: number) => void): () => void;
	// End-to-end build only: the same queue the UI uses, no pointer gesture.
	dispatch?(operations: EditOperations): Promise<EditOutcome>;
	// End-to-end build only: what a move gesture does at its first pointer move
	// and at each later one (fetch the snap points of the revision, snap one
	// frame, draw the snap line). The move and trim gestures call the same
	// functions; this stands in for them until a spec can drag.
	snapProbe?(options: {
		frame: number;
		exclude?: string[];
		words?: boolean;
		alt?: boolean;
	}): Promise<SnapResult>;
}

declare global {
	interface Window {
		__vv?: DebugHooks;
	}
}

// The measures the hooks report: vv:edit (dispatch to the committed answer)
// and vv:edit-apply (the store update alone), vv:truth, vv:pause.
function deepFreeze<T>(value: T): T {
	if (typeof value === "object" && value !== null && !Object.isFrozen(value)) {
		for (const inner of Object.values(value)) deepFreeze(inner);
		Object.freeze(value);
	}
	return value;
}

export function installDebugHooks(
	runtime: EditorRuntime,
	mismatches: number[],
): void {
	const hooks: DebugHooks = {
		get revision() {
			return runtime.documents.getState().revision;
		},
		get document() {
			return deepFreeze(structuredClone(runtime.documents.getState().document));
		},
		get connection() {
			return runtime.documents.getState().connection;
		},
		ghosts: [],
		mismatches,
		measures: () =>
			performance
				.getEntriesByType("measure")
				.filter((m) => m.name.startsWith("vv:"))
				.map((m) => ({ name: m.name, duration: m.duration })),
		onRevisionCommit(callback) {
			let last = runtime.documents.getState().revision;
			return runtime.documents.subscribe((state) => {
				if (state.revision !== null && state.revision !== last) {
					last = state.revision;
					callback(state.revision);
				}
			});
		},
	};

	if (import.meta.env.VITE_E2E === "1") {
		hooks.dispatch = async (operations) => {
			const revision = runtime.documents.getState().revision;
			if (revision === null) throw new Error("vv-e2e-dispatch: not ready");
			const start = performance.now();
			const outcome = await runtime.edits.edit(operations, revision);
			performance.measure("vv:edit", { start, end: performance.now() });
			return outcome;
		};
	}

	if (import.meta.env.VITE_E2E === "1") {
		hooks.snapProbe = async ({
			frame,
			exclude = [],
			words = false,
			alt = false,
		}) => {
			await runtime.snap.begin(words);
			const ui = runtime.ui.getState();
			const result = snapFrame({
				frame,
				ppf: ui.timeline.pxPerFrame,
				playhead: ui.playheadFrame,
				points: runtime.snap.points(),
				exclude: new Set(exclude),
				enabled: ui.timeline.snapEnabled,
				suspended: alt,
			});
			runtime.ghostWriter.setSnapLine(result.line);
			return result;
		};
	}

	window.__vv = hooks;
}

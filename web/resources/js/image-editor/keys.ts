import { type KeyboardEvent as ReactKeyboardEvent, useCallback } from "react";

import { ZOOM_STEP } from "./canvas/viewport";
import { moveOp, step, treeRows } from "./layers/tree";
import { useImageStore } from "./state/context";

// A target that takes text keeps its own keys: undo inside a field undoes the typing, not the document.
function takesText(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	return (
		target.closest('[role="dialog"]') !== null ||
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
	);
}

// The keys of the whole editor (SDD-012 section 24.10), for a focus that is not in a field: undo and redo,
// duplicate, group and ungroup, remove, one place back or forward, and the zoom. A key that the canvas or the
// layer panel handled has its default prevented and is left alone here.
export function useEditorKeys(): (event: ReactKeyboardEvent) => void {
	const store = useImageStore();

	return useCallback(
		(event) => {
			if (event.defaultPrevented || takesText(event.target)) return;
			const state = store.getState();
			if (state.doc === null) return;
			const mod = event.ctrlKey || event.metaKey;
			const key = event.key.toLowerCase();
			const ids = [...state.selection];

			const done = (): void => event.preventDefault();

			if (mod && !event.altKey) {
				if (key === "z") {
					done();
					void (event.shiftKey ? state.redo() : state.undo());
				} else if (key === "y") {
					done();
					void state.redo();
				} else if (key === "d" && ids.length > 0) {
					done();
					void state.send(
						ids.map((layer_id) => ({
							op: "layer.duplicate",
							args: { layer_id, dx: 10, dy: 10 },
						})),
						(r) => r.created,
					);
				} else if (key === "g" && ids.length > 0) {
					done();
					if (event.shiftKey) {
						void state.send(
							ids.map((layer_id) => ({
								op: "layer.ungroup",
								args: { layer_id },
							})),
							() => [],
						);
					} else {
						void state.send(
							[{ op: "layer.group", args: { layer_ids: ids } }],
							(r) => r.created.slice(0, 1),
						);
					}
				}
				return;
			}
			if (event.altKey) return;

			if (
				(event.key === "Delete" || event.key === "Backspace") &&
				ids.length > 0
			) {
				done();
				void state.send(
					ids.map((layer_id) => ({ op: "layer.remove", args: { layer_id } })),
				);
			} else if ((event.key === "[" || event.key === "]") && ids.length === 1) {
				done();
				const forward = event.key === "]";
				void state.send((doc) => {
					const row = treeRows(doc.layers, new Set()).find(
						(r) => r.layer.id === ids[0],
					);
					const move =
						row === undefined ? null : step(row, forward ? "up" : "down");
					return move === null ? [] : [moveOp(move)];
				});
			} else if (event.key === "+" || event.key === "=") {
				done();
				state.zoomBy(ZOOM_STEP);
			} else if (event.key === "-") {
				done();
				state.zoomBy(1 / ZOOM_STEP);
			} else if (event.key === "0") {
				done();
				state.fitView();
			} else if (event.key === "1") {
				done();
				state.actualSize();
			}
		},
		[store],
	);
}

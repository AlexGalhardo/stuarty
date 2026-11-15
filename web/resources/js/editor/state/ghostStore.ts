import { createStore, type ReadStore, readOnly } from "./createStore";

// Ephemeral drag previews (SDD-012 sections 5.1 and 8.1, rule T4): written only
// by gestures, read only by the ghost layer and the snap line. The read model
// never changes while one is on screen.
export interface GhostState {
	// The frame of the line drawn while a gesture is snapped, else null.
	snapLine: number | null;
}

export interface GhostWriter {
	setSnapLine(frame: number | null): void;
}

export function createGhostStore(): {
	store: ReadStore<GhostState>;
	writer: GhostWriter;
} {
	const store = createStore<GhostState>({ snapLine: null });
	return {
		store: readOnly(store),
		writer: {
			setSnapLine: (snapLine) =>
				store.setState((s) =>
					s.snapLine === snapLine ? s : { ...s, snapLine },
				),
		},
	};
}

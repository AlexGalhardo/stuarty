import { createStore, type ReadStore, readOnly } from "./createStore";

// Ephemeral drag previews (SDD-012 sections 5.1 and 8.1, rule T4): written only
// by gestures, read only by the ghost layer, the snap line and the outline of a
// conflicting item. The read model never changes while one is on screen.
export interface Ghost {
	start: number;
	track: string;
	duration: number;
	// Released and waiting for the answer of the core: drawn with a dashed outline.
	pending: boolean;
}

export interface GhostState {
	ghosts: Readonly<Record<string, Ghost>>;
	// The frame of the line drawn while a gesture is snapped, else null.
	snapLine: number | null;
	// The item named by `details.conflicting_item`, outlined for a moment.
	errorItem: string | null;
}

export interface GhostWriter {
	setGhosts(ghosts: Readonly<Record<string, Ghost>>): void;
	// The released gesture waits for the core: every ghost turns pending.
	markPending(): void;
	setSnapLine(frame: number | null): void;
	setErrorItem(id: string | null): void;
	// The gesture ended or was cancelled: no ghost and no snap line remain.
	clear(): void;
}

const EMPTY: GhostState = { ghosts: {}, snapLine: null, errorItem: null };

export function createGhostStore(): {
	store: ReadStore<GhostState>;
	writer: GhostWriter;
} {
	const store = createStore<GhostState>(EMPTY);
	return {
		store: readOnly(store),
		writer: {
			setGhosts: (ghosts) => store.setState((s) => ({ ...s, ghosts })),
			markPending: () =>
				store.setState((s) => ({
					...s,
					ghosts: Object.fromEntries(
						Object.entries(s.ghosts).map(([id, g]) => [
							id,
							{ ...g, pending: true },
						]),
					),
				})),
			setSnapLine: (snapLine) =>
				store.setState((s) =>
					s.snapLine === snapLine ? s : { ...s, snapLine },
				),
			setErrorItem: (errorItem) =>
				store.setState((s) =>
					s.errorItem === errorItem ? s : { ...s, errorItem },
				),
			clear: () =>
				store.setState((s) => ({ ...s, ghosts: {}, snapLine: null })),
		},
	};
}

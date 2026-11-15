import { createStore, type ReadStore, readOnly } from "./createStore";

// The scale and opacity the plan reports for each visible layer at the frame of
// the preview (SDD-004 section 10). The preview writes it when a plan window
// arrives; the inspector reads it instead of working out a keyframed value.
export interface EffectiveValues {
	scale: number;
	opacity: number;
}

export interface EffectiveState {
	items: Readonly<Record<string, EffectiveValues>>;
}

export interface EffectiveWriter {
	set(items: Readonly<Record<string, EffectiveValues>>): void;
}

export function createEffectiveStore(): {
	store: ReadStore<EffectiveState>;
	writer: EffectiveWriter;
} {
	const store = createStore<EffectiveState>({ items: {} });
	return {
		store: readOnly(store),
		writer: { set: (items) => store.setState(() => ({ items })) },
	};
}

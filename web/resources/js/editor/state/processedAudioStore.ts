import { createStore, type ReadStore, readOnly } from "./createStore";

// What the timeline preview plays for an item that has a processed sound
// (SDD-004 section 7.1): "processing" while the file is rendered and the item
// plays unprocessed, "ready" once the preview plays the file. The preview
// writes it; the audio processors panel of the inspector reads it.
export type ProcessedState = "processing" | "ready";

export interface ProcessedAudioState {
	items: Readonly<Record<string, ProcessedState>>;
}

const store = createStore<ProcessedAudioState>({ items: {} });

export const processedAudioStates: ReadStore<ProcessedAudioState> =
	readOnly(store);

// Replaces the whole map: an item that is not in it plays from its proxy.
export function setProcessedAudioStates(
	items: Readonly<Record<string, ProcessedState>>,
): void {
	const old = store.getState().items;
	const keys = Object.keys(items);
	if (
		keys.length === Object.keys(old).length &&
		keys.every((key) => old[key] === items[key])
	) {
		return;
	}
	store.setState(() => ({ items }));
}

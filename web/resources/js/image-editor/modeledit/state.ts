import { useStore } from "zustand";
import { createStore } from "zustand/vanilla";

import type { Job, ModelView, ProviderView } from "../../editor/api/types";
import type { ModelOperation } from "../api/modelEdit";

// What the screen keeps of an operation with a model (SDD-016 section 6.4): the job that was started, so that
// the strip under the toolbar can follow it after the dialog is closed. Nothing here is a result: the job is
// what the service answered.

export interface TrackedJob {
	job: Job;
	operation: ModelOperation;
	// The layer the operation was started for, and its name when it was started, for the words of the strip.
	layerId: string | null;
	layerName: string;
}

export interface ModelJobState {
	tracked: TrackedJob | null;
	track(tracked: TrackedJob): void;
	update(job: Job): void;
	dismiss(): void;
}

export const modelJobStore = createStore<ModelJobState>()((set, get) => ({
	tracked: null,
	track: (tracked) => set({ tracked }),
	update: (job) => {
		const current = get().tracked;
		if (current !== null && current.job.job_id === job.job_id) {
			set({ tracked: { ...current, job } });
		}
	},
	dismiss: () => set({ tracked: null }),
}));

export function useModelJob<T>(selector: (state: ModelJobState) => T): T {
	return useStore(modelJobStore, selector);
}

export function isFinal(job: Job): boolean {
	return (
		job.state === "succeeded" ||
		job.state === "failed" ||
		job.state === "cancelled"
	);
}

// The capability a provider needs for the operation: removing a background sends the picture to an editor of
// images, generating sends the prompt to a maker of images.
export function capabilityOf(operation: ModelOperation): string {
	return operation === "remove_background" ? "image_edit" : "image";
}

export interface Choice {
	connectionId: string;
	providerName: string;
	label: string;
}

// The connections of the providers that have the capability of the operation.
export function choicesOf(
	providers: readonly ProviderView[],
	operation: ModelOperation,
): Choice[] {
	const out: Choice[] = [];
	for (const provider of providers) {
		if (!provider.capabilities.includes(capabilityOf(operation))) continue;
		for (const id of provider.connections) {
			out.push({
				connectionId: id,
				providerName: provider.name,
				label: `${provider.name} (${id})`,
			});
		}
	}
	return out;
}

// The models the picker offers: the ones that are hinted as image models first, since the hint is not a rule
// and the provider is the judge.
export function orderedModels(models: readonly ModelView[]): ModelView[] {
	return [
		...models.filter((model) => model.capability === "image"),
		...models.filter((model) => model.capability !== "image"),
	];
}

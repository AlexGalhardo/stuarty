import type { PlanWindow, PlanWindowLayer } from "../api/types";

// A layer or a sound that comes from a composition names its asset by the id
// it has in the nested timeline, and says which timeline that is (SDD-004
// section 3.16). The files of an asset are served under the timeline that
// holds it, so the executors ask this table where an asset lives; an asset the
// plan does not attribute lives in the timeline being edited.
export class AssetOwners {
	private readonly owners = new Map<string, string>();

	// Remembers the owners named by a plan window, the layers of a group
	// included. A timeline that is not named in the window keeps what it had:
	// the decoders ask for an asset after the window that named it may have
	// been replaced.
	note(plan: PlanWindow): void {
		this.noteLayers(plan.layers);
		for (const audio of plan.audio) {
			if (audio.timeline !== undefined)
				this.owners.set(audio.asset, audio.timeline);
		}
	}

	private noteLayers(layers: readonly PlanWindowLayer[]): void {
		for (const layer of layers) {
			if (layer.kind === "group") this.noteLayers(layer.group.layers);
			else if (layer.kind === "media" && layer.timeline !== undefined)
				this.owners.set(layer.asset, layer.timeline);
		}
	}

	of(asset: string, projectId: string): string {
		return this.owners.get(asset) ?? projectId;
	}
}

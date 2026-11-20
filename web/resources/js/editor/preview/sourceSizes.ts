import type { ApiClient } from "../api/client";

export interface SourceSize {
	width: number;
	height: number;
}

// The size of the source file of an asset: the space the crop of a plan layer
// is measured in (SDD-004 section 3.4), which a still poster is scaled from
// (stillFrame.ts). The document of the open timeline names its own assets. An
// asset of a nested timeline (a layer of a composition, SDD-004 section 3.16)
// may not be in it: its size is asked once of the timeline that holds it
// (media.inspect, the same timeline its poster and proxy are served under) and
// remembered, so a picture wider than the poster is not cut as if it were the
// poster.
export class SourceSizes {
	private readonly nested = new Map<string, SourceSize | null>();
	private readonly asking = new Set<string>();

	constructor(
		private readonly open: (asset: string) => SourceSize | null,
		private readonly inspect: (
			timeline: string,
			asset: string,
		) => Promise<SourceSize | null>,
		private readonly changed: () => void,
	) {}

	// The size of `asset`, whose files are served under `owner`; `projectId` is
	// the open timeline. Null when it is not known (the poster is then taken as
	// the source, as for a still at most 640 wide), undefined while it is asked.
	of(
		asset: string,
		owner: string,
		projectId: string,
	): SourceSize | null | undefined {
		const own = this.open(asset);
		if (own !== null || owner === projectId) return own;
		const key = `${owner}/${asset}`;
		if (this.nested.has(key)) return this.nested.get(key) ?? null;
		if (!this.asking.has(key)) {
			this.asking.add(key);
			this.inspect(owner, asset)
				.catch(() => null)
				.then((size) => {
					this.asking.delete(key);
					this.nested.set(key, size);
					this.changed();
				});
		}
		return undefined;
	}
}

// The size the probe of an asset gives, null when it gives none.
export async function inspectSourceSize(
	client: Pick<ApiClient, "inspectMedia">,
	timeline: string,
	asset: string,
): Promise<SourceSize | null> {
	const result = await client.inspectMedia(timeline, asset);
	const width = result.probe?.width;
	const height = result.probe?.height;
	if (
		typeof width !== "number" ||
		typeof height !== "number" ||
		!(width > 0) ||
		!(height > 0)
	) {
		return null;
	}
	return { width, height };
}

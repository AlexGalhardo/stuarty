import type { ApiClient } from "../api/client";
import type { SnapPoint, SnapPointsResult } from "../api/types";

// The snap points of the core, cached by revision (SDD-012 section 7.8). The
// first move or trim gesture after a revision change calls `begin`, which makes
// the one request of that revision; every later gesture reads the cache.
export interface SnapStore {
	// Fetch the points for the current revision unless they are cached.
	// `words` adds the caption word boundaries (ppf >= 2 and a caption is dragged);
	// a cache without them is fetched again when they are needed. Never rejects:
	// when the request fails, `points()` stays null and gestures snap to the
	// playhead and frame 0 only.
	begin(words: boolean): Promise<void>;
	// The points, only when they belong to the revision of the read model.
	points(): readonly SnapPoint[] | null;
	// Per item id, the frames a start edge and an end edge can reach.
	limits(
		itemId: string,
	): { trim_start_min: number; trim_end_max: number } | null;
}

export function createSnapStore(options: {
	client: Pick<ApiClient, "getSnapPoints">;
	projectId: string;
	getRevision(): number | null;
}): SnapStore {
	const { client, projectId, getRevision } = options;
	let cache: { result: SnapPointsResult; words: boolean } | null = null;
	let inflight: {
		revision: number;
		words: boolean;
		done: Promise<void>;
	} | null = null;

	const current = (): SnapPointsResult | null => {
		const revision = getRevision();
		return cache !== null && cache.result.revision === revision
			? cache.result
			: null;
	};

	async function load(revision: number, words: boolean): Promise<void> {
		let result = await client.getSnapPoints(projectId, { revision, words });
		const now = getRevision();
		// The core answered for a revision the read model has not reached or has
		// passed: ask once more for the one the read model holds.
		if (now !== null && result.revision !== now) {
			result = await client.getSnapPoints(projectId, { revision: now, words });
		}
		cache = { result, words };
	}

	return {
		begin(words) {
			const revision = getRevision();
			if (revision === null) return Promise.resolve();
			if (current() !== null && (cache?.words || !words)) {
				return Promise.resolve();
			}
			if (
				inflight !== null &&
				inflight.revision === revision &&
				(inflight.words || !words)
			) {
				return inflight.done;
			}
			const done = load(revision, words)
				.catch(() => undefined)
				.finally(() => {
					if (inflight?.done === done) inflight = null;
				});
			inflight = { revision, words, done };
			return done;
		},
		points: () => current()?.points ?? null,
		limits: (itemId) => current()?.limits[itemId] ?? null,
	};
}

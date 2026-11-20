// A first-come queue that lets at most `limit` holders run at once. The server
// answers 429 to a credential that has more than two frame requests in flight
// (SDD-007 section 4.4), so a page that draws many pictures asks for them
// through one of these instead of all at once. It schedules requests; it
// computes nothing the user sees (SDD-012 section 1).
export type Release = () => void;

export interface SlotQueue {
	// acquire calls `start` with a release function once a slot is free. The
	// returned function withdraws the request: it frees the slot if it was
	// taken and drops the request if it was still waiting. Calling release
	// more than once is harmless.
	acquire(start: (release: Release) => void): () => void;
}

export function createSlotQueue(limit: number): SlotQueue {
	let running = 0;
	const waiting: Array<() => void> = [];

	const next = (): void => {
		while (running < limit && waiting.length > 0) {
			waiting.shift()?.();
		}
	};

	return {
		acquire(start) {
			let state: "waiting" | "running" | "done" = "waiting";

			const release: Release = () => {
				if (state === "running") running--;
				state = "done";
				next();
			};

			const run = (): void => {
				state = "running";
				running++;
				start(release);
			};

			if (running < limit) {
				run();
			} else {
				waiting.push(run);
			}

			return () => {
				if (state === "waiting") {
					const at = waiting.indexOf(run);
					if (at >= 0) waiting.splice(at, 1);
					state = "done";
					return;
				}
				release();
			};
		},
	};
}

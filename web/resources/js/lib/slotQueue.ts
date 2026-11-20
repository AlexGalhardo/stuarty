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

export interface NearTicket {
	// near reports whether the element is near the viewport.
	near(isNear: boolean): void;
	// stop withdraws the request, or frees its slot if it started.
	stop(): void;
}

// whileNear asks `queue` for a slot while an element is near the viewport and
// withdraws the request if the element leaves before its turn, so scrolling
// past a hundred cards does not line up a hundred pictures ahead of the ones on
// screen (VV-238). Once `start` has run, leaving the viewport changes nothing:
// the request is in flight and keeps its slot until it is released.
export function whileNear(
	queue: SlotQueue,
	start: (release: Release) => void,
): NearTicket {
	let withdraw: (() => void) | null = null;
	let started = false;

	return {
		near(isNear) {
			if (started) return;
			if (isNear && !withdraw) {
				withdraw = queue.acquire((release) => {
					started = true;
					start(release);
				});
			} else if (!isNear && withdraw) {
				withdraw();
				withdraw = null;
			}
		},
		stop() {
			withdraw?.();
			withdraw = null;
		},
	};
}

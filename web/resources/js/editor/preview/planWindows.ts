import type { PlanWindow } from "../api/types";
import { Lru } from "./lru";

export const WINDOW_FRAMES = 300;
export const PREFETCH_FRAMES = 60;
export const MAX_WINDOWS = 8;

export function windowStart(frame: number): number {
	return Math.floor(Math.max(frame, 0) / WINDOW_FRAMES) * WINDOW_FRAMES;
}

export type GetPlan = (
	from: number,
	to: number,
	revision: number,
	width: number | undefined,
) => Promise<PlanWindow>;

// The plan windows of the playhead and the next one (SDD-012 section 11.2
// item 1): at most 8, least recently used out, all dropped on a new revision.
export class PlanWindows {
	private revision = 0;
	// The pixel width the preview draws at: the plan rasterizes its bitmaps at that
	// scale (SDD-004 section 10). Undefined asks for the project width.
	private width: number | undefined;
	private readonly cache = new Lru<number, PlanWindow>(MAX_WINDOWS);
	private readonly inFlight = new Map<number, Promise<PlanWindow>>();

	constructor(private readonly getPlan: GetPlan) {}

	get size(): number {
		return this.cache.size;
	}

	setRevision(revision: number): void {
		if (revision === this.revision) return;
		this.revision = revision;
		this.cache.clear();
		this.inFlight.clear();
	}

	setWidth(width: number | undefined): void {
		if (width === this.width) return;
		this.width = width;
		this.cache.clear();
		this.inFlight.clear();
	}

	peek(frame: number): PlanWindow | undefined {
		return this.cache.get(windowStart(frame));
	}

	ensure(frame: number): Promise<PlanWindow> {
		const from = windowStart(frame);
		const cached = this.cache.get(from);
		if (cached) return Promise.resolve(cached);
		const running = this.inFlight.get(from);
		if (running) return running;
		const revision = this.revision;
		const width = this.width;
		const request = this.getPlan(
			from,
			from + WINDOW_FRAMES,
			revision,
			width,
		).then((plan) => {
			// A revision or width change while waiting makes the answer stale.
			if (revision === this.revision && width === this.width) {
				this.cache.set(from, plan);
			}
			return plan;
		});
		this.inFlight.set(from, request);
		const forget = (): void => {
			if (this.inFlight.get(from) === request) this.inFlight.delete(from);
		};
		request.then(forget, forget);
		return request;
	}

	// Asks for the next window when the playhead is within 60 frames of the end
	// of the current one.
	prefetch(frame: number): void {
		const next = windowStart(frame) + WINDOW_FRAMES;
		if (next - frame > PREFETCH_FRAMES) return;
		this.ensure(next).catch(() => undefined);
	}
}

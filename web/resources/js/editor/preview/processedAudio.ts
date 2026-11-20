import { fetchProbe, type Probe, type ProbeResult } from "../api/audioPreview";
import type { PlanAudio } from "../api/types";
import { audioPreviewUrl } from "../api/urls";
import type { ProcessedState } from "../state/processedAudioStore";
import type { PoolTimers } from "./mediaPool";

// How the preview plays the processed sound of an item (SDD-004 section 7.1,
// SDD-012 section 11.2 item 4). The server renders the file; this module only
// asks for it, waits for it and says which source an item's audio element plays.
// No sample is computed here.

export const POLL_MS = 500;
// A window of two seconds is quick; a whole item can take a while.
export const WINDOW_POLLS = 20;
export const FULL_POLLS = 240;
export const RETRY_AFTER_FAILURE_MS = 10000;

export type { Probe, ProbeResult };

export interface ProcessedSource {
	url: string;
	// The timeline frame of the first sample of the file.
	zero: number;
}

type Phase = "idle" | "polling" | "ready" | "failed";

interface Slot {
	phase: Phase;
	retryAt: number;
}

interface WindowSlot extends Slot {
	from: number;
	first: number;
	last: number;
}

interface Entry {
	full: Slot;
	windows: WindowSlot[];
}

export interface ProcessedAudioDeps {
	projectId: () => string;
	probe?: Probe;
	timers: PoolTimers;
	now: () => number;
	// Called when a file became ready or failed: the executor draws again.
	onChange: () => void;
}

const MAX_WINDOWS_PER_KEY = 8;

export class ProcessedAudio {
	private readonly entries = new Map<string, Entry>();
	private readonly items = new Map<string, string>();
	private readonly handles = new Set<unknown>();
	private readonly probe: Probe;
	private disposed = false;

	constructor(private readonly deps: ProcessedAudioDeps) {
		this.probe = deps.probe ?? fetchProbe;
	}

	// The file that the element of `audio` plays at `frame`, or null while it
	// plays the proxy. It starts the render the first time it is asked.
	resolve(audio: PlanAudio, frame: number): ProcessedSource | null {
		const processed = audio.processed;
		if (processed === undefined) {
			this.items.delete(audio.item);
			return null;
		}
		const key = processed.key;
		this.items.set(audio.item, key);
		const entry = this.entry(key);
		const projectId = this.deps.projectId();
		if (entry.full.phase === "ready") {
			return { url: audioPreviewUrl(projectId, key), zero: processed.start };
		}
		const hit = entry.windows.find(
			(w) => w.phase === "ready" && frame >= w.first && frame <= w.last,
		);
		this.ensureWindow(key, entry, frame, processed.start, processed.frames);
		this.ensureFull(key, entry);
		if (hit === undefined) return null;
		return { url: audioPreviewUrl(projectId, key, hit.from), zero: hit.first };
	}

	// "processing" while an item with a processed sound plays unprocessed and
	// its file is being made; "ready" once it plays a file; failed files and
	// items with nothing to process are absent.
	states(): Record<string, ProcessedState> {
		const out: Record<string, ProcessedState> = {};
		for (const [item, key] of this.items) {
			const entry = this.entries.get(key);
			if (entry === undefined) continue;
			if (
				entry.full.phase === "ready" ||
				entry.windows.some((w) => w.phase === "ready")
			) {
				out[item] = "ready";
			} else if (
				entry.full.phase === "polling" ||
				entry.windows.some((w) => w.phase === "polling")
			) {
				out[item] = "processing";
			}
		}
		return out;
	}

	dispose(): void {
		this.disposed = true;
		for (const handle of this.handles) this.deps.timers.clearTimeout(handle);
		this.handles.clear();
		this.entries.clear();
		this.items.clear();
	}

	private entry(key: string): Entry {
		let entry = this.entries.get(key);
		if (entry === undefined) {
			entry = { full: { phase: "idle", retryAt: 0 }, windows: [] };
			this.entries.set(key, entry);
		}
		return entry;
	}

	private canStart(slot: Slot): boolean {
		return (
			slot.phase === "idle" ||
			(slot.phase === "failed" && this.deps.now() >= slot.retryAt)
		);
	}

	// One window at a time per key: the one that starts at the playhead.
	private ensureWindow(
		key: string,
		entry: Entry,
		frame: number,
		start: number,
		frames: number,
	): void {
		if (entry.windows.some((w) => w.phase === "polling")) return;
		const covered = entry.windows.some(
			(w) => w.phase !== "failed" && frame >= w.first && frame <= w.last,
		);
		if (covered) return;
		const from = Math.min(Math.max(frame, start), start + frames - 1);
		const failed = entry.windows.find(
			(w) => w.phase === "failed" && w.from === from,
		);
		if (failed !== undefined && this.deps.now() < failed.retryAt) return;
		if (entry.windows.length >= MAX_WINDOWS_PER_KEY) entry.windows.shift();
		const slot: WindowSlot = {
			phase: "polling",
			retryAt: 0,
			from,
			first: from,
			last: from,
		};
		entry.windows.push(slot);
		this.poll(
			audioPreviewUrl(this.deps.projectId(), key, from),
			WINDOW_POLLS,
			(result) => {
				if (result.kind === "ready") {
					slot.phase = "ready";
					if (result.window !== null) {
						slot.first = result.window[0];
						slot.last = result.window[1];
					}
				} else {
					slot.phase = "failed";
					slot.retryAt = this.deps.now() + RETRY_AFTER_FAILURE_MS;
				}
			},
		);
	}

	// The whole file is asked for once a window has answered, so the render of
	// the window goes first (the server also goes on to the whole item by itself).
	private ensureFull(key: string, entry: Entry): void {
		if (!this.canStart(entry.full)) return;
		if (entry.windows.some((w) => w.phase === "polling")) return;
		entry.full.phase = "polling";
		this.poll(audioPreviewUrl(this.deps.projectId(), key), FULL_POLLS, (r) => {
			if (r.kind === "ready") {
				entry.full.phase = "ready";
			} else {
				entry.full.phase = "failed";
				entry.full.retryAt = this.deps.now() + RETRY_AFTER_FAILURE_MS;
			}
		});
	}

	private poll(
		url: string,
		attempts: number,
		done: (result: ProbeResult) => void,
	): void {
		const attempt = (left: number): void => {
			void this.probe(url).then((result) => {
				if (this.disposed) return;
				if (result.kind === "processing" && left > 1) {
					const handle = this.deps.timers.setTimeout(() => {
						this.handles.delete(handle);
						attempt(left - 1);
					}, POLL_MS);
					this.handles.add(handle);
					return;
				}
				done(result.kind === "processing" ? { kind: "failed" } : result);
				this.deps.onChange();
			});
		};
		attempt(attempts);
	}
}

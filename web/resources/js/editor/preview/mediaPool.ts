export const RELEASE_AFTER_MS = 5000;

export interface PoolTimers {
	setTimeout(fn: () => void, ms: number): unknown;
	clearTimeout(handle: unknown): void;
}

export interface PooledMedia {
	element: HTMLMediaElement;
	gain: GainNode | null;
}

interface Entry extends PooledMedia {
	release: unknown;
}

// Pooled media elements on the proxy, keyed by asset id for video and by item
// id for audio (SDD-012 section 11.2 items 3 and 4). Each element feeds a
// GainNode so a gain above 1 works; HTMLMediaElement.volume cannot exceed 1.
// An element that left the window is paused and, after 5 s, released: `src`
// removed and `load()` called, to free the decoder.
export class MediaPool {
	private readonly entries = new Map<string, Entry>();

	constructor(
		private readonly kind: "video" | "audio",
		private readonly context: () => AudioContext | null,
		private readonly timers: PoolTimers,
		private readonly onSeeked: () => void,
	) {}

	get size(): number {
		return this.entries.size;
	}

	acquire(key: string, url: string): PooledMedia {
		let entry = this.entries.get(key);
		if (entry === undefined) {
			entry = { element: this.create(url), gain: null, release: null };
			this.connect(entry);
			this.entries.set(key, entry);
		} else if (entry.element.getAttribute("src") !== url) {
			entry.element.src = url;
		}
		if (entry.release !== null) {
			this.timers.clearTimeout(entry.release);
			entry.release = null;
		}
		return entry;
	}

	// Called for every element the window no longer uses.
	idle(inUse: ReadonlySet<string>): void {
		for (const [key, entry] of this.entries) {
			if (inUse.has(key)) continue;
			entry.element.pause();
			if (entry.release !== null) continue;
			entry.release = this.timers.setTimeout(
				() => this.drop(key),
				RELEASE_AFTER_MS,
			);
		}
	}

	pauseAll(): void {
		for (const entry of this.entries.values()) entry.element.pause();
	}

	dispose(): void {
		for (const key of [...this.entries.keys()]) this.drop(key);
	}

	droppedFrames(): number {
		let dropped = 0;
		for (const { element } of this.entries.values()) {
			if (element instanceof HTMLVideoElement) {
				dropped += element.getVideoPlaybackQuality().droppedVideoFrames;
			}
		}
		return dropped;
	}

	private create(url: string): HTMLMediaElement {
		const element = document.createElement(this.kind);
		element.preload = "auto";
		if (element instanceof HTMLVideoElement) element.playsInline = true;
		element.addEventListener("seeked", this.onSeeked);
		// A paused element at its first frame never seeks: its data arriving is the
		// moment to draw it.
		element.addEventListener("loadeddata", this.onSeeked);
		element.src = url;
		return element;
	}

	private connect(entry: Entry): void {
		const context = this.context();
		if (context === null) return;
		const gain = context.createGain();
		context.createMediaElementSource(entry.element).connect(gain);
		gain.connect(context.destination);
		entry.gain = gain;
	}

	private drop(key: string): void {
		const entry = this.entries.get(key);
		if (entry === undefined) return;
		if (entry.release !== null) this.timers.clearTimeout(entry.release);
		entry.element.pause();
		entry.element.removeEventListener("seeked", this.onSeeked);
		entry.element.removeEventListener("loadeddata", this.onSeeked);
		entry.element.removeAttribute("src");
		entry.element.load();
		entry.gain?.disconnect();
		this.entries.delete(key);
	}
}

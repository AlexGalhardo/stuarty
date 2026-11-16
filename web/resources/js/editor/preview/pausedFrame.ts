export const TRUTH_DELAY_MS = 150;

// The end of the timeline has no picture: the core refuses it (E_INVALID_TIME).
export function clampToLastFrame(frame: number, end: number): number {
	return Math.max(Math.min(frame, end - 1), 0);
}

export interface PausedFrameDeps {
	setTimeout(fn: () => void, ms: number): unknown;
	clearTimeout(handle: unknown): void;
	// Loads and decodes the image; rejects on any failure.
	load(url: string): Promise<void>;
	show(url: string): void;
	hide(): void;
	// The core's exact frame for these parameters (api/urls frameUrl).
	urlFor(frame: number, width: number, revision: number): string;
	mark(name: string): void;
	measure(name: string, start: string, end: string): void;
	onFailure?(error: unknown): void;
}

// Truth on pause (SDD-012 section 11.3, SDD-004 section 10 item 6): 150 ms
// after playback stops or a seek ends, the core's frame replaces the canvas.
// Any change of playhead, play start or revision removes it at once.
export class PausedFrame {
	private timer: unknown = null;
	private token = 0;
	private visible = false;

	constructor(private readonly deps: PausedFrameDeps) {}

	get shown(): boolean {
		return this.visible;
	}

	request(frame: number, width: number, revision: number): void {
		this.cancel();
		const token = this.token;
		this.deps.mark("vv:pause");
		this.timer = this.deps.setTimeout(() => {
			this.timer = null;
			const url = this.deps.urlFor(frame, width, revision);
			this.deps.load(url).then(
				() => {
					if (token !== this.token) return;
					this.visible = true;
					this.deps.show(url);
					this.deps.mark("vv:truth-shown");
					this.deps.measure("vv:truth", "vv:pause", "vv:truth-shown");
				},
				(error: unknown) => {
					// The canvas stays: it is a valid approximation, so no toast.
					if (token === this.token) this.deps.onFailure?.(error);
				},
			);
		}, TRUTH_DELAY_MS);
	}

	cancel(): void {
		this.token++;
		if (this.timer !== null) {
			this.deps.clearTimeout(this.timer);
			this.timer = null;
		}
		if (this.visible) {
			this.visible = false;
			this.deps.hide();
		}
	}
}

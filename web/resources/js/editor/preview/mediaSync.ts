import type { PlanLayer, PlanMediaLayer } from "../api/types";
import type { FrameRate } from "../state/selectors";

export const MAX_VIDEO_LAYERS = 4;
const MIN_RATE = 0.25;
const MAX_RATE = 4;
export const DRIFT_FRAMES = 2;

export function frameSeconds(fps: FrameRate): number {
	return fps.den / fps.num;
}

// The slope of `source_seconds` over the window, clamped to 0.25 to 4 (SDD-012
// section 11.2 item 3).
export function playbackRateFor(
	sourceSeconds: readonly number[],
	fps: FrameRate,
): number {
	if (sourceSeconds.length < 2) return 1;
	const first = sourceSeconds[0] ?? 0;
	const last = sourceSeconds[sourceSeconds.length - 1] ?? 0;
	const slope =
		(last - first) / ((sourceSeconds.length - 1) * frameSeconds(fps));
	if (!Number.isFinite(slope) || slope <= 0) return 1;
	return Math.min(Math.max(slope, MIN_RATE), MAX_RATE);
}

// During playback the element's `currentTime` is corrected only when it drifts
// by more than 2 frames from the plan (SDD-004 section 10 item 2).
export function needsCorrection(
	currentTime: number,
	target: number,
	fps: FrameRate,
): boolean {
	return Math.abs(currentTime - target) > DRIFT_FRAMES * frameSeconds(fps);
}

// The video layers that get a decoding element: the topmost `max` in plan
// order (the last drawn is on top). The others show the asset's thumbnail.
export function rankVideoLayers(
	layers: readonly PlanLayer[],
	max: number = MAX_VIDEO_LAYERS,
): Set<string> {
	const videos = layers.filter(
		(layer): layer is PlanMediaLayer =>
			layer.kind === "media" && layer.asset_kind === "video",
	);
	return new Set(videos.slice(-max).map((layer) => layer.item));
}

// The master clock during playback is the audio clock (SDD-004 section 10
// item 4): frame = startFrame + floor((now - t0) * num / den).
export class PlaybackClock {
	private t0 = 0;
	private startFrame = 0;

	constructor(
		private readonly now: () => number,
		private fps: FrameRate,
	) {}

	setRate(fps: FrameRate): void {
		this.fps = fps;
	}

	start(frame: number): void {
		this.startFrame = frame;
		this.t0 = this.now();
	}

	frame(): number {
		const elapsed = this.now() - this.t0;
		return (
			this.startFrame + Math.floor((elapsed * this.fps.num) / this.fps.den)
		);
	}
}

import type { PlanLayer, PlanMediaLayer, PlanSpeedRamp } from "../api/types";
import type { FrameRate } from "../state/selectors";

export const MAX_VIDEO_LAYERS = 4;
const MIN_RATE = 0.25;
const MAX_RATE = 4;
export const DRIFT_FRAMES = 2;

export function frameSeconds(fps: FrameRate): number {
	return fps.den / fps.num;
}

// The time in the proxy for `frame` of an audio entry of the plan, whose
// `source_seconds` is given at its first frame (SDD-004 section 10).
export function audioSourceSeconds(
	audio: { first: number; source_seconds: number; speed: number },
	frame: number,
	fps: FrameRate,
): number {
	return (
		audio.source_seconds +
		(frame - audio.first) * frameSeconds(fps) * audio.speed
	);
}

// The speed of a ramp at `u`, a fraction of the item: the linear interpolation
// of the neighbouring points (SDD-004 section 3.11).
export function curveSpeedAt(
	points: PlanSpeedRamp["points"],
	u: number,
): number {
	const value = (i: number): number => {
		const speed = points[i]?.speed;
		return speed === undefined ? 1 : speed.num / speed.den;
	};
	for (let i = 0; i + 1 < points.length; i++) {
		const t0 = points[i]?.t ?? 0;
		const t1 = points[i + 1]?.t ?? 1;
		if (u <= t1 || i + 2 === points.length) {
			const f = t1 === t0 ? 0 : Math.min(Math.max((u - t0) / (t1 - t0), 0), 1);
			return value(i) + (value(i + 1) - value(i)) * f;
		}
	}
	return 1;
}

// The source frames read before item frame `r`: `D x integral of the speed from
// 0 to r/D`, the formula of SDD-004 section 3.11 in floating point (the preview
// only needs it to place an audio element, not to the sample).
export function curveOffset(ramp: PlanSpeedRamp, r: number): number {
	const d = ramp.item_frames;
	const u = Math.min(Math.max(r / d, 0), 1);
	let total = 0;
	for (let i = 0; i + 1 < ramp.points.length; i++) {
		const t0 = ramp.points[i]?.t ?? 0;
		const t1 = ramp.points[i + 1]?.t ?? 1;
		if (u <= t0) break;
		const end = Math.min(u, t1);
		total +=
			((end - t0) *
				(curveSpeedAt(ramp.points, t0) + curveSpeedAt(ramp.points, end))) /
			2;
	}
	return total * d;
}

// The preview audio: where the proxy element should be at `frame` and how fast
// it should play. A reversed item cannot be played by a media element, so it is
// left silent in the preview (the export plays it).
export interface AudioDrive {
	seconds: number;
	rate: number;
	silent: boolean;
}

export function audioDrive(
	audio: {
		first: number;
		source_seconds: number;
		speed: number;
		speed_curve?: PlanSpeedRamp;
		reverse?: boolean;
	},
	frame: number,
	fps: FrameRate,
): AudioDrive {
	const ramp = audio.speed_curve;
	if (ramp === undefined) {
		return {
			seconds: audioSourceSeconds(audio, frame, fps),
			rate: audio.speed,
			silent: audio.reverse === true,
		};
	}
	const r = ramp.first_frame + (frame - audio.first);
	const frames = curveOffset(ramp, r) - curveOffset(ramp, ramp.first_frame);
	return {
		seconds: audio.source_seconds + frames * frameSeconds(fps),
		rate: Math.min(
			Math.max(
				curveSpeedAt(ramp.points, (r + 0.5) / ramp.item_frames),
				MIN_RATE,
			),
			MAX_RATE,
		),
		silent: audio.reverse === true,
	};
}

// The processed file of an item has its speed in it and starts at the frame
// `zero` of the timeline: it plays at rate 1 and is never silent, a reversed item
// included (SDD-004 section 7.1).
export function processedDrive(
	frame: number,
	zero: number,
	fps: FrameRate,
): AudioDrive {
	return {
		seconds: Math.max(frame - zero, 0) * frameSeconds(fps),
		rate: 1,
		silent: false,
	};
}

// How the video element follows a layer while playing: at a rate, or, when the
// source runs backwards (a reversed item), by a seek at every frame, because a
// media element cannot play backwards. The rate is the slope of the next
// `LOOKAHEAD_FRAMES` of `source_seconds`, so a curve is followed as it ramps.
export const LOOKAHEAD_FRAMES = 8;

export type VideoDrive = { seek: true } | { seek: false; rate: number };

export function videoDrive(
	sourceSeconds: readonly number[],
	at: number,
	fps: FrameRate,
): VideoDrive {
	const last = Math.min(at + LOOKAHEAD_FRAMES, sourceSeconds.length - 1);
	// At the end of the window the slope of the last frames is the best guess.
	const from = Math.max(Math.min(at, last - 1), 0);
	if (last <= from) return { seek: false, rate: 1 };
	const slope =
		((sourceSeconds[last] ?? 0) - (sourceSeconds[from] ?? 0)) /
		((last - from) * frameSeconds(fps));
	if (!Number.isFinite(slope)) return { seek: false, rate: 1 };
	if (slope < 0) return { seek: true };
	return { seek: false, rate: Math.min(Math.max(slope, MIN_RATE), MAX_RATE) };
}

// While playing, React state follows the frame at most every `everyMs`, unless
// an overlay needs the exact frame; the timecode is written directly
// (SDD-012 section 19.2 rule 1).
export function shouldCommitFrame(
	playing: boolean,
	needsLive: boolean,
	now: number,
	last: number,
	everyMs: number,
): boolean {
	return !playing || needsLive || now - last >= everyMs;
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

// How long play() waits for the AudioContext to run. A browser with no audio
// output device (Firefox on a headless Linux machine) leaves resume() pending
// forever, and the preview must still play.
export const AUDIO_RESUME_TIMEOUT_MS = 500;

// Whether the AudioContext can be the master clock: it is running now or
// within `timeoutMs` of the resume request. Otherwise the preview falls back
// to the wall clock, and plays without sound.
export async function audioClockReady(
	context: Pick<AudioContext, "state" | "resume"> | null,
	timeoutMs: number = AUDIO_RESUME_TIMEOUT_MS,
): Promise<boolean> {
	if (context === null) return false;
	if (context.state === "running") return true;
	let timer: ReturnType<typeof setTimeout> | undefined;
	const gave = new Promise<boolean>((resolve) => {
		timer = setTimeout(() => resolve(false), timeoutMs);
	});
	const resumed = context.resume().then(
		() => context.state === "running",
		() => false,
	);
	try {
		return await Promise.race([resumed, gave]);
	} finally {
		clearTimeout(timer);
	}
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

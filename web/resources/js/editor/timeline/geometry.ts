import { LIMITS } from "@/generated/limits";

// Screen arithmetic of the timeline (SDD-012 sections 1.1 and 7.2): pointer,
// pixel and frame coordinates, and the display order of items. This file is on
// the allowlist of rule R5: it may do arithmetic on `start` and `duration`.
// Nothing here decides what an edit does to the timeline.

export const TRACK_HEADER_PX = 176;
export const RULER_HEIGHT_PX = 32;
export const VISUAL_ROW_PX = 64;
export const AUDIO_ROW_PX = 48;
export const TIMELINE_MAX_FRAME = LIMITS.max_frames;
// Empty scrollable space past the end of the project, in frames (SDD-012 7.2).
export const EXTENT_PADDING_FRAMES = 300;
// Pixels per frame; one frame is at least 32 px wide at the last level.
export const ZOOM_LEVELS = [
	0.03125, 0.0625, 0.125, 0.25, 0.5, 1, 2, 4, 8, 16, 32,
] as const;
export const MIN_ZOOM = ZOOM_LEVELS[0];
export const MAX_ZOOM = ZOOM_LEVELS[ZOOM_LEVELS.length - 1];
// Labels are at least this far apart (SDD-012 section 7.3).
export const MIN_LABEL_GAP_PX = 70;
const MIN_TICK_GAP_PX = 8;

export interface Fps {
	num: number;
	den: number;
}

// The schema default of `settings.fps` (SDD-001 section 3.1).
export const DEFAULT_FPS: Fps = { num: 30, den: 1 };

export function clamp(value: number, low: number, high: number): number {
	return Math.min(high, Math.max(low, value));
}

export function frameToContentX(frame: number, ppf: number): number {
	return frame * ppf;
}

export function clientXToFrame(
	clientX: number,
	rectLeft: number,
	scrollLeft: number,
	ppf: number,
): number {
	const frame = Math.round((clientX - rectLeft + scrollLeft) / ppf);
	return clamp(frame, 0, TIMELINE_MAX_FRAME);
}

export function deltaPxToFrames(dx: number, ppf: number): number {
	return Math.round(dx / ppf);
}

export function clampZoom(ppf: number): number {
	return clamp(ppf, MIN_ZOOM, MAX_ZOOM);
}

// The scroll position that keeps `anchorFrame` under the pointer after a zoom
// to `nextPpf`; `anchorViewportX` is the pointer offset inside the viewport.
export function zoomAround(
	anchorFrame: number,
	anchorViewportX: number,
	nextPpf: number,
): { scrollLeft: number } {
	return { scrollLeft: Math.max(0, anchorFrame * nextPpf - anchorViewportX) };
}

export function fitZoom(viewportPx: number, endFrame: number): number {
	if (endFrame <= 0) return 1;
	return clampZoom(viewportPx / (endFrame * 1.05));
}

export function nextZoom(current: number, direction: 1 | -1): number {
	if (direction === 1) {
		return ZOOM_LEVELS.find((level) => level > current) ?? MAX_ZOOM;
	}
	for (let i = ZOOM_LEVELS.length - 1; i >= 0; i--) {
		if (ZOOM_LEVELS[i] < current) return ZOOM_LEVELS[i];
	}
	return MIN_ZOOM;
}

// The width of the scrollable time axis in pixels.
export function extentPx(
	viewportPx: number,
	endFrame: number,
	ppf: number,
): number {
	return Math.max(viewportPx, (endFrame + EXTENT_PADDING_FRAMES) * ppf);
}

export function framesPerSecond(fps: Fps): number {
	return Math.max(1, Math.round(fps.num / fps.den));
}

const FRAME_STEPS = [1, 2, 5, 10] as const;
const SECOND_STEPS = [1, 2, 5, 10, 15, 30, 60, 120, 300, 600] as const;

export interface RulerStep {
	labelEvery: number;
	tickEvery: number;
	format: "m:ss" | "s+ff";
}

// Candidate steps in frames: 1, 2, 5, 10 frames, then 1, 2, 5, 10, 15, 30, 60,
// 120, 300 and 600 seconds at the project rate (SDD-012 section 7.3).
export function rulerStep(ppf: number, fps: Fps): RulerStep {
	const perSecond = framesPerSecond(fps);
	const steps: number[] = [
		...FRAME_STEPS,
		...SECOND_STEPS.map((seconds) => Math.round((seconds * fps.num) / fps.den)),
	];
	const labelEvery =
		steps.find((step) => step * ppf >= MIN_LABEL_GAP_PX) ??
		steps[steps.length - 1];
	let tickEvery = labelEvery;
	for (const step of steps) {
		if (step >= labelEvery) break;
		if (step * ppf >= MIN_TICK_GAP_PX && labelEvery % step === 0) {
			tickEvery = step;
			break;
		}
	}
	return {
		labelEvery,
		tickEvery,
		format: labelEvery >= perSecond ? "m:ss" : "s+ff",
	};
}

const pad2 = (n: number): string => String(n).padStart(2, "0");

export function formatRulerLabel(
	frame: number,
	fps: Fps,
	format: RulerStep["format"],
): string {
	const perSecond = framesPerSecond(fps);
	const whole = Math.floor(frame / perSecond);
	if (format === "s+ff") return `${whole}+${pad2(frame % perSecond)}`;
	return `${Math.floor(whole / 60)}:${pad2(whole % 60)}`;
}

export { formatTimecode } from "../ui/timecode";

export interface Placed {
	id: string;
	start: number;
	duration: number;
}

// Items of one track in display order: by start, then by id. The core
// guarantees no overlap on a track; this only orders the blocks.
export function sortByStart<T extends Placed>(items: readonly T[]): T[] {
	return [...items].sort((a, b) => a.start - b.start || (a.id < b.id ? -1 : 1));
}

// The id of the item whose start is nearest to `start`, the first one on a tie
// (keyboard focus moving to the track above or below, SDD-012 section 7.10).
export function nearestByStart(
	items: readonly Placed[],
	start: number,
): string | null {
	let best: Placed | null = null;
	for (const item of items) {
		if (
			best === null ||
			Math.abs(item.start - start) < Math.abs(best.start - start)
		) {
			best = item;
		}
	}
	return best === null ? null : best.id;
}

export function clampPlayhead(frame: number): number {
	return clamp(Math.round(frame), 0, TIMELINE_MAX_FRAME);
}

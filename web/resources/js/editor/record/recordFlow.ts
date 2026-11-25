import { ApiError } from "../api/errors";
import type { Recording, RecordingSourcesResult } from "../api/types";
import type { Capability } from "./browserCapture";
import type { RecorderState } from "./browserRecorder";

// What the record dialog shows. Every member comes from the service: the state of a recording, whether a person
// added it, the elapsed time and the reason a capture is not available are the service's (SDD-025). The dialog
// computes none of them; this file only chooses which screen draws what the service said.
export type RecordView =
	| { step: "loading" }
	| { step: "unavailable"; reason: string }
	// kept: the recordings a person set aside with "Not now", which only a person can add or delete later
	| { step: "pick"; sources: RecordingSourcesResult; kept: Recording[] }
	| { step: "recording"; recording: Recording }
	// where native capture does not exist the browser records and uploads (SDD-025 section 18): what this browser can
	// do, read before the person starts, and the takes set aside
	| { step: "browser"; capability: Capability; kept: Recording[] }
	// a recording this tab is making in the browser: the state is the recorder's, until the server answers the finish
	| { step: "browser_live"; state: RecorderState }
	// a browser recording whose tab is gone: it can be finished with the chunks the server has, or deleted
	| { step: "interrupted"; recording: Recording }
	| { step: "review"; recording: Recording }
	| { step: "added"; recording: Recording }
	| { step: "failed"; recording: Recording }
	| { step: "kept" }
	| { step: "gone" };

export function viewOf(recording: Recording): RecordView {
	switch (recording.state) {
		case "recording":
			// a browser recording that receives chunks and is not the one this tab is making has lost its tab
			return recording.source.kind === "browser"
				? { step: "interrupted", recording }
				: { step: "recording", recording };
		case "prepared":
		case "paused":
		case "finishing":
			return { step: "recording", recording };
		case "ready":
			return recording.review === "added"
				? { step: "added", recording }
				: { step: "review", recording };
		case "failed":
			return { step: "failed", recording };
		default:
			return { step: "gone" };
	}
}

// The screen of a recording this tab makes in the browser. Once it has ended, whoever ended it (the Stop of the
// person, Stop sharing in the browser, a limit of the server, an upload that failed), the screen is the one of the
// server's answer, which the dialog then follows like any recording.
export function browserViewOf(state: RecorderState): RecordView {
	if (state.phase !== "ended") return { step: "browser_live", state };
	return state.recording === null
		? { step: "loading" }
		: viewOf(state.recording);
}

// The recording is still changing: the dialog asks again.
export function isLive(recording: Recording): boolean {
	return (
		recording.state === "prepared" ||
		recording.state === "recording" ||
		recording.state === "paused" ||
		recording.state === "finishing"
	);
}

// A recording a person set aside with "Not now" (SDD-025 section 4): the service shows it to a person only.
export function isKept(recording: Recording): boolean {
	return recording.state === "ready" && recording.review === "dismissed";
}

// The reason the service gave when capture is not available here (E_CAPTURE_UNAVAILABLE, details.reason), or null
// for any other error.
export function unavailableReason(error: unknown): string | null {
	if (!(error instanceof ApiError) || error.code !== "E_CAPTURE_UNAVAILABLE") {
		return null;
	}
	const reason = error.details?.reason;
	return typeof reason === "string" ? reason : "platform";
}

// Whether the browser recorder is offered when native capture answered with this reason: every one but a missing
// FFmpeg, which the server needs to assemble the file too.
export function browserOffered(reason: string): boolean {
	return reason !== "ffmpeg_missing";
}

const KNOWN_REASONS = [
	"platform",
	"web",
	"network_mode",
	"ffmpeg_missing",
	"helper_missing",
	"protocol_version",
] as const;

// The key of the sentence for a reason; a reason this build does not know falls back to the general one.
export function unavailableKey(reason: string): string {
	return (KNOWN_REASONS as readonly string[]).includes(reason)
		? `ui.editor.record.unavailable.${reason}`
		: "ui.editor.record.unavailable.platform";
}

// The time the service reported, as minutes and seconds for the eye.
export function formatClock(ms: number): string {
	const total = Math.max(0, Math.floor(ms / 1000));
	const minutes = Math.floor(total / 60);
	const seconds = total % 60;
	return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

export const FPS_CHOICES = [24, 30, 60] as const;
export const DEFAULT_FPS = 30;
// How often the dialog asks for a recording that runs: the level meters are drawn from the same answer, so it is
// asked for several times a second.
export const LIVE_POLL_MS = 300;

// What the person chose besides the screen: the ids come from the sources the service listed.
export interface StartOptions {
	microphone: string;
	systemAudio: boolean;
	camera: string;
	events: boolean;
}

export const NO_EXTRAS: StartOptions = {
	microphone: "",
	systemAudio: false,
	camera: "",
	events: true,
};

// The bytes the server confirmed, in megabytes with one decimal, for the eye.
export function formatMegabytes(bytes: number): string {
	return (Math.max(0, bytes) / (1024 * 1024)).toFixed(1);
}

// The peak the service reported, 0 to 1000, kept inside the range of the meter that draws it.
export function meterValue(level: number | undefined): number {
	return Math.min(1000, Math.max(0, level ?? 0));
}

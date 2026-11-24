import { ApiError } from "../api/errors";
import type { Recording, RecordingSourcesResult } from "../api/types";

// What the record dialog shows. Every member comes from the service: the state of a recording, whether a person
// added it, the elapsed time and the reason a capture is not available are the service's (SDD-023). The dialog
// computes none of them; this file only chooses which screen draws what the service said.
export type RecordView =
	| { step: "loading" }
	| { step: "unavailable"; reason: string }
	| { step: "pick"; sources: RecordingSourcesResult }
	| { step: "recording"; recording: Recording }
	| { step: "review"; recording: Recording }
	| { step: "added"; recording: Recording }
	| { step: "failed"; recording: Recording }
	| { step: "gone" };

export function viewOf(recording: Recording): RecordView {
	switch (recording.state) {
		case "prepared":
		case "recording":
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

// The recording is still changing: the dialog asks again.
export function isLive(recording: Recording): boolean {
	return (
		recording.state === "prepared" ||
		recording.state === "recording" ||
		recording.state === "finishing"
	);
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
export const POLL_MS = 1000;

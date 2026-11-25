import type { RecordingContainer } from "../api/types";

// What this browser can capture and record, read before the person starts (SDD-025 section 18.1). The browser
// APIs exist only in a secure context, and each browser records its own codecs: the dialog says so up front
// instead of losing a take. This file decides nothing about a recording; the server decides every rule.

// What the check reads from the browser, as plain members so a test can supply its own.
export interface CaptureEnv {
	isSecureContext: boolean;
	mediaDevices: {
		getDisplayMedia?: unknown;
		getUserMedia?: unknown;
	} | null;
	mediaRecorder: { isTypeSupported(type: string): boolean } | null;
	// the digest that every chunk carries
	hasDigest: boolean;
}

// The reason recording is disabled, as the key of the sentence the dialog shows.
export type CaptureBlocked =
	| "insecure_context"
	| "no_display_capture"
	| "no_media_recorder"
	| "no_codec"
	| "no_digest";

export type Capability =
	| {
			ok: true;
			// the type MediaRecorder is asked for, and the container the server is told
			mimeType: string;
			container: RecordingContainer;
			// a short name of the codecs, for the sentence that says what this browser records
			label: "vp9" | "vp8" | "av1" | "h264" | "default-webm" | "default-mp4";
			camera: boolean;
			microphone: boolean;
	  }
	| { ok: false; reason: CaptureBlocked };

interface Candidate {
	mimeType: string;
	container: RecordingContainer;
	label: Extract<Capability, { ok: true }>["label"];
}

// In order of preference: the open codecs first, then H.264 in MP4 (what Safari records), then whatever the
// browser calls WebM or MP4. The first one `isTypeSupported` accepts is used.
export const CANDIDATES: readonly Candidate[] = [
	{ mimeType: "video/webm;codecs=vp9,opus", container: "webm", label: "vp9" },
	{ mimeType: "video/webm;codecs=vp8,opus", container: "webm", label: "vp8" },
	{ mimeType: "video/webm;codecs=av1,opus", container: "webm", label: "av1" },
	{
		mimeType: "video/mp4;codecs=avc1.42E01E,mp4a.40.2",
		container: "mp4",
		label: "h264",
	},
	{ mimeType: "video/webm", container: "webm", label: "default-webm" },
	{ mimeType: "video/mp4", container: "mp4", label: "default-mp4" },
];

// The environment of the page. A page on plain HTTP outside localhost has no `mediaDevices` at all, which is why
// the secure context is reported first.
export function readEnv(): CaptureEnv {
	const g = globalThis as {
		isSecureContext?: boolean;
		navigator?: { mediaDevices?: CaptureEnv["mediaDevices"] };
		MediaRecorder?: { isTypeSupported(type: string): boolean };
		crypto?: { subtle?: unknown };
	};
	return {
		isSecureContext: g.isSecureContext === true,
		mediaDevices: g.navigator?.mediaDevices ?? null,
		mediaRecorder: g.MediaRecorder ?? null,
		hasDigest: typeof g.crypto?.subtle === "object" && g.crypto.subtle !== null,
	};
}

export function capabilityOf(env: CaptureEnv): Capability {
	if (!env.isSecureContext) return { ok: false, reason: "insecure_context" };
	if (
		env.mediaDevices === null ||
		typeof env.mediaDevices.getDisplayMedia !== "function"
	) {
		return { ok: false, reason: "no_display_capture" };
	}
	if (env.mediaRecorder === null) {
		return { ok: false, reason: "no_media_recorder" };
	}
	if (!env.hasDigest) return { ok: false, reason: "no_digest" };
	const recorder = env.mediaRecorder;
	const found = CANDIDATES.find((c) => recorder.isTypeSupported(c.mimeType));
	if (found === undefined) return { ok: false, reason: "no_codec" };
	return {
		ok: true,
		mimeType: found.mimeType,
		container: found.container,
		label: found.label,
		camera: typeof env.mediaDevices.getUserMedia === "function",
		microphone: typeof env.mediaDevices.getUserMedia === "function",
	};
}

export function currentCapability(): Capability {
	return capabilityOf(readEnv());
}

// The keys of the sentences of the dialog (ui.editor.record.browser.*).
export function blockedKey(reason: CaptureBlocked): string {
	return `ui.editor.record.browser.blocked.${reason}`;
}

export function codecKey(
	label: Extract<Capability, { ok: true }>["label"],
): string {
	return `ui.editor.record.browser.codec.${label}`;
}

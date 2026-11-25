import { describe, expect, test } from "vitest";

import {
	blockedKey,
	CANDIDATES,
	type CaptureEnv,
	capabilityOf,
	readEnv,
} from "./browserCapture";

const good = (types: string[]): CaptureEnv => ({
	isSecureContext: true,
	mediaDevices: { getDisplayMedia: () => {}, getUserMedia: () => {} },
	mediaRecorder: { isTypeSupported: (t) => types.includes(t) },
	hasDigest: true,
});

describe("what this browser can record, read before the person starts", () => {
	test("a page that is not a secure context cannot record, whatever else is there", () => {
		expect(
			capabilityOf({ ...good(["video/webm"]), isSecureContext: false }),
		).toEqual({ ok: false, reason: "insecure_context" });
		// plain HTTP outside localhost has no mediaDevices at all: the reason is still the context
		expect(
			capabilityOf({ ...good([]), isSecureContext: false, mediaDevices: null }),
		).toEqual({ ok: false, reason: "insecure_context" });
	});

	test("each missing piece is its own reason", () => {
		expect(
			capabilityOf({ ...good(["video/webm"]), mediaDevices: null }),
		).toEqual({ ok: false, reason: "no_display_capture" });
		expect(
			capabilityOf({
				...good(["video/webm"]),
				mediaDevices: { getUserMedia: () => {} },
			}),
		).toEqual({ ok: false, reason: "no_display_capture" });
		expect(capabilityOf({ ...good([]), mediaRecorder: null })).toEqual({
			ok: false,
			reason: "no_media_recorder",
		});
		expect(capabilityOf({ ...good(["video/webm"]), hasDigest: false })).toEqual(
			{ ok: false, reason: "no_digest" },
		);
		expect(capabilityOf(good([]))).toEqual({ ok: false, reason: "no_codec" });
	});

	test("the first type the browser supports wins, in the order of preference", () => {
		const every = CANDIDATES.map((c) => c.mimeType);
		expect(capabilityOf(good(every))).toMatchObject({
			ok: true,
			mimeType: "video/webm;codecs=vp9,opus",
			container: "webm",
			label: "vp9",
		});
		expect(
			capabilityOf(good(["video/webm;codecs=vp8,opus", "video/webm"])),
		).toMatchObject({ ok: true, container: "webm", label: "vp8" });
	});

	test("a browser that records only MP4 says so, so the server is told the right container", () => {
		expect(capabilityOf(good(["video/mp4"]))).toMatchObject({
			ok: true,
			mimeType: "video/mp4",
			container: "mp4",
			label: "default-mp4",
		});
	});

	test("the camera and the microphone need getUserMedia", () => {
		expect(
			capabilityOf({
				...good(["video/webm"]),
				mediaDevices: { getDisplayMedia: () => {} },
			}),
		).toMatchObject({ ok: true, camera: false, microphone: false });
		expect(capabilityOf(good(["video/webm"]))).toMatchObject({
			camera: true,
			microphone: true,
		});
	});

	test("the sentence of a reason is a key of the catalogue", () => {
		expect(blockedKey("insecure_context")).toBe(
			"ui.editor.record.browser.blocked.insecure_context",
		);
	});

	test("outside a browser every piece is missing, and nothing throws", () => {
		const env = readEnv();
		expect(env.isSecureContext).toBe(false);
		expect(capabilityOf(env).ok).toBe(false);
	});
});

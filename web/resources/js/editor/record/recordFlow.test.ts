import { describe, expect, test } from "vitest";

import { ApiError } from "../api/errors";
import type { Recording } from "../api/types";
import {
	formatClock,
	isKept,
	isLive,
	unavailableKey,
	unavailableReason,
	viewOf,
} from "./recordFlow";

function recording(
	state: Recording["state"],
	review?: Recording["review"],
): Recording {
	return {
		recording_id: "rec_0123456789",
		project_id: "prj_0123456789",
		state,
		...(review === undefined ? {} : { review }),
		source: { kind: "display" },
		fps: 30,
		duration_ms: 65_400,
		frames: 1962,
		dropped: 0,
		bytes: 1000,
		actor: "local:ui",
		created_at: "2026-11-24T00:00:00Z",
		text: "",
	};
}

describe("the screen the dialog draws for what the service said", () => {
	test("a recording that is not final is the recording screen, and is live", () => {
		for (const state of ["prepared", "recording", "finishing"] as const) {
			expect(viewOf(recording(state)).step).toBe("recording");
			expect(isLive(recording(state))).toBe(true);
		}
	});

	test("a ready one waits for the person until the service says it was added", () => {
		expect(viewOf(recording("ready", "pending")).step).toBe("review");
		expect(viewOf(recording("ready", "dismissed")).step).toBe("review");
		expect(viewOf(recording("ready", "added")).step).toBe("added");
		expect(isLive(recording("ready", "pending"))).toBe(false);
	});

	test("only a ready recording set aside with Not now is kept for the picker", () => {
		expect(isKept(recording("ready", "dismissed"))).toBe(true);
		expect(isKept(recording("ready", "pending"))).toBe(false);
		expect(isKept(recording("ready", "added"))).toBe(false);
		expect(isKept(recording("failed"))).toBe(false);
	});

	test("a failed one and a discarded one have their own screens", () => {
		expect(viewOf(recording("failed")).step).toBe("failed");
		expect(viewOf(recording("cancelled")).step).toBe("gone");
	});
});

describe("when capture is not available", () => {
	const unavailable = (reason?: string) =>
		new ApiError(503, {
			code: "E_CAPTURE_UNAVAILABLE",
			message: "not here",
			details: reason === undefined ? undefined : { reason },
		});

	test("takes the reason of the service from the error", () => {
		expect(unavailableReason(unavailable("web"))).toBe("web");
		expect(unavailableReason(unavailable())).toBe("platform");
	});

	test("is null for any other error", () => {
		expect(
			unavailableReason(
				new ApiError(403, { code: "E_PRIVILEGED_SESSION", message: "x" }),
			),
		).toBeNull();
		expect(unavailableReason(new Error("boom"))).toBeNull();
	});

	test("has a sentence for every reason and a general one for a reason it does not know", () => {
		for (const reason of [
			"platform",
			"web",
			"network_mode",
			"ffmpeg_missing",
			"helper_missing",
			"protocol_version",
		]) {
			expect(unavailableKey(reason)).toBe(
				`ui.editor.record.unavailable.${reason}`,
			);
		}
		expect(unavailableKey("something_new")).toBe(
			"ui.editor.record.unavailable.platform",
		);
	});
});

describe("the clock", () => {
	test("shows the time the service reported as minutes and seconds", () => {
		expect(formatClock(0)).toBe("0:00");
		expect(formatClock(65_400)).toBe("1:05");
		expect(formatClock(3_600_000)).toBe("60:00");
		expect(formatClock(-5)).toBe("0:00");
	});
});

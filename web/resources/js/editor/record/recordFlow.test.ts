import { describe, expect, test } from "vitest";

import { ApiError } from "../api/errors";
import type { Recording } from "../api/types";
import type { RecorderState } from "./browserRecorder";
import {
	browserViewOf,
	capturing,
	formatClock,
	formatClockTenths,
	isKept,
	isLive,
	isPrepared,
	meterValue,
	needsDecision,
	pendingProposals,
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
		for (const state of [
			"prepared",
			"recording",
			"paused",
			"finishing",
		] as const) {
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

	test("a meter keeps the peak the service reported inside its range", () => {
		expect(meterValue(undefined)).toBe(0);
		expect(meterValue(420)).toBe(420);
		expect(meterValue(-5)).toBe(0);
		expect(meterValue(5000)).toBe(1000);
	});

	test("a failed one and a discarded one have their own screens", () => {
		expect(viewOf(recording("failed")).step).toBe("failed");
		expect(viewOf(recording("cancelled")).step).toBe("gone");
	});
});

describe("a recording this tab makes in the browser", () => {
	function live(
		phase: RecorderState["phase"],
		rec: Recording | null,
		stoppedBy: RecorderState["stoppedBy"] = null,
	): RecorderState {
		return {
			phase,
			recordingId: "rec_0123456789",
			elapsedMs: 4000,
			shared: {
				screen: true,
				microphone: false,
				displayAudio: false,
				camera: false,
			},
			upload: {
				phase: "idle",
				pending: 0,
				chunks: { screen: 4, camera: 0 },
				bytes: 4000,
				end: null,
			},
			stoppedBy,
			recording: rec,
			error: null,
		};
	}

	test("is drawn from the recorder until it has ended", () => {
		for (const phase of [
			"starting",
			"recording",
			"sending",
			"finishing",
		] as const) {
			expect(browserViewOf(live(phase, null)).step).toBe("browser_live");
		}
	});

	test("once it ended on its own, the screen follows the server's answer instead of waiting forever", () => {
		for (const by of ["browser", "limit", "failure", "person"] as const) {
			const finishing = {
				...recording("finishing"),
				source: { kind: "browser" },
			};
			expect(browserViewOf(live("ended", finishing, by)).step).toBe(
				"recording",
			);
		}
		expect(
			browserViewOf(live("ended", recording("ready", "pending"), "browser"))
				.step,
		).toBe("review");
		expect(browserViewOf(live("ended", null, "failure")).step).toBe("loading");
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

describe("the plans and the proposals of agents (SDD-025 section 19)", () => {
	const plan: Recording = { ...recording("prepared"), plan: { fps: 30 } };
	const proposal = (state: "pending" | "accepted" | "rejected") => ({
		proposal_id: "prp_0123456789",
		state,
		actor: "local:claude-code",
		created_at: "2026-11-24T00:00:00Z",
		ranges: [{ from_ms: 0, to_ms: 500, reason: "dead air" }],
	});

	test("a prepared recording with a plan is not live and is not drawn as a recording that runs", () => {
		expect(isPrepared(plan)).toBe(true);
		expect(isLive(plan)).toBe(false);
		expect(viewOf(plan).step).toBe("gone");
		// a recording a helper is starting has no plan: it is live as before
		expect(isPrepared(recording("prepared"))).toBe(false);
		expect(isLive(recording("prepared"))).toBe(true);
	});

	test("only a pending proposal of a recording a person added waits for a decision", () => {
		const added = (state: "pending" | "accepted") => ({
			...recording("ready", "added"),
			proposals: [proposal(state)],
		});
		expect(pendingProposals(added("pending"))).toHaveLength(1);
		expect(needsDecision(added("pending"))).toBe(true);
		expect(needsDecision(added("accepted"))).toBe(false);
		expect(needsDecision(recording("ready", "added"))).toBe(false);
		expect(
			needsDecision({
				...recording("ready", "pending"),
				proposals: [proposal("pending")],
			}),
		).toBe(false);
	});

	test("the banner follows the recording that is capturing, running or paused, and nothing else", () => {
		expect(capturing([plan, recording("ready", "added")])).toBeNull();
		expect(capturing([recording("finishing")])).toBeNull();
		expect(capturing([plan, recording("recording")])?.state).toBe("recording");
		expect(capturing([recording("paused")])?.state).toBe("paused");
	});

	test("the time of a range has tenths", () => {
		expect(formatClockTenths(0)).toBe("0:00.0");
		expect(formatClockTenths(1234)).toBe("0:01.2");
		expect(formatClockTenths(61_999)).toBe("1:01.9");
		expect(formatClockTenths(-5)).toBe("0:00.0");
	});
});

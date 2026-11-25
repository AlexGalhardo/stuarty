import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import en from "../../../lang/en.json";
import type { Recording } from "../api/types";
import RecordPanel from "./RecordPanel";
import type { RecordView } from "./recordFlow";
import type { RecordFlow } from "./useRecordFlow";

function flat(
	tree: Record<string, unknown>,
	prefix: string,
	out: Record<string, string> = {},
): Record<string, string> {
	for (const [key, value] of Object.entries(tree)) {
		if (typeof value === "string") out[`${prefix}${key}`] = value;
		else flat(value as Record<string, unknown>, `${prefix}${key}.`, out);
	}
	return out;
}
const messages = flat(en.ui, "ui.");

const rec = (over: Partial<Recording>): Recording => ({
	recording_id: "rec_0123456789",
	project_id: "prj_0123456789",
	state: "recording",
	source: { kind: "display" },
	fps: 30,
	duration_ms: 65_000,
	frames: 0,
	dropped: 0,
	bytes: 0,
	actor: "local:ui",
	created_at: "2026-11-24T00:00:00Z",
	text: "",
	...over,
});

const noDevices = { microphones: [], cameras: [], system_audio: false };

function draw(view: RecordView, over: Partial<RecordFlow> = {}): string {
	const flow: RecordFlow = {
		view,
		failure: null,
		busy: false,
		placed: false,
		notice: null,
		startPrepared: async () => {},
		decide: async () => {},
		start: async () => {},
		startBrowser: async () => {},
		stopBrowser: async () => {},
		stop: async () => {},
		pause: async () => {},
		resume: async () => {},
		confirm: async () => {},
		dismiss: async () => {},
		remove: async () => {},
		place: async () => {},
		reset: () => {},
		...over,
	};
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<RecordPanel flow={flow} onClose={() => {}} />
		</I18nProvider>,
	);
}

describe("the record dialog", () => {
	test("lists the displays and the windows by name, with a frame rate and a start button", () => {
		const html = draw({
			step: "pick",
			sources: {
				displays: [{ id: "d1", name: "Display 1", width: 1920, height: 1080 }],
				windows: [{ id: "w9", name: "My notes", width: 800, height: 600 }],
				...noDevices,
			},
			kept: [],
		});
		expect(html).toContain("Display 1");
		expect(html).not.toContain("data-record-kept");
		expect(html).toContain("My notes");
		expect(html).toContain("Displays");
		expect(html).toContain('data-record-start=""');
		expect(html).toContain("Frames per second");
	});

	test("where capture is unavailable the browser records: what it can do is said before the person starts", () => {
		const html = draw({
			step: "browser",
			capability: {
				ok: true,
				mimeType: "video/webm;codecs=vp9,opus",
				container: "webm",
				label: "vp9",
				camera: true,
				microphone: true,
			},
			kept: [],
		});
		expect(html).toContain(
			"This browser records WebM with VP9 video and Opus sound.",
		);
		expect(html).toContain("asks your permission");
		expect(html).toContain(
			"Pointer and click positions are not recorded in a browser.",
		);
		expect(html).toContain('data-record-browser-microphone=""');
		expect(html).toContain('data-record-browser-camera=""');
		expect(html).toContain('data-record-start=""');
		expect(html).not.toContain("data-record-browser-blocked");
		expect(html).not.toContain('disabled=""');
	});

	test("on a page that is not a secure context recording is disabled and the reason is shown", () => {
		const html = draw({
			step: "browser",
			capability: { ok: false, reason: "insecure_context" },
			kept: [],
		});
		expect(html).toContain('data-record-browser-blocked="insecure_context"');
		expect(html).toContain("HTTPS or on localhost");
		expect(html).toMatch(/<button[^>]*disabled=""[^>]*data-record-start=""/);
		expect(html).not.toContain("<form");
	});

	test("each reason a browser cannot record has its own sentence", () => {
		for (const [reason, words] of [
			["no_display_capture", "cannot capture the screen"],
			["no_media_recorder", "cannot record what it captures"],
			["no_codec", "format Stuarty cannot take"],
			["no_digest", "cannot check the files it sends"],
		] as const) {
			expect(
				draw({ step: "browser", capability: { ok: false, reason }, kept: [] }),
			).toContain(words);
		}
	});

	test("a camera that the browser cannot give is not offered", () => {
		const html = draw({
			step: "browser",
			capability: {
				ok: true,
				mimeType: "video/mp4",
				container: "mp4",
				label: "default-mp4",
				camera: false,
				microphone: false,
			},
			kept: [],
		});
		expect(html).toContain(
			"cannot give the microphone or the camera to a page",
		);
		expect(html).toContain("This browser records MP4.");
	});

	test("a recording made in the browser shows its clock, what is shared and what the server holds", () => {
		const base = {
			recordingId: "rec_0123456789",
			elapsedMs: 65_000,
			shared: {
				screen: true,
				microphone: true,
				displayAudio: false,
				camera: false,
			},
			upload: {
				phase: "sending",
				pending: 0,
				chunks: { screen: 3, camera: 0 },
				bytes: 3 * 1024 * 1024,
				end: null,
			},
			stoppedBy: null,
			recording: null,
			error: null,
		} as const;
		const html = draw({
			step: "browser_live",
			state: { ...base, phase: "recording" },
		});
		expect(html).toContain("Elapsed 1:05");
		expect(html).toContain('data-record-browser-shared-item="microphone"');
		expect(html).not.toContain('data-record-browser-shared-item="camera"');
		expect(html).toContain("3.0 MB saved on your server");
		expect(html).toContain('data-record-stop=""');
		expect(html).not.toContain("data-record-browser-retrying");

		const retrying = draw({
			step: "browser_live",
			state: {
				...base,
				phase: "recording",
				upload: { ...base.upload, phase: "retrying" },
			},
		});
		expect(retrying).toContain(
			"The connection dropped. Trying again: nothing is lost.",
		);

		const limited = draw({
			step: "browser_live",
			state: {
				...base,
				phase: "sending",
				stoppedBy: "limit",
				upload: { ...base.upload, end: { kind: "limit", reason: "size" } },
			},
		});
		expect(limited).toContain("reached the size limit");
		expect(limited).toContain("Saving the file");
		expect(limited).toContain('disabled=""');

		const browser = draw({
			step: "browser_live",
			state: { ...base, phase: "sending", stoppedBy: "browser" },
		});
		expect(browser).toContain("You stopped sharing in the browser");
	});

	test("a take whose tab is gone can be finished with what the server holds, or deleted", () => {
		const html = draw({
			step: "interrupted",
			recording: rec({ source: { kind: "browser" }, bytes: 5 * 1024 * 1024 }),
		});
		expect(html).toContain("was interrupted");
		expect(html).toContain("5.0 MB saved on your server");
		expect(html).toContain('data-record-finish-interrupted=""');
		expect(html).toContain('data-record-delete=""');
	});

	test("shows a recording in progress with the clock of the service and a stop button", () => {
		const html = draw({ step: "recording", recording: rec({}) });
		expect(html).toContain("Recording");
		expect(html).toContain("Elapsed 1:05");
		expect(html).toContain('data-record-stop=""');
		expect(html).not.toContain('disabled=""');
	});

	test("cannot be stopped twice while the file is being saved", () => {
		const html = draw({
			step: "recording",
			recording: rec({ state: "finishing" }),
		});
		expect(html).toContain("Saving the file");
		expect(html).toContain('disabled=""');
	});

	test("asks the person, and says that agents with access to the project can read it", () => {
		const html = draw({
			step: "review",
			recording: rec({ state: "ready", review: "pending", dropped: 3 }),
		});
		expect(html).toContain(
			"Agents with access to this project will be able to read this recording.",
		);
		expect(html).toContain('data-record-add=""');
		expect(html).toContain('data-record-not-now=""');
		expect(html).toContain('data-record-delete=""');
		expect(html).toContain("1:05 recorded");
		expect(html).toContain("3 frames were repeated");
	});

	test("offers to place an added recording on the timeline, and says so once it is placed", () => {
		const view: RecordView = {
			step: "added",
			recording: rec({ state: "ready", review: "added" }),
		};
		expect(draw(view)).toContain('data-record-place=""');
		const placed = draw(view, { placed: true });
		expect(placed).toContain("Placed on the timeline.");
		expect(placed).not.toContain('data-record-place=""');
	});

	test("says why capture is not available, in the words of the reason", () => {
		for (const [reason, words] of [
			["web", "The browser cannot capture the screen"],
			["helper_missing", "capture helper is not installed"],
			["platform", "desktop app on Windows"],
		]) {
			expect(draw({ step: "unavailable", reason: reason ?? "" })).toContain(
				words ?? "",
			);
		}
	});

	test("shows the failure the service reported, and offers to try again", () => {
		const html = draw({
			step: "failed",
			recording: rec({
				state: "failed",
				error: { code: "E_CAPTURE_FAILED", message: "the encoder died" },
			}),
		});
		expect(html).toContain("The recording failed");
		expect(html).toContain("the encoder died");
		expect(html).toContain("Record again");
	});

	test("draws no markup of a window title", () => {
		const html = draw({
			step: "pick",
			sources: {
				displays: [],
				windows: [
					{
						id: "w1",
						name: "<img src=x onerror=alert(1)>",
						width: 1,
						height: 1,
					},
				],
				...noDevices,
			},
			kept: [],
		});
		expect(html).not.toContain("<img");
	});

	test("lists the recordings set aside with Not now, so a person can still add or delete them", () => {
		const html = draw({
			step: "pick",
			sources: { displays: [], windows: [], ...noDevices },
			kept: [
				rec({
					recording_id: "rec_000000000k",
					state: "ready",
					review: "dismissed",
				}),
			],
		});
		expect(html).toContain("Out of the project");
		expect(html).toContain("1:05 recorded");
		expect(html).toContain('data-record-kept-add="rec_000000000k"');
		expect(html).toContain('data-record-kept-delete="rec_000000000k"');
		expect(html).toContain(
			"Agents with access to this project will be able to read this recording.",
		);
	});

	test("lists a prepared recording with its notes as text and a Record button, and starts nothing by itself", () => {
		const html = draw({
			step: "pick",
			sources: { displays: [], windows: [], ...noDevices },
			kept: [],
			prepared: [
				rec({
					recording_id: "rec_000000000p",
					state: "prepared",
					actor: "local:claude-code",
					source: { kind: "display", name: "Main <b>screen</b>" },
					plan: {
						source_id: "d1",
						fps: 30,
						notes: "Say the hook first <img src=x>",
					},
				}),
			],
		});
		expect(html).toContain("Ready to record");
		expect(html).toContain("Nothing records until you press Record.");
		expect(html).toContain('data-record-prepared-start="rec_000000000p"');
		expect(html).toContain('data-record-prepared-delete="rec_000000000p"');
		expect(html).toContain("Prepared by local:claude-code");
		expect(html).toContain("Say the hook first");
		expect(html).not.toContain("<img");
		expect(html).not.toContain("<b>");
	});

	test("names the source of a plan by the title the helper gives it and lists every track it records", () => {
		const html = draw({
			step: "pick",
			sources: {
				displays: [{ id: "d1", name: "Display 1", width: 1920, height: 1080 }],
				windows: [{ id: "w9", name: "Slides", width: 800, height: 600 }],
				...noDevices,
			},
			kept: [],
			prepared: [
				rec({
					recording_id: "rec_000000000p",
					state: "prepared",
					source: { kind: "display", name: "Only the slides window" },
					plan: {
						source_id: "d1",
						fps: 30,
						microphone: "default",
						camera: "c0123456789abcdef",
					},
				}),
			],
		});
		expect(html).toContain("Display 1");
		expect(html).not.toContain("Only the slides window");
		expect(html).toContain("data-record-prepared-tracks");
		expect(html).toContain("Microphone");
		expect(html).toContain("Camera");
	});

	test("shows every range of a pending proposal with its own accept and reject, and one apply", () => {
		const html = draw({
			step: "pick",
			sources: { displays: [], windows: [], ...noDevices },
			kept: [],
			proposals: [
				rec({
					state: "ready",
					review: "added",
					proposals: [
						{
							proposal_id: "prp_000000000a",
							state: "pending",
							actor: "local:claude-code",
							created_at: "2026-11-24T00:00:00Z",
							ranges: [
								{ from_ms: 1200, to_ms: 3400, reason: "a long pause <i>x</i>" },
								{ from_ms: 10_000, to_ms: 10_500, reason: "um" },
							],
						},
						{
							proposal_id: "prp_000000000b",
							state: "accepted",
							actor: "local:claude-code",
							created_at: "2026-11-24T00:00:00Z",
							ranges: [{ from_ms: 1, to_ms: 2, reason: "done" }],
						},
					],
				}),
			],
		});
		expect(html).toContain("Cuts to decide");
		expect(html).toContain('data-record-proposal="prp_000000000a"');
		expect(html).not.toContain('data-record-proposal="prp_000000000b"');
		expect(html).toContain('data-record-range-accept="0"');
		expect(html).toContain('data-record-range-reject="1"');
		expect(html).toContain("0:01.2 to 0:03.4");
		expect(html).toContain("0:10.0 to 0:10.5");
		expect(html).toContain("2 of 2 ranges will be cut");
		expect(html).toContain('data-record-proposal-apply="prp_000000000a"');
		expect(html).not.toContain("<i>");
	});

	test("says what the service answered after a decision", () => {
		const html = draw(
			{
				step: "pick",
				sources: { displays: [], windows: [], ...noDevices },
				kept: [],
			},
			{ notice: "1 of 2 ranges were cut in one batch (revision 7)." },
		);
		expect(html).toContain("1 of 2 ranges were cut in one batch (revision 7).");
	});

	test("the added step shows the cuts that wait for a person", () => {
		const html = draw({
			step: "added",
			recording: rec({
				state: "ready",
				review: "added",
				proposals: [
					{
						proposal_id: "prp_000000000a",
						state: "pending",
						actor: "key abcd",
						created_at: "2026-11-24T00:00:00Z",
						ranges: [{ from_ms: 0, to_ms: 500, reason: "dead air" }],
					},
				],
			}),
		});
		expect(html).toContain('data-record-proposal="prp_000000000a"');
	});

	const withDevices = {
		displays: [{ id: "d1", name: "Display 1", width: 1920, height: 1080 }],
		windows: [],
		microphones: [{ id: "m0123456789abcdef", name: "Desk mic", default: true }],
		cameras: [{ id: "c0123456789abcdef", name: "Webcam", default: false }],
		system_audio: true,
	};

	test("offers the microphone, the computer sound, the camera and the pointer, with the pointer on", () => {
		const html = draw({ step: "pick", sources: withDevices, kept: [] });
		expect(html).toContain("Desk mic");
		expect(html).toContain("Default microphone");
		expect(html).toContain("Webcam");
		expect(html).toContain("data-record-system-audio");
		// the pointer switch is on by default, the computer sound is not
		expect(html).toMatch(/<input[^>]*data-record-events[^>]*>/);
		expect(html.match(/<input[^>]*data-record-events[^>]*>/)?.[0]).toContain(
			"checked",
		);
		expect(
			html.match(/<input[^>]*data-record-system-audio[^>]*>/)?.[0],
		).not.toContain("checked");
		expect(html).toContain("never the keys");
		// it says where the positions are, and promises no deletion an added recording does not have
		expect(html).toContain("stay on this machine");
		expect(html).not.toContain("deleted with the recording");
	});

	test("says when there is no camera and hides the computer sound where it cannot be recorded", () => {
		const html = draw({
			step: "pick",
			sources: { ...withDevices, cameras: [], system_audio: false },
			kept: [],
		});
		expect(html).toContain("No camera was found.");
		expect(html).toContain("data-record-no-camera");
		expect(html).not.toContain("data-record-system-audio");
	});

	test("draws no markup of a device name", () => {
		const html = draw({
			step: "pick",
			sources: {
				...withDevices,
				microphones: [
					{ id: "m1", name: "<img src=x onerror=alert(1)>", default: false },
				],
			},
			kept: [],
		});
		expect(html).not.toContain("<img");
	});

	test("shows a meter for each sound track from the levels of the service, and the note about protected content", () => {
		const html = draw({
			step: "recording",
			recording: rec({
				tracks: [
					{ kind: "screen", start_ms: 0, duration_ms: 0, bytes: 0 },
					{ kind: "microphone", start_ms: 0, duration_ms: 0, bytes: 0 },
					{ kind: "system", start_ms: 0, duration_ms: 0, bytes: 0 },
				],
				levels: { microphone: 420, system: 1500 },
			}),
		});
		expect(html).toContain('data-record-meter="microphone"');
		expect(html).toContain('value="420"');
		// a peak outside the range is kept inside the meter
		expect(html).toContain('value="1000"');
		expect(html).toContain("protected content");
		expect(html).toContain('data-record-pause=""');
		expect(html).not.toContain('data-record-resume=""');
	});

	test("a recording with no sound track has no meter", () => {
		const html = draw({
			step: "recording",
			recording: rec({
				tracks: [{ kind: "screen", start_ms: 0, duration_ms: 0, bytes: 0 }],
			}),
		});
		expect(html).not.toContain("data-record-meters");
	});

	test("a paused recording says so and offers to resume, and can still be stopped", () => {
		const html = draw({
			step: "recording",
			recording: rec({ state: "paused" }),
		});
		expect(html).toContain("Paused");
		expect(html).toContain('data-record-resume=""');
		expect(html).not.toContain('data-record-pause=""');
		expect(html).toContain('data-record-stop=""');
		expect(html).not.toContain('disabled=""');
	});

	test("after Not now, says the recording is kept out of the project, not discarded", () => {
		const html = draw({ step: "kept" });
		expect(html).toContain("stays out of the project");
		expect(html).not.toContain("discarded");
	});
});

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

function draw(view: RecordView, over: Partial<RecordFlow> = {}): string {
	const flow: RecordFlow = {
		view,
		failure: null,
		busy: false,
		placed: false,
		start: async () => {},
		stop: async () => {},
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
			},
		});
		expect(html).toContain("Display 1");
		expect(html).toContain("My notes");
		expect(html).toContain("Displays");
		expect(html).toContain('data-record-start=""');
		expect(html).toContain("Frames per second");
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
			},
		});
		expect(html).not.toContain("<img");
	});
});

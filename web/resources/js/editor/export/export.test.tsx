import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { I18nProvider } from "@/lib/i18n";
import type { Job, LintFinding, MediaEntry } from "../api/types";
import { devFixture } from "../boot/devFixture";
import { startHarness } from "../fixtures/harness";
import Library from "../library/Library";
import { EditorProvider } from "../state/EditorContext";
import { announcedPercent, exportPhase, groupFindings } from "./exportPhase";

const job = (state: Job["state"]): Job => ({
	job_id: "job_1",
	kind: "export",
	project_id: "prj_x",
	state,
	progress: 0.4,
	actor: "local:ui",
	created_at: "2026-11-14T12:00:00Z",
	text: "",
});

const finding = (severity: string): LintFinding => ({
	code: "L_X",
	severity,
	message: "m",
});

describe("export phase", () => {
	test("lint errors block, warnings do not", () => {
		const base = { job: undefined, checking: false };
		expect(exportPhase({ ...base, findings: [finding("error")] })).toBe(
			"blocked",
		);
		expect(exportPhase({ ...base, findings: [finding("warning")] })).toBe(
			"ready",
		);
		expect(exportPhase({ ...base, findings: [] })).toBe("ready");
		expect(exportPhase({ ...base, checking: true, findings: [] })).toBe(
			"checking",
		);
	});

	test("a job decides the phase over the check", () => {
		const base = { checking: false, findings: [] };
		expect(exportPhase({ ...base, job: job("running") })).toBe("running");
		expect(exportPhase({ ...base, job: job("succeeded") })).toBe("succeeded");
		expect(exportPhase({ ...base, job: job("failed") })).toBe("failed");
		expect(exportPhase({ ...base, job: job("cancelled") })).toBe("cancelled");
	});

	test("findings group by severity and progress is announced every 10%", () => {
		const groups = groupFindings([
			finding("error"),
			finding("warning"),
			finding("info"),
		]);
		expect([
			groups.errors.length,
			groups.warnings.length,
			groups.notes.length,
		]).toEqual([1, 1, 1]);
		expect([0, 0.09, 0.1, 0.42, 1].map(announcedPercent)).toEqual([
			0, 0, 10, 40, 100,
		]);
	});
});

describe("Library", () => {
	test("shows a progress bar for an ingesting asset and gates Add to timeline", async () => {
		const h = await startHarness();
		const entry: MediaEntry = {
			asset_id: "ast_pending",
			name: "clip.mp4",
			kind: "video",
			state: "ingesting",
			progress: 0.4,
			duration_frames: null,
			duration_seconds: null,
			has_audio: true,
			trashed: false,
		};
		h.runtime.libraryWriter.setEntries([entry]);
		const html = renderToString(
			<I18nProvider messages={devFixture(1).bootstrap.messages} locale="en">
				<EditorProvider runtime={h.runtime}>
					<TooltipProvider>
						<Library />
					</TooltipProvider>
				</EditorProvider>
			</I18nProvider>,
		);
		expect(html).toContain('role="progressbar"');
		expect(html).toContain('aria-valuenow="40"');
		expect(html).toContain("clip.mp4");
		expect(html).toMatch(
			/<button[^>]*disabled[^>]*>[^<]*(<svg.*?<\/svg>)?Add to timeline/,
		);
	});
});

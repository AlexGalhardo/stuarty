import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { createClient } from "../api/client";
import type {
	BatchResult,
	ExportCheckResult,
	ExportPreset,
	Job,
} from "../api/types";
import { devFixture } from "../boot/devFixture";
import BatchProgress from "./BatchProgress";
import ExportPresets from "./ExportPresets";
import { exportIsStale } from "./exportPhase";
import {
	batchItems,
	groupByNetwork,
	itemName,
	limitLines,
	queuedItem,
} from "./presetText";

const preset = (
	over: Partial<ExportPreset> & { id: string },
): ExportPreset => ({
	network: "Facebook",
	placement: "Reels",
	label: "Facebook Reels",
	container: "mp4",
	codecs: ["hevc", "h264"],
	limits: {
		min_duration_ms: 3000,
		max_duration_ms: 90000,
		min_fps: 24,
		max_fps: 60,
		aspect_ratio: "9:16",
	},
	targets: { width: 1080, height: 1920, video_kbps: 8000, audio_kbps: 192 },
	sources: [
		{
			url: "https://developers.facebook.com/docs/video-api/guides/reels-publishing/",
			read: "2026-11-19",
			says: "limits",
		},
	],
	...over,
});

const presets: ExportPreset[] = [
	preset({ id: "facebook-reels" }),
	preset({
		id: "instagram-reels",
		network: "Instagram",
		label: "Instagram Reels",
		limits: { max_duration_ms: 900000, max_bytes: 300_000_000 },
	}),
	preset({
		id: "tiktok",
		network: "TikTok",
		label: "TikTok",
		limits: { max_duration_ms: 600000, max_bytes: 4_000_000_000 },
	}),
	preset({
		id: "facebook-stories",
		label: "Facebook Stories",
		limits: { max_fps: 30 },
	}),
];

const t = (key: string, params?: Record<string, string | number>): string =>
	params === undefined ? key : `${key} ${JSON.stringify(params)}`;

const messages = devFixture(1).bootstrap.messages;

describe("preset text", () => {
	test("presets group by network in the order the core lists them", () => {
		const groups = groupByNetwork(presets);
		expect(groups.map((g) => [g.network, g.presets.map((p) => p.id)])).toEqual([
			["Facebook", ["facebook-reels", "facebook-stories"]],
			["Instagram", ["instagram-reels"]],
			["TikTok", ["tiktok"]],
		]);
	});

	test("every limit the core states is worded, and none that it does not", () => {
		expect(limitLines(t, presets[0] as ExportPreset)).toEqual([
			'ui.editor.export.presets.limit_duration_range {"min":"3","max":"90"}',
			'ui.editor.export.presets.limit_fps {"min":24,"max":60}',
			'ui.editor.export.presets.limit_aspect {"ratio":"9:16"}',
		]);
		expect(limitLines(t, presets[1] as ExportPreset)).toEqual([
			'ui.editor.export.presets.limit_duration_max {"seconds":"900"}',
			'ui.editor.export.presets.limit_size {"size":"300 MB"}',
		]);
		expect(limitLines(t, presets[2] as ExportPreset)[1]).toContain("4 GB");
		expect(limitLines(t, presets[3] as ExportPreset)).toEqual([
			'ui.editor.export.presets.limit_fps_max {"max":30}',
		]);
		expect(
			limitLines(t, preset({ id: "x", limits: { min_duration_ms: 500 } })),
		).toEqual([
			'ui.editor.export.presets.limit_duration_min {"seconds":"0.5"}',
		]);
	});

	test("a batch has one export per chosen preset, with the range and the codec", () => {
		expect(
			batchItems(["tiktok", "x-video"], "", { start: "30f", end: "90f" }),
		).toEqual([
			{ preset: "tiktok", codec: undefined, start: "30f", end: "90f" },
			{ preset: "x-video", codec: undefined, start: "30f", end: "90f" },
		]);
		expect(batchItems(["tiktok"], "av1", {})[0]?.codec).toBe("av1");
		expect(
			queuedItem({ format: "gif", codec: undefined, start: "1f" }),
		).toEqual({
			format: "gif",
			codec: undefined,
			start: "1f",
			end: undefined,
		});
	});

	test("an item is named by its preset label, else by its format", () => {
		expect(itemName({ preset: "tiktok", format: "mp4" }, presets, t)).toBe(
			"TikTok",
		);
		expect(itemName({ preset: "unknown", format: "mp4" }, presets, t)).toBe(
			"unknown",
		);
		expect(itemName({ format: "gif" }, presets, t)).toBe("gif");
		expect(itemName({ format: "mp4" }, presets, t)).toBe(
			"ui.editor.export.batch.plain",
		);
	});
});

function renderPresets(
	over: Partial<Parameters<typeof ExportPresets>[0]> = {},
): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			<ExportPresets
				presets={presets}
				selected={[]}
				checks={{}}
				codecs={null}
				codec=""
				codecDisabled={false}
				disabled={false}
				onToggle={() => undefined}
				onCodec={() => undefined}
				{...over}
			/>
		</I18nProvider>,
	);
}

const finding = {
	code: "X_DURATION_TOO_LONG",
	severity: "error",
	message: "The video is 100 s long; Facebook Reels allows at most 90 s.",
	limit: "90 s",
	got: "100 s",
};

describe("ExportPresets", () => {
	test("lists the platforms by network with their limits and the date they were read", () => {
		const html = renderPresets();
		expect(html).toContain("Platforms");
		for (const network of ["Facebook", "Instagram", "TikTok"]) {
			expect(html).toContain(network);
		}
		expect(html).toContain("Facebook Reels");
		expect(html).toContain("3 to 90 s");
		expect(html).toContain("aspect 9:16");
		expect(html).toContain("file up to 300 MB");
		expect(html).toContain("Limits read on 2026-11-19");
		expect(html).toContain(
			'href="https://developers.facebook.com/docs/video-api/guides/reels-publishing/"',
		);
		expect(html).toContain('rel="noopener noreferrer"');
	});

	test("a chosen preset shows what the core says, with the exact limit", () => {
		const check: ExportCheckResult = {
			project_id: "prj_x",
			revision: 3,
			preset: "facebook-reels",
			ok: false,
			findings: [finding],
			duration_ms: 100000,
			estimated_bytes: 1,
			text: "",
		};
		const html = renderPresets({
			selected: ["facebook-reels", "tiktok"],
			checks: {
				"facebook-reels": check,
				tiktok: { ...check, preset: "tiktok", ok: true, findings: [] },
			},
		});
		expect(html).toContain("The project does not fit:");
		expect(html).toContain("X_DURATION_TOO_LONG");
		expect(html).toContain("allows at most 90 s");
		expect(html).toContain("limit 90 s, now 100 s");
		expect(html).toContain("The project fits.");
		expect(html).toContain(
			"1 of the chosen platforms do not fit and will fail",
		);
		expect(html).not.toContain("Instagram Reels</label><p");
	});

	test("a check that has not answered says so", () => {
		const html = renderPresets({
			selected: ["tiktok"],
			checks: { tiktok: "pending" },
		});
		expect(html).toContain("Checking");
	});

	test("a codec the machine cannot write is offered disabled", () => {
		const html = renderPresets({
			codecs: [
				{ codec: "h264", available: true, encoder: "libopenh264" },
				{ codec: "hevc", available: false, reason: "hevc_nvenc failed" },
				{ codec: "av1", available: true, encoder: "av1_nvenc" },
			],
		});
		expect(html).toMatch(
			/<option value="hevc" disabled[^>]*>H\.265 \(HEVC, smaller files\) \(not available on this machine\)/,
		);
		expect(html).toMatch(/<option value="av1"[^>]*>AV1/);
		expect(html).not.toMatch(/<option value="av1" disabled/);
		expect(html).toContain("Automatic (as the platform prefers)");
	});
});

const batch: BatchResult = {
	total: 3,
	succeeded: 2,
	failed: 1,
	cancelled: 0,
	items: [
		{
			index: 0,
			preset: "tiktok",
			format: "mp4",
			job_id: "job_aaaaaaaaaa",
			state: "succeeded",
			progress: 1,
			result: {
				revision: 3,
				path: "C:/ws/exports/prj_x/job_aaaaaaaaaa.mp4",
				download_url: "/api/v1/jobs/job_aaaaaaaaaa/file",
				bytes: 2_500_000,
				duration: { frame: 90, seconds: 3 },
				width: 1080,
				height: 1920,
				video_codec: "h264",
				preset: "tiktok",
				codec_fallback: "hevc",
				notes: ["HEVC (H.265) is not available on this machine."],
			},
		},
		{
			index: 1,
			preset: "facebook-reels",
			format: "mp4",
			job_id: "job_bbbbbbbbbb",
			state: "failed",
			progress: 0,
			error: {
				code: "E_VALIDATION",
				message: "the project does not fit Facebook Reels: too long.",
				hint: "shorten the project",
				retryable: false,
			},
		},
		{
			index: 2,
			preset: "instagram-reels",
			format: "mp4",
			state: "queued",
			progress: 0,
		},
	],
};

describe("BatchProgress", () => {
	test("each export shows where it stands, its file or its error", () => {
		const html = renderToString(
			<I18nProvider messages={messages} locale="en">
				<BatchProgress batch={batch} presets={presets} />
			</I18nProvider>,
		);
		expect(html).toContain("2 of 3 done");
		expect(html).toContain("Export 1: TikTok");
		expect(html).toContain("Export 2: Facebook Reels");
		expect(html).toContain("Export 3: Instagram Reels");
		expect(html).toContain('data-state="succeeded"');
		expect(html).toContain('data-state="failed"');
		expect(html).toContain('data-state="queued"');
		expect(html).toContain("job_aaaaaaaaaa.mp4");
		expect(html).toContain("File: h264, 2.5 MB");
		expect(html).toContain("HEVC (H.265) is not available on this machine.");
		expect(html).toContain('href="/api/v1/jobs/job_aaaaaaaaaa/file"');
		expect(html).toContain("E_VALIDATION");
		expect(html).toContain("shorten the project");
		// A queued export has no file and no download.
		expect(html).not.toContain("job_cccccccccc");
	});
});

describe("batch job and the client", () => {
	test("a succeeded batch of an older revision is stale", () => {
		const done = (revision: number): Job =>
			({
				job_id: "job_p",
				kind: "export_batch",
				state: "succeeded",
				result: {
					batch: {
						...batch,
						items: [
							{
								...(batch.items[0] as BatchResult["items"][number]),
								result: {
									...(batch.items[0]?.result as NonNullable<
										BatchResult["items"][number]["result"]
									>),
									revision,
								},
							},
						],
					},
				},
			}) as unknown as Job;
		expect(exportIsStale(done(3), 4)).toBe(true);
		expect(exportIsStale(done(3), 3)).toBe(false);
	});

	test("the client reads the presets and the check, and starts a batch without the unset members", async () => {
		const requests: { method: string; path: string; body?: unknown }[] = [];
		const client = createClient({
			send: async (request: {
				method: string;
				path: string;
				body?: unknown;
			}) => {
				requests.push(request);
				return { status: 200, body: {} };
			},
		} as never);
		await client.getExportPresets();
		await client.getExportPresets(true);
		await client.checkExport("prj_a", "tiktok", { start: "30f" });
		await client.checkExport("prj_a", "x-video");
		await client.startExportBatch("prj_a", [
			{ preset: "tiktok", codec: undefined, start: "30f", end: undefined },
			{ format: "gif" },
		]);
		expect(requests.map((r) => [r.method, r.path])).toEqual([
			["GET", "/export-presets"],
			["GET", "/export-presets?codecs=true"],
			["GET", "/projects/prj_a/export-check?preset=tiktok&start=30f"],
			["GET", "/projects/prj_a/export-check?preset=x-video"],
			["POST", "/projects/prj_a/export-batches"],
		]);
		expect(requests[4]?.body).toEqual({
			items: [{ preset: "tiktok", start: "30f" }, { format: "gif" }],
		});
	});
});

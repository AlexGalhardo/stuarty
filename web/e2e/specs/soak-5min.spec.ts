import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { MB, type MemorySample, sampleMemory } from "../support/memory";
import { caller, expect, ffmpegExe, seedProject, test } from "./helpers";

// P2-QA-002 (SDD-012 sections 19.1 and 20.1 item 28, NFR-008, NFR-009): the
// project of NFR-009 (5 minutes, 200 items, 2,040 caption words) is opened and
// edited for 10 minutes: timeline scroll, zoom, playhead moves, play loops and
// 100 edits. After a forced garbage collection the JS heap stays within 300 MB
// and the tab (renderer plus GPU process, resident memory read from the
// operating system by process id) within 1 GB. The DOM nodes and the event
// listeners must not grow with the time spent: a leak shows as a line that
// keeps rising, so the last reading is held against the first.
//
// The spec runs in the `soak` project of playwright.config.ts, which exists only
// with STUARTY_SOAK=1, alone, because ten minutes next to other specs
// measure the neighbours. STUARTY_SOAK_MINUTES shortens it (default 10);
// the edits are always ten a minute.
const MINUTES = Number(process.env.STUARTY_SOAK_MINUTES ?? 10);
const ROUND_MS = 6_000;
const EDIT_EVERY_MS = 6_000;
const HEAP_LIMIT = 300 * MB;
const TAB_LIMIT = 1024 * MB;
// A reading is compared with the first after the warm-up: the timeline draws
// the items in view, so the same view is restored before each reading.
const NODE_GROWTH = 1.25;
const NODE_SLACK = 200;
const LISTENER_GROWTH = 1.25;
const LISTENER_SLACK = 50;
// The VideoFrames the WebCodecs preview may hold open at once (SDD-012 section
// 11.2 item 8): the cache of 12, 4 decoded ahead in the worker and one in transit,
// for the one video asset of the project.
const OPEN_FRAMES_BOUND = 12 + 4 + 1;

test.use({
	launchOptions: {
		args: [
			"--enable-precise-memory-info",
			"--js-flags=--expose-gc",
			"--disable-gpu-vsync",
			"--disable-frame-rate-limit",
		],
	},
});

// A 5 minute video of 320 x 568: the clip under the timeline, as long as the
// project it plays in.
function soakClip(): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, "soak-301s.mp4");
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				"gradients=size=320x568:rate=30:duration=301:seed=3,hue=s=0.6",
				"-f",
				"lavfi",
				"-i",
				"sine=frequency=330:duration=301",
				"-c:v",
				"libopenh264",
				"-b:v",
				"400k",
				"-pix_fmt",
				"yuv420p",
				"-c:a",
				"aac",
				"-shortest",
				out,
			],
			{ stdio: "ignore" },
		);
	}
	return out;
}

const mb = (bytes: number): string => (bytes / MB).toFixed(0);

test.describe("soak-5min", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test(`the 5 minute project of NFR-009 holds its memory over ${MINUTES} minutes of editing`, async ({
		page,
		request,
		browser,
	}) => {
		test.setTimeout((MINUTES + 10) * 60_000);

		// The project: one clip of 300 s, 165 texts and 34 captions of 60 words.
		const seeded = await seedProject(request, {
			tracks: 0,
			files: [soakClip()],
		});
		const call = await caller(request);
		let revision = (
			(await call("GET", `/projects/${seeded.id}`)) as { revision: number }
		).revision;
		const edit = async (operations: unknown[]): Promise<string[]> => {
			const out = (await call("POST", `/projects/${seeded.id}/edit`, {
				base_revision: revision,
				operations,
			})) as { revision: number; created: string[] };
			revision = out.revision;
			return out.created;
		};
		const [captionTrack, textTrack, videoTrack] = await edit(
			["Captions", "Texts", "Video"].map((name) => ({
				op: "track.add",
				args: { kind: "visual", name },
			})),
		);
		await edit([
			{
				op: "clip.add",
				args: {
					track_id: videoTrack,
					asset_id: seeded.assetId,
					start: "0s",
					source_in: "0s",
					duration: "300s",
				},
			},
		]);
		const words = Array.from({ length: 60 }, (_, i) => `word${i}`).join(" ");
		const texts = Array.from({ length: 165 }, (_, i) => ({
			op: "text.add",
			args: {
				track_id: textTrack,
				start: `${i * 54}f`,
				duration: "30f",
				text: `Soak ${i}`,
			},
		}));
		const captions = Array.from({ length: 34 }, (_, i) => ({
			op: "caption.add",
			args: {
				track_id: captionTrack,
				start: `${i * 260}f`,
				duration: "120f",
				text: words,
			},
		}));
		const textIds: string[] = [];
		const batch = [...texts, ...captions];
		for (let i = 0; i < batch.length; i += 50) {
			const created = await edit(batch.slice(i, i + 50));
			textIds.push(...created.slice(0, texts.slice(i, i + 50).length));
		}
		expect(textIds).toHaveLength(165);

		await page.addInitScript(() => {
			const longTasks: { at: number; duration: number }[] = [];
			new PerformanceObserver((list) => {
				for (const entry of list.getEntries()) {
					longTasks.push({ at: entry.startTime, duration: entry.duration });
				}
			}).observe({ type: "longtask", buffered: true });
			(window as unknown as { __longTasks: typeof longTasks }).__longTasks =
				longTasks;
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]").first()).toBeVisible();
		await page.waitForFunction(() => window.__vv?.dispatch !== undefined);
		const session = await page.context().newCDPSession(page);

		let nextEdit = 0;
		const soakStart = Date.now();
		const round = async (n: number): Promise<void> => {
			// Scroll the whole axis out and back.
			await page.evaluate(async () => {
				const scroller = document.querySelector<HTMLElement>(
					"#timeline .overflow-auto",
				);
				if (!scroller) throw new Error("no timeline");
				const nextFrame = (): Promise<void> =>
					new Promise((r) => requestAnimationFrame(() => r()));
				const zoomIn = document.querySelector<HTMLElement>(
					'[aria-label="Zoom in"]',
				);
				const zoomOut = document.querySelector<HTMLElement>(
					'[aria-label="Zoom out"]',
				);
				zoomIn?.click();
				zoomIn?.click();
				await nextFrame();
				const max = scroller.scrollWidth - scroller.clientWidth;
				for (let i = 0; i <= 60; i++) {
					scroller.scrollLeft = max * (i <= 30 ? i / 30 : 2 - i / 30);
					await nextFrame();
				}
				zoomOut?.click();
				zoomOut?.click();
				await nextFrame();
			});

			// Move the playhead and play a loop.
			if (n % 12 === 0) {
				await page.getByRole("button", { name: "Go to start" }).click();
			}
			const slider = page.getByRole("slider", { name: "Playhead" });
			await slider.focus();
			for (let i = 0; i < 10; i++) await page.keyboard.press("PageUp");
			await page.getByRole("button", { name: "Play" }).click();
			await page.waitForTimeout(2000);
			await page.getByRole("button", { name: "Pause" }).click();

			// Ten edits a minute whatever the rounds cost: more than one when a round
			// ran long. Three kinds, touching different parts of the core.
			const due = Math.floor((Date.now() - soakStart) / EDIT_EVERY_MS) + 1;
			do {
				await editOnce(n);
			} while (nextEdit < due);
		};
		const editOnce = async (n: number): Promise<void> => {
			const id = textIds[nextEdit % textIds.length] as string;
			const kind = nextEdit % 3;
			nextEdit++;
			const outcome = await page.evaluate(
				async ({ id, kind, n }) => {
					const vv = window.__vv;
					if (!vv?.dispatch) throw new Error("no dispatch");
					const ops = [
						[{ op: "item.rename", args: { item_id: id, name: `Soak ${n}` } }],
						[
							{
								op: "item.set_transform",
								args: { item_id: id, x: 200 + (n % 50) * 4 },
							},
						],
						[{ op: "text.set", args: { item_id: id, text: `Edit ${n}` } }],
					];
					return (await vv.dispatch(ops[kind] as never)).ok;
				},
				{ id, kind, n },
			);
			expect(outcome, `edit ${n}`).toBe(true);
		};

		// Opening the project and playing it: NFR-009 is that no task runs longer
		// than 50 ms (a `longtask` is one) while it does. Only the open and one play
		// count here; the rounds that follow scroll and zoom as hard as they can.
		await page.getByRole("button", { name: "Play" }).click();
		await page.waitForTimeout(3000);
		await page.getByRole("button", { name: "Pause" }).click();
		const opening = await page.evaluate(
			() =>
				(
					window as unknown as {
						__longTasks: { at: number; duration: number }[];
					}
				).__longTasks,
		);
		console.log(
			`soak: open and first play: ${opening.length} long tasks${opening.length > 0 ? ` (${opening.map((t) => t.duration.toFixed(0)).join(", ")} ms)` : ""}`,
		);
		// Soft: a task over 50 ms is a budget of the reference machine, and this
		// spec must go on to its memory numbers on a loaded one.
		expect.soft(opening, "long tasks while opening and playing").toEqual([]);
		await round(0);

		const readings: { minute: number; sample: MemorySample }[] = [];
		const take = async (minute: number): Promise<MemorySample> => {
			await page.getByRole("button", { name: "Go to start" }).click();
			await page.evaluate(() => {
				const scroller = document.querySelector<HTMLElement>(
					"#timeline .overflow-auto",
				);
				if (scroller) scroller.scrollLeft = 0;
			});
			await page.waitForTimeout(500);
			const sample = await sampleMemory(browser, page, session);
			readings.push({ minute, sample });
			console.log(
				`soak minute ${minute}: JS heap ${mb(sample.jsHeapBytes)} MB, tab (renderer + GPU) ${mb(sample.tabBytes)} MB, tree ${mb(sample.treeBytes)} MB, DOM nodes ${sample.domNodes}, listeners ${sample.listeners}, documents ${sample.documents}, edits ${nextEdit}`,
			);
			return sample;
		};
		const first = await take(0);

		const end = Date.now() + MINUTES * 60_000;
		let nextReading = Date.now() + 60_000;
		let n = 1;
		let minute = 0;
		while (Date.now() < end) {
			const started = Date.now();
			await round(n++);
			if (Date.now() >= nextReading) {
				minute++;
				nextReading += 60_000;
				await take(minute);
			}
			const rest = ROUND_MS - (Date.now() - started);
			if (rest > 0) await page.waitForTimeout(rest);
		}
		const last = await take(MINUTES);

		const longTasks = await page.evaluate(
			() =>
				(
					window as unknown as {
						__longTasks: { at: number; duration: number }[];
					}
				).__longTasks,
		);
		const peak = (pick: (s: MemorySample) => number): number =>
			Math.max(...readings.map((r) => pick(r.sample)));
		console.log(
			`soak ${MINUTES} min: ${nextEdit} edits, ${n} rounds; peak JS heap ${mb(peak((s) => s.jsHeapBytes))} MB (limit ${mb(HEAP_LIMIT)}), peak tab ${mb(peak((s) => s.tabBytes))} MB (limit ${mb(TAB_LIMIT)}); DOM nodes ${first.domNodes} to ${last.domNodes}, listeners ${first.listeners} to ${last.listeners}, documents ${first.documents} to ${last.documents}; ${longTasks.length} long tasks over the whole run`,
		);

		expect(nextEdit, "edits made").toBeGreaterThanOrEqual(
			Math.floor(MINUTES * 10) - 5,
		);
		expect(
			peak((s) => s.jsHeapBytes),
			"JS heap",
		).toBeLessThanOrEqual(HEAP_LIMIT);
		expect(
			peak((s) => s.tabBytes),
			"tab memory",
		).toBeLessThanOrEqual(TAB_LIMIT);
		expect(last.domNodes, "DOM nodes").toBeLessThanOrEqual(
			first.domNodes * NODE_GROWTH + NODE_SLACK,
		);
		expect(last.listeners, "event listeners").toBeLessThanOrEqual(
			first.listeners * LISTENER_GROWTH + LISTENER_SLACK,
		);
		expect(last.documents, "documents").toBeLessThanOrEqual(
			first.documents + 1,
		);
		// A leak of VideoFrames would grow without bound over ten minutes of seeks
		// and plays (P5-UI-002); the preview counts the most it held open.
		const canvas = page
			.getByRole("main", { name: "Preview" })
			.locator("canvas");
		const peakFrames = Number(await canvas.getAttribute("data-peak-frames"));
		console.log(
			`soak: peak open VideoFrames ${peakFrames} (limit ${OPEN_FRAMES_BOUND})`,
		);
		expect(peakFrames, "open VideoFrames").toBeLessThanOrEqual(
			OPEN_FRAMES_BOUND,
		);
	});
});

import { readFile } from "node:fs/promises";
import { createServer, type Server } from "node:net";
import { join } from "node:path";
import type { Page } from "@playwright/test";

import { ensureAccount, signInWithPassword } from "../support/account";
import { test as base, caller, expect, home, seedProject } from "./helpers";

// Epic 63 (SDD-025 section 18): a person records the screen and the microphone in the browser, the chunks go up to
// the server as they are made, and the take lands in the same project through the same review and placement as a
// recording of the desktop app. The browsers play the devices their own way, and no real device is opened:
// Chromium has fake-device flags (a test pattern for the screen and the camera, a beep for the microphone) and its
// fake UI answers the permission prompts that a person answers in a real browser; Firefox has the preferences
// media.navigator.streams.fake and media.navigator.permission.disabled, which do the same. The product never
// answers a prompt. A browser whose build has no screen capture for a page at all (the WebKit of Playwright on
// Windows has no navigator.mediaDevices) cannot record: there the test checks what SDD-025 section 18.1 promises
// that person, the sentence that says the browser cannot capture the screen and the Start button off, in place of
// the recording. One browser upload may be open at a time for a person (SDD-025 section 18.4), and every test here,
// in every browser, is that person on the same server: the tests of a browser run one after the other, and a test
// holds the recording lock below for as long as it lasts, so the browsers of a local run never record at once.
// The permissions option of Chromium and the launch options of each engine are set per browser: Firefox does not
// know the permission names microphone and camera ("Unknown permission"), and Chromium ignores the preferences.
const test = base.extend({
	permissions: async ({ browserName }, use) => {
		await use(browserName === "chromium" ? ["microphone", "camera"] : []);
	},
	launchOptions: async ({ browserName }, use) => {
		if (browserName === "chromium") {
			await use({
				args: [
					"--use-fake-ui-for-media-stream",
					"--use-fake-device-for-media-stream",
					"--auto-select-desktop-capture-source=Entire screen",
				],
			});
		} else if (browserName === "firefox") {
			await use({
				firefoxUserPrefs: {
					"media.navigator.streams.fake": true,
					"media.navigator.permission.disabled": true,
				},
			});
		} else {
			await use({});
		}
	},
});

test.describe.configure({ timeout: 240_000, mode: "serial" });

test.use({ viewport: { width: 1440, height: 900 } });

// The recording lock: a port of the run's block (the last one, after the account server), listened on by the test
// that holds it. A local run starts the projects at once, each in its own worker process, so the lock has to hold
// across processes; the system lets the port go when its worker dies, so a killed run leaves no lock behind.
const RECORDING_LOCK_PORT = Number(process.env.STUARTY_E2E_PORT ?? 3101) + 9;
// The longest a test waits for the lock: the tests of the other browsers that hold it, one after the other.
const LOCK_WAIT_MS = 300_000;

async function holdRecordingLock(): Promise<Server> {
	const server = createServer();
	const tryListen = (): Promise<boolean> =>
		new Promise((resolve, reject) => {
			const failed = (error: NodeJS.ErrnoException): void => {
				if (error.code === "EADDRINUSE") resolve(false);
				else reject(error);
			};
			server.once("error", failed);
			server.listen(
				{ port: RECORDING_LOCK_PORT, host: "127.0.0.1", exclusive: true },
				() => {
					server.off("error", failed);
					resolve(true);
				},
			);
		});
	await expect
		.poll(tryListen, { timeout: LOCK_WAIT_MS, intervals: [500] })
		.toBe(true);
	return server;
}

interface RecordingView {
	recording_id: string;
	state: string;
	review?: string;
	duration_ms: number;
	bytes: number;
	asset_id?: string;
	source: { kind: string };
	upload?: { tracks: { kind: string; next_index: number; bytes: number }[] };
	tracks?: { kind: string; duration_ms: number; asset_id?: string }[];
}

async function recordingsOf(
	request: import("@playwright/test").APIRequestContext,
	project: string,
): Promise<RecordingView[]> {
	const call = await caller(request);
	const got = (await call("GET", `/projects/${project}/recordings`)) as {
		recordings: RecordingView[];
	};
	return got.recordings;
}

// The session of a person: the password sign-in of the account every server of the run has (ADR-0052). Only a
// person records in the browser, so the session of the page is made one before the dialog opens.
async function signInAsPerson(page: Page): Promise<void> {
	await ensureAccount(page.request, home);
	await signInWithPassword(page);
}

async function openDialog(page: Page, project: string): Promise<void> {
	await signInAsPerson(page);
	await page.goto(`/editor/${project}`);
	await page.locator("[data-import-open]").click();
	await page.locator("[data-record-open]").click();
	await expect(page.locator("[data-record-step]")).toBeVisible();
}

// A browser that cannot capture the screen for a page says so in the dialog and leaves Start off. Returns true when
// this browser is one of them, after checking that sentence, so the test that needed a recording ends there.
// Whether it is one is read from the browser itself, not from the dialog: a dialog that blocks a browser that can
// capture fails the test, and Chromium and Firefox, which have their fake devices here, must always be able to.
async function endsAsBlocked(page: Page, project: string): Promise<boolean> {
	const browserName = page.context().browser()?.browserType().name();
	const canCapture = await page.evaluate(
		() => typeof navigator.mediaDevices?.getDisplayMedia === "function",
	);
	if (browserName === "chromium" || browserName === "firefox") {
		expect(canCapture, `${browserName} offers screen capture to a page`).toBe(
			true,
		);
	}
	const blocked = page.locator("[data-record-browser-blocked]");
	if (canCapture) {
		await expect(page.locator("[data-record-browser-codec]")).toBeVisible();
		await expect(blocked).toHaveCount(0);
		return false;
	}
	await expect(blocked).toHaveAttribute(
		"data-record-browser-blocked",
		"no_display_capture",
	);
	await expect(blocked).toContainText("cannot capture the screen");
	await expect(page.locator("[data-record-start]")).toBeDisabled();
	expect(await recordingsOf(page.request, project)).toHaveLength(0);
	return true;
}

test.describe("recording in the browser", () => {
	let lock: Server | undefined;
	test.beforeEach(async () => {
		// the wait for the lock is added to the time of the test, so a test is never failed by the length of another
		test.setTimeout(test.info().timeout + LOCK_WAIT_MS);
		lock = await holdRecordingLock();
	});
	test.afterEach(async () => {
		const held = lock;
		lock = undefined;
		if (held) await new Promise<void>((resolve) => held.close(() => resolve()));
	});

	test("a person records 30 seconds, the take lands in the project and goes on the timeline", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 0, assets: 1 });
		await openDialog(page, seeded.id);
		if (await endsAsBlocked(page, seeded.id)) return;

		// before starting, the dialog says what this browser records and that it asks for permission itself
		const step = page.locator("[data-record-step]");
		await expect(step).toHaveAttribute("data-record-step", "browser");
		await expect(page.locator("[data-record-browser-codec]")).toBeVisible();
		await expect(step).toContainText("asks your permission");
		await expect(page.locator("[data-record-start]")).toBeEnabled();

		await page.locator("[data-record-start]").click();
		const live = page.locator("[data-record-browser-live]");
		await expect(live).toBeVisible({ timeout: 30_000 });
		await expect(live).toHaveAttribute("data-record-browser-live", "recording");
		await expect(
			page.locator('[data-record-browser-shared-item="microphone"]'),
		).toBeVisible();

		// the server holds what the browser sent while it records, and says where each track is
		await expect
			.poll(
				async () => {
					const [rec] = await recordingsOf(request, seeded.id);
					return rec?.source.kind === "browser" &&
						rec.state === "recording" &&
						(rec.upload?.tracks[0]?.next_index ?? 0) >= 2
						? rec.bytes
						: 0;
				},
				{ timeout: 30_000 },
			)
			.toBeGreaterThan(0);
		// the clock of the recorder moves, and the megabytes the server confirmed grow
		await expect(page.locator("[data-record-clock]")).not.toContainText(
			"Elapsed 0:00",
			{ timeout: 10_000 },
		);

		// thirty seconds of recording, counted by the clock of the recorder, which starts with the capture
		await expect(page.locator("[data-record-clock]")).toContainText(
			"Elapsed 0:30",
			{ timeout: 45_000 },
		);
		await page.locator("[data-record-stop]").click();

		// the server assembles the chunks, and the person is asked, as for a recording of the desktop app
		await expect(page.locator("[data-record-notice]")).toBeVisible({
			timeout: 90_000,
		});
		await expect(step).toHaveAttribute("data-record-step", "review");

		const [ready] = await recordingsOf(request, seeded.id);
		expect(ready?.state).toBe("ready");
		expect(ready?.review).toBe("pending");
		// 30 s plus or minus 1 s
		expect(ready?.duration_ms).toBeGreaterThan(29_000);
		expect(ready?.duration_ms).toBeLessThan(31_000);

		await page.locator("[data-record-add]").click();
		await expect(step).toHaveAttribute("data-record-step", "added");

		// the asset exists in the project, and the recording says which one it is
		const call = await caller(request);
		const [added] = await recordingsOf(request, seeded.id);
		const assetId = added?.asset_id ?? "";
		expect(assetId).toMatch(/^ast_/);
		expect(added?.duration_ms).toBeGreaterThan(29_000);
		expect(added?.duration_ms).toBeLessThan(31_000);
		// the asset is in the library of the project once its ingest is done, and it is as long as the take
		await expect
			.poll(
				async () => {
					const media = (await call("GET", `/projects/${seeded.id}/media`)) as {
						assets: {
							asset_id: string;
							state: string;
							duration_seconds: number | null;
						}[];
					};
					const entry = media.assets.find((a) => a.asset_id === assetId);
					return entry?.state === "ready" ? entry.duration_seconds : null;
				},
				{ timeout: 60_000 },
			)
			.toBeGreaterThan(29);
		const media = (await call("GET", `/projects/${seeded.id}/media`)) as {
			assets: { asset_id: string; duration_seconds: number | null }[];
		};
		const length =
			media.assets.find((a) => a.asset_id === assetId)?.duration_seconds ?? 0;
		expect(length).toBeGreaterThan(29);
		expect(length).toBeLessThan(31);

		// and it goes on the timeline in one step
		await page.locator("[data-record-place]").click();
		await expect(page.getByText("Placed on the timeline.")).toBeVisible();
		const doc = (await call(
			"GET",
			`/projects/${seeded.id}?detail=document`,
		)) as {
			document: {
				assets: Record<string, { kind: string }>;
				items: Record<string, { asset?: string }>;
			};
		};
		expect(doc.document.assets[assetId]?.kind).toBe("video");
		expect(
			Object.values(doc.document.items).some((item) => item.asset === assetId),
		).toBe(true);
	});

	test("a page that is not a secure context cannot record: Start is off and the reason is shown", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 0, assets: 1 });
		// A page on plain HTTP outside localhost has no isSecureContext and no mediaDevices; the server of this run
		// answers only on loopback, so the page is made to look like one.
		await page.addInitScript(() => {
			Object.defineProperty(window, "isSecureContext", { value: false });
			Object.defineProperty(navigator, "mediaDevices", { value: undefined });
		});
		await openDialog(page, seeded.id);

		await expect(page.locator("[data-record-step]")).toHaveAttribute(
			"data-record-step",
			"browser",
		);
		await expect(
			page.locator('[data-record-browser-blocked="insecure_context"]'),
		).toContainText("HTTPS or on localhost");
		await expect(page.locator("[data-record-start]")).toBeDisabled();
		expect(await recordingsOf(request, seeded.id)).toHaveLength(0);
	});

	test("a tab closed in the middle leaves a take that can be finished into a playable file", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 0, assets: 1 });
		const context = page.context();
		// the tour of the first run would sit over the dialog of a tab that has not seen it
		await context.addInitScript(() => {
			try {
				localStorage.setItem("vv.shortcutsSeen", "1");
			} catch {
				// storage can be blocked
			}
		});
		const first = await context.newPage();
		await openDialog(first, seeded.id);
		if (await endsAsBlocked(first, seeded.id)) return;
		await first.locator("[data-record-start]").click();
		await expect(first.locator("[data-record-browser-live]")).toBeVisible({
			timeout: 30_000,
		});

		// wait until the server holds a few chunks, then close the tab with no Stop and no finish
		await expect
			.poll(
				async () =>
					(await recordingsOf(request, seeded.id))[0]?.upload?.tracks[0]
						?.next_index ?? 0,
				{ timeout: 30_000 },
			)
			.toBeGreaterThanOrEqual(3);
		await first.close();

		const [open] = await recordingsOf(request, seeded.id);
		expect(open?.state).toBe("recording");

		// a new tab finds the take and offers to finish it with what the server received
		const second = await context.newPage();
		await openDialog(second, seeded.id);
		await expect(second.locator("[data-record-step]")).toHaveAttribute(
			"data-record-step",
			"interrupted",
		);
		await second.locator("[data-record-finish-interrupted]").click();
		await expect(second.locator("[data-record-notice]")).toBeVisible({
			timeout: 90_000,
		});

		const [done] = await recordingsOf(request, seeded.id);
		expect(done?.state).toBe("ready");
		expect(done?.review).toBe("pending");
		expect(done?.duration_ms).toBeGreaterThan(1_000);
		await second.close();
	});

	// Chromium only: the test plays the Stop sharing of the browser by dispatching an `ended` event on the track of
	// the screen. In Firefox a listener added with addEventListener on a MediaStreamTrack never receives an untrusted
	// event, one that a page dispatches itself (only `onended` does), and Firefox has no way to end a fake screen
	// capture from outside, so the path cannot be exercised there. The `ended` that Firefox fires when a person
	// presses its own Stop sharing is trusted and reaches the listener of the recorder. In WebKit the
	// browser has no capture to stop. The unit tests of the recorder cover the handler itself.
	test("Stop sharing in the browser ends the take and the dialog moves on to the review @chromium-only", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 0, assets: 1 });
		// keep the stream the browser gives, so the test can do what the browser's own Stop sharing does
		await page.addInitScript(() => {
			const devices = navigator.mediaDevices;
			const give = devices.getDisplayMedia.bind(devices);
			devices.getDisplayMedia = async (constraints) => {
				const stream = await give(constraints);
				(window as unknown as { shared: MediaStream }).shared = stream;
				return stream;
			};
		});
		await openDialog(page, seeded.id);
		if (await endsAsBlocked(page, seeded.id)) return;
		await page.locator("[data-record-start]").click();
		await expect(page.locator("[data-record-browser-live]")).toHaveAttribute(
			"data-record-browser-live",
			"recording",
			{ timeout: 30_000 },
		);
		await expect
			.poll(
				async () =>
					(await recordingsOf(request, seeded.id))[0]?.upload?.tracks[0]
						?.next_index ?? 0,
				{ timeout: 30_000 },
			)
			.toBeGreaterThanOrEqual(2);

		// Stop sharing: the browser stops the track of the screen and says it ended
		await page.evaluate(() => {
			const shared = (window as unknown as { shared: MediaStream }).shared;
			const [track] = shared.getVideoTracks();
			track?.stop();
			track?.dispatchEvent(new Event("ended"));
		});

		// the take is finished with what was sent, and the dialog asks the person instead of waiting forever
		await expect(page.locator("[data-record-notice]")).toBeVisible({
			timeout: 90_000,
		});
		await expect(page.locator("[data-record-step]")).toHaveAttribute(
			"data-record-step",
			"review",
		);
		const [done] = await recordingsOf(request, seeded.id);
		expect(done?.state).toBe("ready");
		expect(done?.review).toBe("pending");
	});

	test("an agent token cannot upload a recording", async ({ request }) => {
		const seeded = await seedProject(request, { tracks: 0, assets: 1 });
		const begin = await request.post("/api/v1/recordings/upload", {
			headers: {
				"Content-Type": "application/json",
				Authorization: `Bearer ${await readToken()}`,
			},
			data: { project_id: seeded.id, container: "webm" },
		});
		expect(begin.status()).toBe(403);
		expect(((await begin.json()) as { code: string }).code).toBe(
			"E_PRIVILEGED_SESSION",
		);
	});
});

async function readToken(): Promise<string> {
	return (await readFile(join(home, "token"), "utf8")).trim();
}

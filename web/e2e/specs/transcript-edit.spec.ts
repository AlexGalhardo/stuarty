import { type Running, startFirstFlow } from "../support/firstFlow";
import { caller, expect, seedProject, test } from "./helpers";

// Epic 69 (SDD-023): a creator deletes a sentence in the transcript and the video is cut there; undo brings it back; the
// search for fillers changes nothing until a person confirms.
//
// The spec runs its own server because it needs the test transcriber (STUARTY_TEST_TRANSCRIBER works only under
// APP_ENV=test), which answers every audio with the words hello, brave, new and world. It starts the same server as the
// first-flow spec, on a workspace of its own.
const PORT = Number(process.env.STUARTY_E2E_PORT ?? 3101) + 6;
const BASE = `http://127.0.0.1:${PORT}`;
const FPS = 30;

test.use({ baseURL: BASE, viewport: { width: 1600, height: 900 } });
test.describe.configure({ mode: "serial" });

let running: Running | undefined;

test.beforeAll(async () => {
	test.setTimeout(600_000);
	running = await startFirstFlow(PORT, "transcript-edit");
});

test.afterAll(() => {
	running?.server.kill();
	running?.fake.kill();
});

interface Document {
	items: Record<string, { type: string; duration: number }>;
}

// @chromium-only: the spec owns one server on one port, and one engine is enough.
test.describe("edit by text @chromium-only", () => {
	test("deleting a sentence cuts the video and undo restores it", async ({
		page,
		request,
	}) => {
		test.setTimeout(300_000);
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "2s",
		});
		const api = await caller(request);

		const started = (await api("POST", `/projects/${seeded.id}/jobs`, {
			kind: "transcribe",
			args: { asset_id: seeded.assetId },
			timeline_id: seeded.id,
		})) as { job_id: string };
		await expect
			.poll(
				async () =>
					((await api("GET", `/jobs/${started.job_id}`)) as { state: string })
						.state,
				{ timeout: 60_000 },
			)
			.toBe("succeeded");

		const revision = async (): Promise<number> =>
			((await api("GET", `/projects/${seeded.id}`)) as { revision: number })
				.revision;
		const frames = async (): Promise<number> => {
			const shown = (await api(
				"GET",
				`/projects/${seeded.id}?detail=document`,
			)) as { document: Document };
			return Object.values(shown.document.items)
				.filter((item) => item.type === "clip")
				.reduce((sum, item) => sum + item.duration, 0);
		};
		const whole = await frames();

		await page.goto(`/editor/${seeded.id}`);
		await page.locator(`[data-item-id="${seeded.itemIds[0]}"]`).click();
		await page.getByRole("tab", { name: "Captions" }).click();
		const panel = page.getByRole("region", { name: "Captions" });
		const words = panel.locator("[data-word-index]");
		await expect(words).toHaveCount(4);
		await expect(words.nth(2)).toHaveText("new");

		// The search for fillers changes nothing: the transcript has none, and the panel says what it checked.
		const before = await revision();
		await panel
			.getByRole("button", { name: "Find fillers and repetitions" })
			.click();
		await expect(panel.locator("[data-fillers]")).toContainText(
			"Nothing found. Checked:",
		);
		expect(await revision()).toBe(before);

		// "new world" is the sentence: select its two words and delete them.
		await words.nth(2).click();
		await words.nth(3).click({ modifiers: ["Shift"] });
		await expect(panel.getByText("2 words selected")).toBeVisible();
		await panel.getByRole("button", { name: "Delete selected words" }).click();
		await expect(words.nth(2)).toHaveAttribute("data-cut", "true");
		await expect(words.nth(3)).toHaveAttribute("data-cut", "true");
		await expect(words.nth(1)).not.toHaveAttribute("data-cut", "true");

		// The timeline loses the range of the two words: 1100 ms to 1900 ms, within one frame.
		const lost = Math.round(((1900 - 1100) * FPS) / 1000);
		expect(Math.abs(whole - (await frames()) - lost)).toBeLessThanOrEqual(1);

		await page.getByRole("button", { name: "Undo" }).first().click();
		await expect(words.nth(2)).not.toHaveAttribute("data-cut", "true");
		await expect(words.nth(3)).not.toHaveAttribute("data-cut", "true");
		expect(await frames()).toBe(whole);
	});
});

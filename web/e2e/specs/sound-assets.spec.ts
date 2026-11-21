import { audioClip, caller, expect, test } from "./helpers";

// Epic 58 (SDD-011 section 14.6, D-121): a sound is imported from the Sound Assets page, plays with its waveform
// (the peaks come from the server) and is used in a project with two timelines, choosing the timeline.
test.describe("Sound Assets", () => {
	test("import a sound, play it with its waveform, use it in a project with two timelines", async ({
		page,
		request,
		browserName,
	}) => {
		const call = await caller(request);
		const stamp = Date.now();
		const create = async (name: string): Promise<string> =>
			((await call("POST", "/projects", { name })) as { project_id: string })
				.project_id;
		await create(`Sound source ${stamp}`);
		const target = await create(`Sound target ${stamp}`);
		await call("POST", `/projects/${target}/items`, {
			kind: "timeline",
			title: "Second cut",
			preset: "16:9",
		});

		await page.goto("/sound-assets");
		await expect(
			page.getByRole("heading", { name: "Sound Assets", level: 1 }),
		).toBeVisible();
		await expect(
			page.getByRole("link", { name: "Sound Assets" }),
		).toBeVisible();

		const form = page.getByTestId("sound-import");
		await form.getByLabel("Project").selectOption({
			label: `Sound source ${stamp}`,
		});
		await form
			.getByLabel("Path of the file on this computer")
			.fill(audioClip(58));
		await form.getByRole("button", { name: "Import" }).click();
		await expect(form.getByRole("status")).toContainText("being prepared");

		// The file is ingested by the service; the list is read again until it holds the sound.
		const row = page
			.getByTestId("sound-row")
			.filter({ hasText: "tone4-58.wav" });
		await expect(async () => {
			await page.getByRole("button", { name: "Apply" }).click();
			await expect(row.first()).toBeVisible({ timeout: 1500 });
		}).toPass({ timeout: 30_000 });

		// Only audio is listed: the Video Assets page does not hold it.
		const hash = await row.first().getAttribute("data-asset-hash");
		expect(hash).toMatch(/^sha256:[0-9a-f]{64}$/);
		await expect(page.getByTestId("sound-duration").first()).toBeVisible();

		// The waveform is drawn from the peaks of the server, and the file plays.
		await expect(row.first().getByTestId("waveform")).toHaveAttribute(
			"data-state",
			"ready",
		);
		// The WebKit build of Playwright ships no AAC decoder, so the proxy (an audio-only MP4) cannot load there.
		if (browserName !== "webkit") {
			const duration = await row
				.first()
				.locator("audio")
				.evaluate(
					(el: HTMLAudioElement) =>
						new Promise<number>((resolve, reject) => {
							el.addEventListener("loadedmetadata", () => resolve(el.duration));
							el.addEventListener("error", () =>
								reject(new Error("audio error")),
							);
							el.load();
						}),
				);
			expect(duration).toBeGreaterThan(0);
		}

		await page.goto("/video-assets");
		await expect(page.locator(`[data-asset-hash="${hash}"]`)).toHaveCount(0);

		await page.goto("/sound-assets");
		await page
			.locator(`[data-asset-hash="${hash}"]`)
			.getByRole("button", { name: /tone4-58/ })
			.click();
		const detail = page.getByTestId("asset-detail");
		await detail.getByLabel(`Sound target ${stamp}`).check();
		await expect(detail.getByRole("button", { name: "Use" })).toBeDisabled();
		await detail
			.getByTestId("asset-use-timelines")
			.getByLabel("Second cut")
			.check();
		await detail.getByRole("button", { name: "Use" }).click();
		await expect(detail.getByRole("status")).toContainText("No copy was made");
	});
});

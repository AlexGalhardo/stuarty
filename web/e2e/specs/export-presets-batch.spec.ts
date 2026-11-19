import { expect, seedProject, test } from "./helpers";

test.describe.configure({ timeout: 240_000 });

// RF-087 to RF-089, SDD-004 sections 8.2 to 8.4, SDD-012 section 14: the export
// dialog lists the platform presets with their limits, asks the core whether the
// project fits each one before anything is encoded, runs several at once as a
// batch, and queues an export that found every slot busy. Everything runs
// against the real server and the real encoders of the machine.

const MP4_FTYP = "ftyp";

async function openExport(
	page: import("@playwright/test").Page,
	projectId: string,
	items = 1,
): Promise<import("@playwright/test").Locator> {
	await page.goto(`/editor/${projectId}`);
	await expect(page.locator("[data-item-id]")).toHaveCount(items);
	await page.getByRole("button", { name: "Export", exact: true }).click();
	const dialog = page.getByRole("dialog");
	await expect(dialog.getByTestId("export-presets")).toBeVisible();
	return dialog;
}

test.describe("export-presets-batch", () => {
	test("two presets run as one batch and both files download", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const dialog = await openExport(page, seeded.id);

		// The platforms are grouped by network, each with its limits and the date they were read.
		for (const network of ["YouTube", "Instagram", "Facebook", "TikTok", "X"]) {
			await expect(
				dialog.getByTestId("export-presets").locator("legend", {
					hasText: new RegExp(`^${network}$`),
				}),
			).toBeVisible();
		}
		await expect(
			dialog.getByText("Limits read on 2026-11-19").first(),
		).toBeVisible();
		await expect(dialog.getByText("up to 180 s").first()).toBeVisible();

		await dialog.locator("#export-preset-tiktok").check();
		await dialog.locator("#export-preset-youtube-shorts").check();
		// The core says the project fits, before anything is encoded.
		await expect(dialog.getByText("The project fits.")).toHaveCount(2);
		await dialog.getByRole("button", { name: "Export 2 files" }).click();

		await expect(dialog.getByTestId("batch-progress")).toBeVisible();
		await expect(dialog.locator("[data-phase='succeeded']")).toBeVisible({
			timeout: 200_000,
		});
		const items = dialog.locator("[data-batch-item]");
		await expect(items).toHaveCount(2);
		await expect(items.nth(0)).toHaveAttribute("data-state", "succeeded");
		await expect(items.nth(1)).toHaveAttribute("data-state", "succeeded");
		await expect(items.nth(0)).toContainText("TikTok");
		await expect(items.nth(1)).toContainText("YouTube Shorts");

		const links = dialog.getByRole("link", { name: /^Download/ });
		await expect(links).toHaveCount(2);
		for (let i = 0; i < 2; i++) {
			const href = await links.nth(i).getAttribute("href");
			expect(href).toMatch(/\/api\/v1\/jobs\/job_.+\/file$/);
			const file = await page.request.get(href as string);
			expect(file.ok()).toBe(true);
			const body = await file.body();
			expect(body.length).toBeGreaterThan(1000);
			expect(body.subarray(4, 8).toString("latin1")).toBe(MP4_FTYP);
		}
	});

	test("a preset the project does not fit shows the exact limit before encoding", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "2s",
		});
		const jobs: string[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && /\/(jobs|export-batches)$/.test(r.url())) {
				jobs.push(r.url());
			}
		});
		const dialog = await openExport(page, seeded.id);
		await dialog.locator("#export-preset-facebook-reels").check();
		await dialog.locator("#export-preset-tiktok").check();

		await expect(dialog.getByText("The project does not fit:")).toBeVisible();
		await expect(dialog.getByText("X_DURATION_TOO_SHORT")).toBeVisible();
		await expect(dialog.getByText("limit 3 s, now 2 s")).toBeVisible();
		await expect(
			dialog.getByText(
				"1 of the chosen platforms do not fit and will fail; fix the project or leave them out.",
			),
		).toBeVisible();
		await expect(dialog.getByText("The project fits.")).toHaveCount(1);
		// Looking at the check started nothing.
		expect(jobs).toEqual([]);
	});

	test("a batch with one export that cannot fit ends with the files of the others and one error", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "2s",
		});
		const dialog = await openExport(page, seeded.id);
		for (const id of ["tiktok", "facebook-reels", "youtube-shorts"]) {
			await dialog.locator(`#export-preset-${id}`).check();
		}
		await dialog.getByRole("button", { name: "Export 3 files" }).click();

		await expect(dialog.locator("[data-phase='failed']")).toBeVisible({
			timeout: 200_000,
		});
		await expect(dialog.getByText("Some exports failed")).toBeVisible();
		const items = dialog.locator("[data-batch-item]");
		await expect(items).toHaveCount(3);
		await expect(items.nth(0)).toHaveAttribute("data-state", "succeeded");
		await expect(items.nth(1)).toHaveAttribute("data-state", "failed");
		await expect(items.nth(2)).toHaveAttribute("data-state", "succeeded");
		// The failed one names the exact limit and what to do; the others have their files.
		await expect(items.nth(1)).toContainText("Facebook Reels");
		await expect(items.nth(1)).toContainText("at least 3 s");
		await expect(items.nth(1)).toContainText("E_VALIDATION");
		await expect(
			items.nth(0).getByRole("link", { name: /^Download/ }),
		).toBeVisible();
		await expect(
			items.nth(2).getByRole("link", { name: /^Download/ }),
		).toBeVisible();
		await expect(
			items.nth(1).getByRole("link", { name: /^Download/ }),
		).toHaveCount(0);
	});

	// VV-220: every export slot of the server busy is not a dead end.
	test("an export that finds every slot busy is queued and runs by itself", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "3s",
		});
		const bodies: unknown[] = [];
		let refused = 0;
		await page.route("**/api/v1/projects/*/jobs", async (route) => {
			const body = route.request().postDataJSON() as {
				kind?: string;
				dry_run?: boolean;
			};
			if (
				route.request().method() !== "POST" ||
				body.kind !== "export" ||
				body.dry_run === true
			) {
				return route.fallback();
			}
			refused += 1;
			return route.fulfill({
				status: 429,
				contentType: "application/problem+json",
				body: JSON.stringify({
					code: "E_QUEUE_FULL",
					message: "a concurrency limit is reached.",
					retryable: true,
					details: { running: ["job_aaaaaaaaaa", "job_bbbbbbbbbb"] },
				}),
			});
		});
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/export-batches")) {
				bodies.push(r.postDataJSON());
			}
		});
		const dialog = await openExport(page, seeded.id);
		await dialog.getByRole("button", { name: "Start export" }).click();

		await expect(dialog.locator("[data-phase='succeeded']")).toBeVisible({
			timeout: 200_000,
		});
		expect(refused).toBe(1);
		expect(bodies).toHaveLength(1);
		await expect(dialog.locator("[data-batch-item]")).toHaveCount(1);
		await expect(dialog.getByRole("link", { name: /^Download/ })).toBeVisible();
	});

	// VV-223: the dialog offers what the CLI and the API have.
	test("the dialog offers the formats and the range of the API", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const dialog = await openExport(page, seeded.id);
		const format = dialog.locator("#export-format");
		await expect(format.locator("option")).toHaveText([
			"MP4 video",
			"GIF animation",
			"WAV audio only",
			"AAC audio only",
			"PNG image of the frame at the playhead",
		]);

		await page.keyboard.press("Escape");
		const ruler = page.getByRole("slider", { name: "Playhead" });
		await ruler.focus();
		await page.keyboard.press("PageUp");
		await page.keyboard.press("i");
		await page.keyboard.press("PageUp");
		await page.keyboard.press("o");
		await page.getByRole("button", { name: "Export", exact: true }).click();
		await expect(
			page
				.getByRole("dialog")
				.getByLabel("Export only the range between the in and out points"),
		).toBeVisible();
	});

	test("the codec choice names what this machine cannot write", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "3s",
		});
		const dialog = await openExport(page, seeded.id);
		const codec = dialog.locator("#export-codec");
		await expect(codec.locator("option").first()).toHaveText(
			"Automatic (as the platform prefers)",
		);
		await expect(codec.locator("option")).toHaveCount(4);
		// The core answers which codecs the machine writes; the select follows it.
		await expect
			.poll(
				async () =>
					await codec
						.locator("option[value='h264']")
						.evaluate((o) => (o as HTMLOptionElement).disabled),
				{ timeout: 60_000 },
			)
			.toBe(false);
	});
});

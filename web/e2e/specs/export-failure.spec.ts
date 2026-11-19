import { expect, seedProject, test } from "./helpers";

// Scenario 9 of SDD-012 section 20.1: the server runs with an encoder that
// cannot work (STUARTY_ENCODER, the second web server of the config) and
// the export dialog shows the code headline with its hint.
test.describe("export-failure", () => {
	test("the dialog shows E_NO_ENCODER or E_EXPORT_FAILED with its hint", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Export" }).click();
		await page.getByRole("button", { name: "Start export" }).click();

		const dialog = page.getByRole("dialog");
		await expect(
			dialog.getByText(/No encoder available|Export failed/),
		).toBeVisible({ timeout: 60_000 });
		// The hint follows the headline: worded in the language of the page when
		// the catalogue has the code (VV-126), else the core's English, marked
		// lang="en". The code is shown too.
		const alert = dialog.getByRole("alert");
		await expect(
			alert
				.getByText("No H.264 encoder works on this machine.")
				.or(alert.locator("[lang='en']"))
				.first(),
		).toBeVisible();
		await expect(
			alert.getByText(/^E_(NO_ENCODER|EXPORT_FAILED)$/),
		).toBeVisible();
		await expect(dialog.getByRole("link", { name: "Download" })).toHaveCount(0);
	});
});

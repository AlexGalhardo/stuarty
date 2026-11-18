import { expect, test } from "./helpers";

// VV-077: the settings page tells whether the transcription helper is installed and offers the
// install to a person. A session the server refuses sees the command to run in a terminal. The
// install itself is never pressed against the real server: it would download the helper.
test.describe("settings transcription", () => {
	test("shows the state and the install button", async ({ page }) => {
		await page.goto("/settings");
		await expect(
			page.getByRole("heading", { name: "Transcription", level: 2 }),
		).toBeVisible();
		await expect(page.getByTestId("transcription-state")).toHaveText(
			/Installed\. Models: .+|Not installed yet\./,
		);
		await expect(
			page.getByRole("button", { name: "Install transcription" }),
		).toBeVisible();
	});

	test("a refused install shows the command to run in a terminal", async ({
		page,
	}) => {
		await page.route("**/api/v1/system/tools/install", (route) =>
			route.fulfill({
				status: 403,
				contentType: "application/json",
				json: {
					code: "E_FORBIDDEN",
					message: "only a person can install the transcription helper.",
				},
			}),
		);
		await page.goto("/settings");
		await page.getByLabel("Model", { exact: true }).selectOption("base");
		await page.getByRole("button", { name: "Install transcription" }).click();
		await expect(page.getByLabel("Install command")).toHaveValue(
			"vukem-video tools:install whisper --model base",
		);
	});
});

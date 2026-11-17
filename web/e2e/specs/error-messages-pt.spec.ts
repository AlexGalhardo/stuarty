import { expect, seedProject, test } from "./helpers";

// VV-126: the body and the hint of an error of the core are worded in the
// language of the page, from the code and the `details`.
test.describe("error messages in pt-BR", () => {
	test.beforeEach(async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
	});

	test("E_OVERLAP names the other item and the next free position in Portuguese", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		await items.nth(1).click();
		const start = page.getByLabel("Início", { exact: true }).first();
		await start.fill("0.5");
		await start.press("Enter");
		const alert = page.locator('[data-notice-code="E_OVERLAP"]').first();
		await expect(alert).toContainText("Ficaria sobre");
		await expect(alert).toContainText("próxima posição livre");
		await expect(alert).not.toContainText("would occupy");
		await expect(alert.locator('[lang="en"]')).toHaveCount(0);
	});

	test("E_VALIDATION with accepted values is worded in Portuguese", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		// The one answer the core does not give for a field the inspector accepts.
		await page.route("**/api/v1/projects/*/edit", (route) =>
			route.fulfill({
				status: 400,
				contentType: "application/json",
				body: JSON.stringify({
					code: "E_VALIDATION",
					message: 'args.mode is "x", which is not one of the accepted values.',
					hint: "Use one of: normal, multiply.",
					retryable: false,
					details: { allowed: ["normal", "multiply"] },
				}),
			}),
		);
		await page.goto(`/editor/${seeded.id}`);
		await page.locator("[data-item-id]").click();
		const crop = page.getByLabel("Corte à esquerda").first();
		await crop.fill("0.25");
		await crop.press("Enter");
		const alert = page.getByRole("alert").first();
		await expect(alert).toContainText(
			"Os valores aceitos são: normal, multiply.",
		);
		await expect(alert).not.toContainText("accepted values");
	});
});

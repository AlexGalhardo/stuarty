import { expect, seedProject, test } from "./helpers";

// End-to-end verification for epic 113: when Stuarty runs in pt-BR, error
// dialogs, lint findings, alerts, and error pages render clear Brazilian
// Portuguese messages and recovery hints instead of raw English backend strings.
test.describe("pt-BR error localization (epic 113)", () => {
	test.beforeEach(async ({ page }) => {
		await page.addInitScript(() => {
			localStorage.setItem("vv.lang", "pt-BR");
		});
	});

	test("404 error page renders Portuguese guidance and localized error summary", async ({
		page,
	}) => {
		await page.goto("/projects/prj_0000000000");
		await expect(
			page.getByRole("heading", {
				name: "Não encontramos esta página",
				level: 1,
			}),
		).toBeVisible();
		await expect(page.getByText("Como resolver:")).toBeVisible();
		await expect(
			page.getByRole("link", { name: "Voltar aos projetos" }),
		).toBeVisible();
	});

	test("Export dialog localizes lint warnings and E_EXPORT_FAILED (encoder_failed) into Portuguese", async ({
		page,
	}) => {
		const { id } = await seedProject(page.request, {
			name: "Epic 113 Export Error",
		});

		await page.route(`**/api/v1/projects/${id}/jobs`, async (route) => {
			if (route.request().method() !== "POST") {
				await route.continue();
				return;
			}
			await route.fulfill({
				status: 500,
				contentType: "application/json",
				body: JSON.stringify({
					code: "E_EXPORT_FAILED",
					message: "The video encoder failed.",
					hint: 'Run "stuarty doctor" and try the export again; if it fails again, report it with the output of doctor.',
					details: {
						reason: "encoder_failed",
						encoder: "h264_mf",
					},
				}),
			});
		});

		await page.goto(`/editor/${id}`);
		await expect(page.locator("html")).toHaveAttribute("lang", "pt-BR");

		await page.getByRole("button", { name: "Exportar" }).first().click();
		const dialog = page.getByRole("dialog");
		await expect(dialog).toBeVisible();

		// Wait until lint finishes and the Start export button is enabled.
		const startExport = dialog.getByRole("button", {
			name: "Iniciar exportação",
		});
		await expect(startExport).toBeEnabled({ timeout: 15_000 });

		// Any L_LOUDNESS warning shown before export must be in Portuguese, never raw English.
		await expect(dialog).not.toContainText("the estimated mix loudness");
		await expect(dialog).not.toContainText("adjust volumes or accept");

		await startExport.click();

		const alert = dialog.getByRole("alert");
		await expect(alert).toBeVisible();
		await expect(alert).toContainText("Falha na exportação");
		await expect(alert).toContainText("O codificador de vídeo falhou.");
		await expect(alert).toContainText('Execute "stuarty doctor"');
		await expect(alert).not.toContainText("The video encoder failed");
	});

	test("Dashboard FailureNote localizes E_WORKSPACE_LOCKED into Portuguese", async ({
		page,
	}) => {
		await page.route("**/api/v1/assets**", async (route) => {
			if (route.request().method() !== "GET") {
				await route.continue();
				return;
			}
			await route.fulfill({
				status: 503,
				contentType: "application/json",
				body: JSON.stringify({
					code: "E_WORKSPACE_LOCKED",
					message: "Another process owns the workspace and cannot be reached.",
					hint: "Close the other instance and try again.",
				}),
			});
		});

		await page.goto("/video-assets");
		await expect(page.locator("html")).toHaveAttribute("lang", "pt-BR");

		const alert = page.getByRole("alert");
		await expect(alert).toBeVisible();
		await expect(alert).toContainText("O espaço de trabalho está bloqueado");
		await expect(alert).toContainText(
			"Outro processo do Stuarty está usando este espaço de trabalho.",
		);
		await expect(alert).not.toContainText("Another process owns the workspace");
	});
});

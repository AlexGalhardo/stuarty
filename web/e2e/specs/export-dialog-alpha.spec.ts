import { caller, expect, seedProject, test } from "./helpers";

// P2-UI-028 (SDD-012 sections 14.1, 14.2 and 22.2 item 9): the export dialog
// cancels a running export, lists the failed checks of a verification, names
// the lint findings and finds a running export again after a reload.
test.describe("export-dialog-alpha", () => {
	test("cancel mid-export ends in cancelled, shown as a status and not as an error", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, {
			tracks: 2,
			perTrack: 1,
			clipDuration: "10s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await page.getByRole("button", { name: "Export" }).click();
		await page.getByRole("button", { name: "Start export" }).click();

		const dialog = page.getByRole("dialog");
		await expect(dialog.locator("[data-phase='running']")).toBeVisible();
		await dialog.getByRole("button", { name: "Cancel export" }).click();

		await expect(dialog.locator("[data-phase='cancelled']")).toBeVisible({
			timeout: 60_000,
		});
		await expect(dialog.getByText("Export cancelled")).toBeVisible();
		await expect(dialog.getByRole("alert")).toHaveCount(0);
		await expect(page.getByRole("alert")).toHaveCount(0);
		await expect(
			dialog.getByRole("button", { name: "Export again" }),
		).toBeVisible();
	});

	test("a verification failure lists each failed check", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		// The real server has no way to make a file fail its checks, so the
		// answer of the start request is the one the core gives for it.
		await page.route("**/api/v1/projects/*/jobs", async (route) => {
			if (route.request().method() !== "POST") {
				await route.fallback();
				return;
			}
			await route.fulfill({
				status: 201,
				contentType: "application/json",
				body: JSON.stringify({
					job_id: "job_verifyfail",
					kind: "export",
					project_id: seeded.id,
					state: "failed",
					progress: 1,
					actor: "local:ui",
					created_at: "2026-11-15T12:00:00Z",
					text: "failed",
					error: {
						code: "E_VERIFY_FAILED",
						message: "The exported file failed 2 check(s) and was deleted.",
						hint: "Export again.",
						details: {
							checks: [
								{ name: "duration", expected: "1.000 s", got: "0.500 s" },
								{ name: "dimensions", expected: "1080x1920", got: "1080x1918" },
							],
						},
					},
				}),
			});
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Export" }).click();
		await page.getByRole("button", { name: "Start export" }).click();

		const dialog = page.getByRole("dialog");
		await expect(dialog.getByText("E_VERIFY_FAILED")).toBeVisible();
		const checks = dialog.locator("[data-verify-checks] li");
		await expect(checks).toHaveCount(2);
		await expect(checks.first()).toContainText("duration");
		await expect(checks.first()).toContainText("expected 1.000 s, got 0.500 s");
		await expect(checks.last()).toContainText("dimensions");
		await expect(dialog.getByText("[object Object]")).toHaveCount(0);
	});

	test("a finding has the localized rule name and a link to its item", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		const out = (await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "text.add",
					args: {
						track_id: seeded.trackIds[0],
						start: "0s",
						duration: "3s",
						text: "Hello",
					},
				},
			],
		})) as { created: string[] };
		const itemId = out.created[0] as string;
		const marked = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: marked.revision,
			operations: [
				{ op: "slot.mark", args: { item_id: itemId, name: "title" } },
			],
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Export" }).click();

		const dialog = page.getByRole("dialog");
		await expect(dialog.getByText("Template slot not filled")).toBeVisible();
		await expect(dialog.getByText("L_SLOT_UNFILLED")).toBeVisible();
		await dialog.getByRole("button", { name: `Go to ${itemId}` }).click();
		await expect(dialog).toHaveCount(0);
		await expect(page.getByRole("heading", { name: /^Text/ })).toBeVisible();
	});

	test("a reload during the export keeps the chip and reopens the dialog at Running", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, {
			tracks: 2,
			perTrack: 1,
			clipDuration: "10s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await page.getByRole("button", { name: "Export" }).click();
		await page.getByRole("button", { name: "Start export" }).click();
		await expect(
			page.getByRole("dialog").locator("[data-phase='running']"),
		).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(page.getByRole("dialog")).toHaveCount(0);

		await page.reload();
		const dialog = page.getByRole("dialog");
		await expect(dialog.locator("[data-phase='running']")).toBeVisible();
		await expect(dialog.getByText(/^Revision [0-9]+$/)).toBeVisible();

		// Closed, the export is still there in the chip, and the chip reopens it.
		await page.keyboard.press("Escape");
		await expect(dialog).toHaveCount(0);
		const chip = page.getByRole("button", { name: /^1 job · [0-9]+%$/ });
		await expect(chip).toBeVisible();
		await chip.click();
		await expect(dialog.locator("[data-phase='running']")).toBeVisible();

		await dialog.getByRole("button", { name: "Cancel export" }).click();
		await expect(dialog.locator("[data-phase='cancelled']")).toBeVisible({
			timeout: 60_000,
		});
	});
});

import type { Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const edits: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			edits.push(r.postDataJSON());
		}
	});
	return edits;
}

// P2-UI-024 (SDD-012 sections 2.1, 6.2, 12.1 and 16.3): the project panel, the
// top bar name, the language select and the settings page.
test.describe("project-settings", () => {
	test("changing the aspect sends one settings.reframe", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const preset = page.getByLabel("Change aspect");
		await expect(preset).toHaveValue("9:16");
		const edits = recordEdits(page);

		await preset.selectOption("1:1");
		await expect(page.getByLabel("Height", { exact: true })).toHaveValue(
			"1080",
		);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toEqual([
			{ op: "settings.reframe", args: { aspect: "1:1" } },
		]);
		await expect(preset).toHaveValue("1:1");
	});

	test("the frame rate asks first and lists what the core adjusted afterwards", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		// A one-frame item at 60 fps turns into zero frames at 24 fps.
		const call = await caller(request, "e2e-setup");
		const project = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		const first = (await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: project.revision,
			operations: [{ op: "settings.set_fps", args: { fps: "60" } }],
		})) as { revision: number };
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: first.revision,
			operations: [
				{
					op: "clip.add",
					args: {
						track_id: seeded.trackIds[0],
						asset_id: seeded.assetId,
						start: "0f",
						source_in: "0f",
						duration: "1f",
					},
				},
			],
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const edits = recordEdits(page);

		await page.getByLabel("Frame rate").selectOption("24");
		const dialog = page.getByRole("dialog");
		await expect(dialog).toContainText("Change the frame rate to 24?");
		expect(edits).toHaveLength(0);

		const answered = page.waitForResponse(
			(r) => r.url().endsWith("/edit") && r.status() === 200,
		);
		await dialog.getByRole("button", { name: "Change frame rate" }).click();
		const body = (await (await answered).json()) as {
			effective: { op: string; effective: { adjusted?: unknown[] } }[];
		};
		const adjusted =
			body.effective.find((e) => e.op === "settings.set_fps")?.effective
				.adjusted ?? [];

		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toEqual([
			{ op: "settings.set_fps", args: { fps: "24" } },
		]);
		expect(adjusted.length).toBeGreaterThan(0);
		await expect(dialog).toContainText("The frame rate is now 24");
		await expect(dialog.getByRole("listitem")).toHaveCount(adjusted.length);
		await expect(dialog).toContainText(`${adjusted.length} item`);
		await dialog.getByRole("button", { name: "Done" }).click();
		await expect(dialog).toHaveCount(0);
		await expect(page.getByLabel("Frame rate")).toHaveValue("24");
	});

	test("cancelling the frame rate dialog sends nothing", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const edits = recordEdits(page);
		await page.getByLabel("Frame rate").selectOption("25");
		await page.getByRole("button", { name: "Cancel" }).click();
		await expect(page.getByRole("dialog")).toHaveCount(0);
		await expect(page.getByLabel("Frame rate")).toHaveValue("30");
		expect(edits).toHaveLength(0);
	});

	test("the top bar name renames the project with one project.rename", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const edits = recordEdits(page);

		await page.getByRole("button", { name: /^Rename project/ }).click();
		const field = page.getByRole("textbox", { name: "Project name" }).first();
		await field.fill("A new name");
		await field.press("Enter");

		await expect(page.getByRole("heading", { level: 1 })).toContainText(
			"A new name",
		);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toEqual([
			{ op: "project.rename", args: { name: "A new name" } },
		]);
	});

	test("the language select keeps the locale and reloads at the same playhead", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?t=45f`);
		await expect(
			page.getByRole("link", { name: "Back to projects" }),
		).toBeVisible();

		const reloaded = page.waitForRequest(
			(r) => r.isNavigationRequest() && r.url().includes("t=45f"),
		);
		await page.getByRole("combobox", { name: "Language" }).click();
		await page.getByRole("option", { name: "Português (Brasil)" }).click();
		expect(await page.evaluate(() => localStorage.getItem("vv.lang"))).toBe(
			"pt-BR",
		);
		await reloaded;

		await expect(
			page.getByRole("link", { name: "Voltar aos projetos" }),
		).toBeVisible();
		await expect(
			page.locator('[role="slider"][aria-valuenow="45"]'),
		).toBeVisible();
	});

	test("/settings loads in pt-BR and shows the version and the workspace path", async ({
		page,
	}) => {
		await page.addInitScript(() => {
			localStorage.setItem("vv.lang", "pt-BR");
		});
		const response = await page.goto("/settings");
		expect(response?.status()).toBe(200);
		await expect(
			page.getByRole("heading", { name: "Configurações", level: 1 }),
		).toBeVisible();
		await expect(page.getByTestId("settings-version")).not.toBeEmpty();
		await expect(page.getByTestId("settings-workspace")).not.toBeEmpty();
	});
});

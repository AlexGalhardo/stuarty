import { mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";
import { openSection } from "./sections";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

// `settled.count` is how many of the posted edits the server has answered. A
// field builds its batch from the document it shows, so a test that edits the
// same item again must wait for the answer, not for the request: the next
// field sent in between would carry the old values.
function recordEdits(page: Page, settled = { count: 0 }): Posted[] {
	const posted: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			posted.push(r.postDataJSON() as Posted);
		}
	});
	page.on("requestfinished", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) settled.count++;
	});
	return posted;
}

// A project with one visual track and, through the API, one text item.
async function seedText(
	request: Parameters<typeof seedProject>[0],
): Promise<{ id: string; itemId: string }> {
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
	return { id: seeded.id, itemId: out.created[0] as string };
}

// P2-UI-027 (SDD-012 sections 9.1 and 12.1): a slot is marked in the inspector,
// filled in the slots panel, and an open required slot is a finding of the
// export dialog; each action is one command.
test.describe("templates", () => {
	test("mark a slot, see the finding in the export dialog, fill it; one command each", async ({
		page,
		request,
	}) => {
		const seeded = await seedText(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const item = page.locator("[data-item-id]");
		await expect(item).toHaveCount(1);
		await item.click();
		const posted = recordEdits(page);

		await page.getByLabel("Template slot", { exact: true }).check();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "slot.mark",
				args: {
					item_id: seeded.itemId,
					name: "slot_1",
					description: "",
					required: true,
				},
			},
		]);
		const name = page.getByLabel("Slot name", { exact: true });
		await name.fill("title");
		await name.press("Enter");
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations).toEqual([
			{ op: "slot.unmark", args: { item_id: seeded.itemId } },
			{
				op: "slot.mark",
				args: {
					item_id: seeded.itemId,
					name: "title",
					description: "",
					required: true,
				},
			},
		]);

		const description = page.getByLabel("Slot description", { exact: true });
		await description.fill("Headline");
		await description.press("Enter");
		await expect.poll(() => posted.length).toBe(3);
		expect(posted[2]?.operations).toEqual([
			{
				op: "slot.mark",
				args: {
					item_id: seeded.itemId,
					name: "title",
					description: "Headline",
					required: true,
				},
			},
		]);

		await page.getByRole("button", { name: "Export" }).click();
		const dialog = page.getByRole("dialog");
		await expect(dialog.getByText("L_SLOT_UNFILLED")).toBeVisible();
		await dialog.getByRole("button", { name: "Close" }).click();
		await expect(dialog).toHaveCount(0);

		// Nothing selected: the project panel with the slots panel.
		await item.click({ modifiers: ["Control"] });
		const row = page.locator("[data-slot-row='title']");
		await expect(row).toBeVisible();
		await expect(page.locator("[data-unfilled]")).toContainText(
			"1 required slot is not filled",
		);
		await row.getByLabel("Text", { exact: true }).fill("Big news");
		await row.getByRole("button", { name: "Fill" }).click();
		await expect.poll(() => posted.length).toBe(4);
		expect(posted[3]?.operations).toEqual([
			{ op: "slot.fill", args: { name: "title", text: "Big news" } },
		]);
		await expect(row).toContainText("Filled");
		await expect(page.locator("[data-unfilled]")).toHaveCount(0);
		await expect(item).toContainText("Big news");

		await page.getByRole("button", { name: "Export" }).click();
		await expect(
			page.getByRole("dialog").getByText("L_SLOT_UNFILLED"),
		).toHaveCount(0);
	});

	test("the toggle off unmarks the slot, and the project panel sets the template info", async ({
		page,
		request,
	}) => {
		const seeded = await seedText(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const item = page.locator("[data-item-id]");
		await expect(item).toHaveCount(1);
		await item.click();
		const settled = { count: 0 };
		const posted = recordEdits(page, settled);

		await page.getByLabel("Template slot", { exact: true }).check();
		const name = page.getByLabel("Slot name", { exact: true });
		await name.fill("headline");
		await name.press("Enter");
		await expect.poll(() => settled.count).toBe(2);
		await page.getByLabel("Required", { exact: true }).uncheck();
		await expect.poll(() => settled.count).toBe(3);
		expect(posted[2]?.operations).toEqual([
			{
				op: "slot.mark",
				args: {
					item_id: seeded.itemId,
					name: "headline",
					description: "",
					required: false,
				},
			},
		]);
		await page.getByLabel("Template slot", { exact: true }).uncheck();
		await expect.poll(() => posted.length).toBe(4);
		expect(posted[3]?.operations).toEqual([
			{ op: "slot.unmark", args: { item_id: seeded.itemId } },
		]);

		await item.click({ modifiers: ["Control"] });
		await openSection(page, "Template");
		const info = page.getByLabel("Template description", { exact: true });
		await expect(info).toBeVisible();
		await info.fill("A short template");
		await info.press("Enter");
		await expect.poll(() => posted.length).toBe(5);
		expect(posted[4]?.operations).toEqual([
			{
				op: "template.set_info",
				args: { description: "A short template" },
			},
		]);
	});

	// VV-132: the project panel saves the project as a template, and shows the answer.
	test("the project panel saves the project as a template", async ({
		page,
		request,
	}) => {
		const seeded = await seedText(request);
		await page.goto(`/editor/${seeded.id}`);
		const item = page.locator("[data-item-id]");
		await expect(item).toHaveCount(1);
		await item.click();
		await page.getByLabel("Template slot", { exact: true }).check();
		await item.click({ modifiers: ["Control"] });
		const name = `Saved ${Date.now()}`;
		await openSection(page, "Save as template");
		await page.getByLabel("Template name", { exact: true }).fill(name);
		await page
			.getByRole("button", { name: "Save template", exact: true })
			.click();
		await expect(
			page.getByRole("status").filter({ hasText: name }),
		).toBeVisible();
		await page.goto("/projects");
		await expect(
			page.locator("[data-template-id]", { hasText: name }),
		).toBeVisible();
	});

	test("the projects page lists the starter templates and makes a project from one", async ({
		page,
	}) => {
		await page.goto("/projects");
		const rows = page.locator("[data-template-id]");
		await expect(rows.first()).toBeVisible();
		await expect(
			page.getByRole("link", { name: /^Export file/ }).first(),
		).toHaveAttribute("href", /\/api\/v1\/templates\/.+\/file$/);

		await rows
			.first()
			.getByRole("button", { name: /^New from template/ })
			.click();
		await rows.first().getByLabel("Name").fill("From a template");
		await rows.first().getByRole("button", { name: "Create project" }).click();
		await expect(page).toHaveURL(/\/editor\/prj_/);
		await expect(page.getByRole("toolbar", { name: "Editing" })).toBeVisible();
	});

	// VV-054: in the desktop window the native dialog fills the template path;
	// a browser has no such button.
	test("the desktop window picks the template path with its native dialog", async ({
		page,
	}) => {
		await page.addInitScript(() => {
			(window as unknown as Record<string, unknown>).vukemDesktop = {
				pickTemplate: () =>
					Promise.resolve(["C:\tspring.stuarty-template.zip"]),
			};
		});
		await page.goto("/projects");
		await page.getByRole("button", { name: "Choose file" }).click();
		await expect(
			page.getByLabel("Path of a template file on this computer"),
		).toHaveValue("C:\tspring.stuarty-template.zip");
	});

	test("a browser has no template file dialog", async ({ page }) => {
		await page.goto("/projects");
		await expect(
			page.getByLabel("Path of a template file on this computer"),
		).toBeVisible();
		await expect(page.getByRole("button", { name: "Choose file" })).toHaveCount(
			0,
		);
	});

	// VV-128: importing the same file again says so and adds no second template.
	test("importing the same template file twice says it was already imported", async ({
		page,
	}) => {
		await page.goto("/projects");
		const rows = page.locator("[data-template-id]");
		await expect(rows.first()).toBeVisible();
		const id = (await rows.first().getAttribute("data-template-id")) as string;
		const file = await page.request.get(`/api/v1/templates/${id}/file`);
		expect(file.ok()).toBe(true);
		const dir = join(tmpdir(), "stuarty-e2e-template-import");
		mkdirSync(dir, { recursive: true });
		const path = join(dir, `again-${Date.now()}.stuarty-template.zip`);
		writeFileSync(path, await file.body());

		const field = page.getByLabel("Path of a template file on this computer");
		await field.fill(path);
		await page.getByRole("button", { name: "Import template" }).click();
		await expect(
			page.getByRole("status").filter({ hasText: "Imported:" }),
		).toBeVisible();
		const count = await rows.count();

		await field.fill(path);
		await page.getByRole("button", { name: "Import template" }).click();
		await expect(
			page.getByText("This file was already imported, so nothing was added."),
		).toBeVisible();
		await expect(rows).toHaveCount(count);
	});
});

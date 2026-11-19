import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

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

// TASK-0290 (SDD-012 section 6.4): the timeline toolbar holds the tools, undo
// and redo, the cuts, delete, captions, the modes and the zoom.
test.describe("timeline toolbar", () => {
	test("the edit tools group holds the tools, undo, cuts, delete and captions", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const tools = page.getByRole("group", { name: "Edit tools" });
		for (const name of [
			"Select",
			"Blade",
			"Undo",
			"Redo",
			"Split",
			"Trim start",
			"Trim end",
			"Delete",
			"Add caption",
		]) {
			await expect(
				tools.getByRole("button", { name, exact: true }),
			).toBeVisible();
		}
		const modes = page.getByRole("group", { name: "Modes and zoom" });
		await expect(modes.getByRole("slider", { name: "Zoom" })).toBeVisible();
		await expect(modes.getByRole("button", { name: "Magnet" })).toBeVisible();
	});

	test("Select and Blade are a pair, and the blade cuts with item.split", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const edits = recordEdits(page);
		const select = page.getByRole("button", { name: "Select", exact: true });
		const blade = page.getByRole("button", { name: "Blade", exact: true });
		await expect(select).toHaveAttribute("aria-pressed", "true");
		await expect(blade).toHaveAttribute("aria-pressed", "false");
		await blade.click();
		await expect(select).toHaveAttribute("aria-pressed", "false");
		await expect(blade).toHaveAttribute("aria-pressed", "true");
		const box = await page.locator("[data-item-id]").first().boundingBox();
		if (box === null) throw new Error("no item box");
		await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		expect(edits.map((e) => e.operations.map((o) => o.op))).toEqual([
			["item.split"],
		]);
		await select.click();
		await expect(blade).toHaveAttribute("aria-pressed", "false");
	});

	test("the zoom slider changes the scale and the buttons follow it", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const slider = page.getByRole("slider", { name: "Zoom" });
		const before = Number(await slider.getAttribute("aria-valuenow"));
		await page.getByRole("button", { name: "Zoom in" }).click();
		await expect
			.poll(async () => Number(await slider.getAttribute("aria-valuenow")))
			.toBeGreaterThan(before);
		await slider.focus();
		await page.keyboard.press("Home");
		// The Base UI slider is a native range input: its lower bound is `min`.
		await expect(slider).toHaveAttribute(
			"aria-valuenow",
			(await slider.getAttribute("min")) ?? "",
		);
	});

	test("Link is pressed when the selected items are linked", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 2,
			clipDuration: "1s",
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		const link = page.getByRole("button", { name: "Link", exact: true });
		await expect(link).toBeDisabled();
		await page.locator("[data-item-id]").first().click();
		await page
			.locator("[data-item-id]")
			.nth(1)
			.click({ modifiers: ["Shift"] });
		await expect(link).toBeEnabled();
		await expect(link).toHaveAttribute("aria-pressed", "false");
		await link.click();
		await expect(link).toHaveAttribute("aria-pressed", "true");
	});

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`axe finds nothing in the toolbar in ${theme} in ${lang}`, async ({
				page,
				request,
			}) => {
				await page.addInitScript(
					([l, th]) => {
						localStorage.setItem("vv.lang", l);
						if (th === "light") localStorage.setItem("theme", "light");
					},
					[lang, theme],
				);
				const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
				await page.goto(`/editor/${seeded.id}`);
				await expect(page.locator("[data-item-id]")).toHaveCount(1);
				const { violations } = await new AxeBuilder({ page })
					.include("section[data-landmark='4']")
					.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
					.analyze();
				expect(violations.map((v) => v.id)).toEqual([]);
			});
		}
	}
});

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

async function openMenu(page: Page, submenu: string): Promise<void> {
	await page.getByRole("button", { name: "Menu", exact: true }).click();
	await page.getByRole("menuitem", { name: submenu }).click();
}

// TASK-0292 (SDD-012 section 6.4): the editor menu of the top bar and the menu
// of a clip.
test.describe("editor menu", () => {
	test("each item calls the action of the page and shows the key of the cheat sheet", async ({
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

		await openMenu(page, "Clip");
		const split = page.getByRole("menuitem", { name: /^Split/ });
		await expect(split).toContainText(/(Ctrl|Cmd)\+B/);
		await expect(split).toBeDisabled();
		await page.keyboard.press("Escape");
		await page.keyboard.press("Escape");

		await openMenu(page, "Edit");
		await page.getByRole("menuitem", { name: /^Select all/ }).click();
		await expect(
			page.locator("[data-item-id][aria-selected='true']"),
		).toHaveCount(1);

		await openMenu(page, "Timeline");
		await page.getByRole("menuitem", { name: "Add audio track" }).click();
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{ op: "track.add", args: { kind: "audio", position: "bottom" } },
		]);

		await openMenu(page, "File");
		await page.getByRole("menuitem", { name: /^Export/ }).click();
		await expect(page.getByRole("dialog")).toBeVisible();
		await page.keyboard.press("Escape");

		await openMenu(page, "Help");
		await page.getByRole("menuitem", { name: "Keyboard shortcuts" }).click();
		await expect(page.getByRole("dialog")).toContainText("Keyboard shortcuts");
	});

	test("the Menu button is not drawn in the desktop shell", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.addInitScript(() => {
			(window as unknown as Record<string, unknown>).vukemDesktop = {};
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await expect(
			page.getByRole("button", { name: "Menu", exact: true }),
		).toHaveCount(0);
	});

	test("the clip menu has trim, speed, copy, cut and paste with their keys", async ({
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
		await page.locator("[data-item-id]").first().click({ button: "right" });
		for (const name of [
			/^Trim start/,
			/^Trim end/,
			/^Speed/,
			/^Cut/,
			/^Paste(?! attributes)/,
		]) {
			await expect(page.getByRole("menuitem", { name })).toBeVisible();
		}
		await expect(
			page.getByRole("menuitem", { name: /^Copy(?! attributes)/ }),
		).toContainText(/(Ctrl|Cmd)\+C/);
		await page
			.getByRole("menuitem", { name: /^Copy(?! attributes)/ })
			.first()
			.click();
		// The core refuses a paste onto the item it was copied from (E_OVERLAP), so
		// the playhead goes to the end first.
		await page.getByRole("button", { name: "Go to end" }).click();
		await page.locator("[data-item-id]").first().click({ button: "right" });
		await page
			.getByRole("menuitem", { name: /^Paste(?! attributes)/ })
			.first()
			.click();
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		expect(edits.at(-1)?.operations.map((o) => o.op)).toEqual([
			"item.duplicate",
		]);
	});

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`axe finds nothing in the open menu in ${theme} in ${lang}`, async ({
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
				await page.getByRole("button", { name: /^Menu/ }).click();
				await page.getByRole("menuitem").first().focus();
				await page.keyboard.press("ArrowRight");
				await expect(page.getByRole("menu").last()).toBeVisible();
				await page.evaluate(() =>
					Promise.all(
						document
							.getAnimations()
							.map((a) => a.finished.catch(() => undefined)),
					),
				);
				const { violations } = await new AxeBuilder({ page })
					.include("[role='menu']")
					.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
					.analyze();
				expect(violations.map((v) => v.id)).toEqual([]);
			});
		}
	}
});

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

// TASK-0291 (SDD-012 section 6.4): the player row has the time at the left, the
// transport in the centre and the aspect ratio, Fit, safe area and full screen
// at the right.
test.describe("player row", () => {
	test("the ratio select sends settings.reframe and shows the size the core answered", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const row = page.getByRole("toolbar", { name: "Playback" });
		const ratio = row.getByLabel("Aspect ratio");
		await expect(ratio).toHaveValue("9:16");
		const edits = recordEdits(page);
		await ratio.selectOption("16:9");
		await expect(ratio).toHaveValue("16:9");
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toEqual([
			{ op: "settings.reframe", args: { aspect: "16:9" } },
		]);
		await expect(page.getByLabel("Width", { exact: true })).toHaveValue("1920");
	});

	test("Fit, the safe area and full screen are buttons of the row", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const row = page.getByRole("toolbar", { name: "Playback" });
		await expect(row.getByRole("button", { name: "Fit" })).toHaveAttribute(
			"aria-pressed",
			"true",
		);
		const safe = row.getByRole("button", { name: "Safe area" });
		await safe.click();
		await expect(safe).toHaveAttribute("aria-pressed", "true");
		await expect(
			row.getByRole("button", { name: "Full screen preview" }),
		).toBeVisible();
		const timeBox = await row.getByLabel("Current time").boundingBox();
		const ratioBox = await row.getByLabel("Aspect ratio").boundingBox();
		const playBox = await row
			.getByRole("button", { name: "Play" })
			.boundingBox();
		expect((timeBox?.x ?? 1) < (playBox?.x ?? 0)).toBe(true);
		expect(ratioBox).not.toBeNull();
	});

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`axe finds nothing in the player row in ${theme} in ${lang}`, async ({
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
					.include("main[data-landmark='3']")
					.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
					.analyze();
				expect(violations.map((v) => v.id)).toEqual([]);
			});
		}
	}
});

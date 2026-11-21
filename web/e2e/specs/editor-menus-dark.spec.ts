import type { Locator, Page } from "@playwright/test";

import { expect, seedProject, test } from "./helpers";

// Epic 60 (D-125, SDD-012 section 6.4): in the dark theme every menu of the
// editor, the portal menus and the lists of the native selects, has a dark
// background under light text, with the contrast of WCAG AA.
test.describe("editor menus in the dark theme", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	interface Colours {
		background: string;
		text: string;
	}

	async function coloursOf(target: Locator): Promise<Colours> {
		return target.evaluate((el) => {
			const style = getComputedStyle(el);
			return { background: style.backgroundColor, text: style.color };
		});
	}

	// getComputedStyle returns rgb(...) or rgba(...) for sRGB and color(srgb ...)
	// or oklch(...) for the wider spaces, so the colour is drawn on a canvas and
	// read back as plain sRGB.
	async function luminances(
		page: Page,
		colours: Colours,
	): Promise<{ background: number; text: number }> {
		return page.evaluate((c) => {
			const toRgb = (css: string): [number, number, number] => {
				const canvas = document.createElement("canvas");
				canvas.width = canvas.height = 1;
				const ctx = canvas.getContext("2d", { willReadFrequently: true });
				if (ctx === null) throw new Error("no 2d context");
				ctx.fillStyle = "#000";
				ctx.fillRect(0, 0, 1, 1);
				ctx.fillStyle = css;
				ctx.fillRect(0, 0, 1, 1);
				const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
				return [r ?? 0, g ?? 0, b ?? 0];
			};
			const lum = ([r, g, b]: [number, number, number]): number => {
				const lin = (v: number): number => {
					const s = v / 255;
					return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
				};
				return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
			};
			return { background: lum(toRgb(c.background)), text: lum(toRgb(c.text)) };
		}, colours);
	}

	// The list of a native select is painted by the browser from the colours of
	// its options: a transparent one opens a light list under light text, which a
	// luminance read through the canvas (transparent over black) would not tell.
	async function expectOpaque(target: Locator): Promise<void> {
		const { background } = await coloursOf(target);
		expect(background, "the option paints its own background").not.toMatch(
			/^(transparent|rgba\(\d+, \d+, \d+, 0(\.0+)?\))$/,
		);
	}

	async function expectDark(page: Page, target: Locator): Promise<void> {
		const lum = await luminances(page, await coloursOf(target));
		expect(lum.background, "the background is dark").toBeLessThan(0.1);
		expect(lum.text, "the text is light").toBeGreaterThan(0.5);
		const ratio =
			(Math.max(lum.background, lum.text) + 0.05) /
			(Math.min(lum.background, lum.text) + 0.05);
		expect(ratio, "contrast of AA").toBeGreaterThanOrEqual(4.5);
	}

	test("the portal menus are dark", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("html")).toHaveClass(/\bdark\b/);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);

		// The menu of the top bar.
		await page.getByRole("button", { name: "Menu", exact: true }).click();
		const bar = page.getByRole("menu").first();
		await expect(bar).toBeVisible();
		await expectDark(page, bar);
		await expectDark(page, bar.getByRole("menuitem").first());
		await page.keyboard.press("Escape");

		// The menu of a clip.
		await page.locator("[data-item-id]").first().click({ button: "right" });
		const clip = page.getByRole("menu").first();
		await expect(clip).toBeVisible();
		await expectDark(page, clip);
		await page.keyboard.press("Escape");

		// The menu of a card of the library.
		await page
			.locator("[data-asset-id]")
			.first()
			.getByRole("button", { name: /^Actions for / })
			.click();
		const card = page.getByRole("menu").first();
		await expect(card).toBeVisible();
		await expectDark(page, card);
		await page.keyboard.press("Escape");
	});

	test("the list of a native select is dark", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);

		// The ratio of the player is a native select.
		const options = page.locator("select option");
		expect(await options.count()).toBeGreaterThan(0);
		await expectOpaque(options.first());
		await expectDark(page, options.first());

		// The frame rate of the project panel of the inspector, the list of the
		// screenshot that started the epic.
		const rate = page.getByLabel("Frame rate", { exact: true });
		await expect(rate).toBeVisible();
		const rates = rate.locator("option");
		expect(await rates.count()).toBeGreaterThan(1);
		await expectOpaque(rates.first());
		await expectDark(page, rates.first());
		await expectDark(page, rate);
	});
});

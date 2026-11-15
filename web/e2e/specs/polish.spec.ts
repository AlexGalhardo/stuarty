import type { Page, Request } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

// P3-UI-003 (SDD-012 sections 7.4, 11.4, 11.5, 17 item 5 and 21.1): interface
// motion that reduced motion turns off, row heights that persist, the preview
// size, volume and mute (screen state only) and the full screen preview.
test.describe.configure({ timeout: 120_000 });

// Every request after this call, so a toggle that reaches the core or any
// other endpoint shows. The long-lived event stream is not a request of a toggle.
function recordRequests(page: Page): Request[] {
	const seen: Request[] = [];
	page.on("request", (r) => {
		if (!r.url().endsWith("/events")) seen.push(r);
	});
	return seen;
}

test.describe("polish", () => {
	test.describe("reduced motion", () => {
		test("the animated elements have a transition of 0s when reduced and 150 ms otherwise", async ({
			page,
			request,
		}) => {
			const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
			await page.goto(`/editor/${seeded.id}?debug=1`);
			const item = page.locator("[data-item-id]").first();
			const row = page.locator("[data-track-id]").first();
			const header = page.locator("[data-header-for]").first();
			const splitter = page.locator("[data-splitter]").first();
			await expect(item).toBeVisible();

			await page.emulateMedia({ reducedMotion: "no-preference" });
			for (const element of [item, row, header, splitter]) {
				await expect(element).toHaveCSS("transition-duration", "0.15s");
			}

			await page.emulateMedia({ reducedMotion: "reduce" });
			for (const element of [item, row, header, splitter]) {
				await expect(element).toHaveCSS("transition-duration", "0s");
			}
		});
	});

	test.describe("track rows", () => {
		test("the splitter resizes the rows of a kind, keeps them after a reload and sends nothing", async ({
			page,
			request,
		}) => {
			const seeded = await seedProject(request, {
				tracks: 1,
				perTrack: 1,
				audioTracks: 1,
			});
			await page.goto(`/editor/${seeded.id}?debug=1`);
			const visual = page.locator(`[data-track-id="${seeded.trackIds[0]}"]`);
			const audio = page.locator(
				`[data-track-id="${seeded.audioTrackIds[0]}"]`,
			);
			await expect(visual).toBeVisible();
			await expect(visual).toHaveCSS("height", "64px");
			await expect(audio).toHaveCSS("height", "48px");

			const requests = recordRequests(page);
			const handle = page.getByRole("separator", {
				name: "Height of video track rows",
			});
			await handle.focus();
			await page.keyboard.press("ArrowDown");
			await page.keyboard.press("Shift+ArrowDown");
			await expect(visual).toHaveCSS("height", "84px");
			await expect(handle).toHaveAttribute("aria-valuenow", "84");
			await expect(audio).toHaveCSS("height", "48px");
			await expect(
				page.locator(`[data-header-for="${seeded.trackIds[0]}"]`),
			).toHaveCSS("height", "84px");

			const box = await handle.boundingBox();
			if (box === null) throw new Error("no splitter box");
			await page.mouse.move(box.x + 20, box.y + box.height / 2);
			await page.mouse.down();
			await page.mouse.move(box.x + 20, box.y + box.height / 2 - 40, {
				steps: 6,
			});
			await page.mouse.up();
			await expect(visual).toHaveCSS("height", "44px");

			await page.keyboard.press("End");
			await expect(visual).toHaveCSS("height", "160px");
			await page.keyboard.press("Home");
			await expect(visual).toHaveCSS("height", "32px");
			expect(
				requests.filter((r) => r.method() !== "GET"),
				"resizing a row must not send a request",
			).toEqual([]);
			expect(await page.evaluate(() => localStorage.getItem("vv.layout"))).toBe(
				'{"rows":{"visual":32,"audio":48}}',
			);

			await page.reload();
			await expect(visual).toHaveCSS("height", "32px");
			await expect(audio).toHaveCSS("height", "48px");

			await page
				.getByRole("separator", { name: /video track rows/ })
				.dblclick();
			await expect(visual).toHaveCSS("height", "64px");
		});
	});

	test.describe("preview", () => {
		test("50% and 100% are shares of the buffer and Fit comes back", async ({
			page,
			request,
		}) => {
			const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
			await page.goto(`/editor/${seeded.id}?debug=1`);
			const canvas = page.getByTestId("canvas-overlay");
			await expect(canvas).toBeVisible();
			const settings = await page.evaluate(
				() => window.__vv?.document?.settings,
			);
			const long = Math.max(settings?.width ?? 1, settings?.height ?? 1);
			const full = Math.min(1, 1080 / long);
			const width = async (): Promise<number> =>
				(await canvas.boundingBox())?.width ?? 0;

			await page.getByRole("button", { name: "Full (100%)" }).click();
			await expect
				.poll(async () =>
					Math.abs((await width()) - (settings?.width ?? 0) * full),
				)
				.toBeLessThan(1.5);
			await expect(
				page.getByRole("button", { name: "Full (100%)" }),
			).toHaveAttribute("aria-pressed", "true");

			await page.getByRole("button", { name: "Half (50%)" }).click();
			await expect
				.poll(async () =>
					Math.abs((await width()) - ((settings?.width ?? 0) * full) / 2),
				)
				.toBeLessThan(1.5);

			const fit = page
				.getByRole("group", { name: "Preview size" })
				.getByRole("button", { name: "Fit" });
			await fit.click();
			await expect(fit).toHaveAttribute("aria-pressed", "true");
		});

		test("volume and mute never reach the core", async ({ page, request }) => {
			const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
			await page.goto(`/editor/${seeded.id}?debug=1`);
			const mute = page.getByRole("button", { name: "Mute preview" });
			await expect(mute).toBeVisible();
			await page.waitForTimeout(500);
			const requests = recordRequests(page);

			const volume = page.getByRole("slider", { name: "Preview volume" });
			await volume.fill("0.4");
			await expect(volume).toHaveValue("0.4");
			await mute.click();
			await expect(
				page.getByRole("button", { name: "Unmute preview" }),
			).toHaveAttribute("aria-pressed", "true");
			await expect(volume).toHaveValue("0");
			await page.getByRole("button", { name: "Unmute preview" }).click();
			await expect(volume).toHaveValue("0.4");
			await page.waitForTimeout(500);

			expect(
				requests.map((r) => `${r.method()} ${r.url()}`),
				"volume and mute are screen state",
			).toEqual([]);
		});

		test("the full screen control shows only where the browser has the API", async ({
			page,
			request,
		}) => {
			const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
			await page.goto(`/editor/${seeded.id}?debug=1`);
			const enter = page.getByRole("button", { name: "Full screen preview" });
			await expect(enter).toBeVisible();
			await enter.click();
			await expect
				.poll(() => page.evaluate(() => document.fullscreenElement !== null))
				.toBe(true);
			await expect(
				page.getByRole("button", { name: "Exit full screen" }),
			).toBeVisible();
			await page.getByRole("button", { name: "Exit full screen" }).click();
			await expect
				.poll(() => page.evaluate(() => document.fullscreenElement === null))
				.toBe(true);

			await page.addInitScript(() => {
				Object.defineProperty(document, "fullscreenEnabled", {
					value: false,
				});
			});
			await page.reload();
			await expect(
				page.getByRole("button", { name: "Mute preview" }),
			).toBeVisible();
			await expect(
				page.getByRole("button", { name: "Full screen preview" }),
			).toHaveCount(0);
		});
	});
});

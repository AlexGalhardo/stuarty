import { mkdirSync } from "node:fs";
import { join } from "node:path";
import AxeBuilder from "@axe-core/playwright";

import {
	audioClip,
	caller,
	expect,
	seedProject,
	solidClip,
	test,
} from "./helpers";

// Epic 81 (SDD-012 section 23.5): the three libraries share one frame and the same four states. The empty, the
// loading and the failed read are shown with the list of the service replaced by a fixed answer; the populated
// page is read from the real workspace. Each is scanned by axe in both themes and checked at three widths.
// STUARTY_SHOT_DIR, when set, is a folder that receives a screenshot of each page (for the hand-off).
test.describe.configure({ timeout: 180_000 });

const SECTIONS = [
	{ path: "/video-assets", title: "Video Assets", empty: "No videos yet" },
	{ path: "/visual-assets", title: "Visual Assets", empty: "No images yet" },
	{ path: "/sound-assets", title: "Sound Assets", empty: "No sounds yet" },
] as const;

const WIDTHS = [1440, 1280, 390] as const;
const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];
const LIST = "**/api/v1/assets?*";

async function shoot(
	page: import("@playwright/test").Page,
	name: string,
): Promise<void> {
	const dir = process.env.STUARTY_SHOT_DIR;
	if (dir === undefined || dir === "") return;
	mkdirSync(dir, { recursive: true });
	await page.screenshot({ path: join(dir, `${name}.png`), fullPage: true });
}

async function noViolation(page: import("@playwright/test").Page) {
	await page.evaluate(() =>
		Promise.race([
			Promise.all(
				document.getAnimations().map((a) => a.finished.catch(() => undefined)),
			),
			new Promise((resolve) => setTimeout(resolve, 3000)),
		]),
	);
	const { violations } = await new AxeBuilder({ page })
		.withTags(TAGS)
		.analyze();
	expect(violations.map((v) => `${v.id}: ${v.nodes[0]?.html}`)).toEqual([]);
}

async function noSidewaysScroll(page: import("@playwright/test").Page) {
	const overflow = await page.evaluate(
		() => document.documentElement.scrollWidth - window.innerWidth,
	);
	const wide = await page.evaluate(() =>
		[...document.querySelectorAll("body *")]
			.filter((el) => el.getBoundingClientRect().right > window.innerWidth + 1)
			.slice(0, 6)
			.map((el) => `${el.tagName}.${String(el.className).slice(0, 80)}`),
	);
	expect({ overflow, wide }).toEqual({ overflow: 0, wide: [] });
}

for (const theme of ["dark", "light"] as const) {
	test.describe(`asset library states ${theme}`, () => {
		// The dark theme is checked with the system in light mode: the choice is the stored one.
		test.use({ colorScheme: theme === "dark" ? "light" : "dark" });

		test.beforeEach(async ({ page }) => {
			await page.addInitScript((value) => {
				localStorage.setItem("theme", value);
			}, theme);
		});

		for (const section of SECTIONS) {
			test(`${section.title}: empty, loading and failed read`, async ({
				page,
			}) => {
				// Empty: the service answers with no file. The words speak of files and give three steps.
				await page.route(LIST, (route) =>
					route.fulfill({ json: { assets: [], total: 0 } }),
				);
				await page.goto(section.path);
				const empty = page.getByTestId("asset-empty");
				await expect(
					empty.getByRole("heading", { name: section.empty }),
				).toBeVisible();
				await expect(empty.getByRole("listitem")).toHaveCount(3);
				await expect(
					empty.getByRole("link", { name: "Open the projects" }),
				).toHaveAttribute("href", "/projects");
				await noViolation(page);
				await shoot(page, `${section.path.slice(1)}-empty-${theme}`);
				await page.unroute(LIST);

				// Loading: a status sentence and a skeleton that screen readers skip, until the answer comes.
				let release: () => void = () => undefined;
				const held = new Promise<void>((resolve) => {
					release = resolve;
				});
				await page.route(LIST, async (route) => {
					await held;
					await route.fulfill({ json: { assets: [], total: 0 } });
				});
				await page.goto(section.path);
				const loading = page.getByTestId("asset-loading");
				await expect(loading).toBeVisible();
				await expect(loading).toHaveAttribute("role", "status");
				await expect(page.getByTestId("asset-grid")).toHaveCount(0);
				await noViolation(page);
				await shoot(page, `${section.path.slice(1)}-loading-${theme}`);
				release();
				await expect(page.getByTestId("asset-empty")).toBeVisible();
				await page.unroute(LIST);

				// Failed read: the service's own words and a way to try again; the retry reads the list again.
				await page.route(LIST, (route) =>
					route.fulfill({
						status: 500,
						json: {
							code: "E_INTERNAL",
							message: "the media store did not answer",
							hint: "Try again in a few seconds.",
						},
					}),
				);
				await page.goto(section.path);
				const failed = page.getByTestId("asset-read-failed");
				await expect(failed).toBeVisible();
				await expect(page.getByRole("alert")).toContainText(
					"the media store did not answer",
				);
				await noViolation(page);
				await shoot(page, `${section.path.slice(1)}-error-${theme}`);
				await page.unroute(LIST);
				await failed.getByRole("button", { name: "Try again" }).click();
				await expect(failed).toHaveCount(0);
				await expect(page.getByRole("alert")).toHaveCount(0);
			});
		}

		test("the populated libraries pass axe and do not scroll sideways at three widths", async ({
			page,
			request,
		}) => {
			const call = await caller(request);
			const stamp = Date.now();
			const seeded = await seedProject(request, {
				tracks: 1,
				perTrack: 1,
				files: [
					solidClip(`0x${(stamp % 0xffffff).toString(16).padStart(6, "0")}`),
				],
			});
			await call("POST", `/projects/${seeded.id}/media`, {
				files: [{ path: audioClip(58) }],
				wait_ms: 20_000,
				origin: { kind: "generated", provider: "acme", model: "voice-1" },
				info: { tags: [`e81-${stamp}`], reusable: true },
			});
			for (const width of WIDTHS) {
				await page.setViewportSize({ width, height: 900 });
				for (const section of SECTIONS) {
					await page.goto(section.path);
					await expect(
						page.getByRole("heading", { name: section.title, level: 1 }),
					).toBeVisible();
					await expect(page.getByTestId("asset-loading")).toHaveCount(0);
					if (section.path !== "/visual-assets") {
						await expect(
							page.getByTestId("asset-grid").locator("li").first(),
						).toBeVisible();
						await page
							.getByTestId("asset-grid")
							.locator("li")
							.first()
							.getByRole("button")
							.first()
							.click();
						await expect(page.getByTestId("asset-detail")).toBeVisible();
						await expect(page.getByTestId("asset-rights")).toBeVisible();
					}
					await noViolation(page);
					await noSidewaysScroll(page);
					await shoot(page, `${section.path.slice(1)}-${width}-${theme}`);
				}
			}
		});
	});
}

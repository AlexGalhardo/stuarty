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
// page is read from the real workspace. Each is scanned by axe in both themes and checked at four widths, down to 320 px.
// STUARTY_SHOT_DIR, when set, is a folder that receives a screenshot of each page (for the hand-off).
test.describe.configure({ timeout: 180_000 });

const SECTIONS = [
	{ path: "/video-assets", title: "Videos", empty: "No videos yet" },
	{ path: "/visual-assets", title: "Images", empty: "No images yet" },
	{ path: "/sound-assets", title: "Sounds", empty: "No sounds yet" },
] as const;

const WIDTHS = [1440, 1280, 390, 320] as const;
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
	// A fade that is halfway reads as low contrast: wait for the finite animations to end.
	await page.evaluate(() =>
		Promise.all(
			document
				.getAnimations()
				.filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
				.map((a) => a.finished.catch(() => undefined)),
		),
	);
	// At 320 px the detail opens below the filters and the page scrolls to it, which leaves a filter under the
	// sticky header; axe then reads that filter as an obscured target. The person scrolls back to reach it, so the
	// sizes and the spacing are read from the top of the page.
	await page.evaluate(() => window.scrollTo(0, 0));
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

		// Epic 94 (SDD-012 section 23.5 item 5): the size of each file is the service's text, a file with no size says
		// why in words (never 0 B), each header says the count and the total, and two files of the same name show
		// their size and day. The list is replaced by a fixed answer so the three states are always there.
		for (const section of SECTIONS) {
			test(`${section.title}: sizes, states and the total of the library`, async ({
				page,
			}) => {
				const k = {
					"/visual-assets": "image",
					"/video-assets": "video",
					"/sound-assets": "audio",
				}[section.path];
				const entry = (n: string, extra: Record<string, unknown>) => ({
					hash: `sha256:${n.repeat(64)}`,
					name: "Same name.mp4",
					kind: k,
					origin: { kind: "imported" },
					tags: [],
					projects: 1,
					created_at: "2026-11-20T10:00:00Z",
					...extra,
				});
				await page.route(LIST, (route) =>
					route.fulfill({
						json: {
							assets: [
								entry("a", {
									bytes: 4_294_967_296,
									size_text: "4.3 GB",
									state: "ready",
									projects: 3,
								}),
								entry("b", {
									bytes: null,
									size_text: "",
									state: "importing",
									created_at: "2026-11-21T10:00:00Z",
								}),
								entry("c", { bytes: null, size_text: "", state: "missing" }),
							],
							total: 3,
							file_count: 3,
							total_bytes: 4_294_967_296,
							total_size_text: "4.3 GB",
							unsized: 2,
							next_cursor: null,
						},
					}),
				);
				await page.goto(section.path);
				const grid = page.getByTestId("asset-grid");
				await expect(grid.getByTestId("asset-size")).toHaveCount(3);
				await expect(grid).toContainText("4.3 GB");
				await expect(grid).not.toContainText("0 B");
				const states = await grid
					.getByTestId("asset-size")
					.evaluateAll((els) =>
						els.map((el) => [
							el.getAttribute("data-state"),
							el.textContent?.replace(/\s+/g, " ").trim(),
						]),
					);
				expect(states).toEqual([
					["ready", "4.3 GB"],
					["importing", "Size unknown (Still importing)"],
					["missing", "Size unknown (File missing)"],
				]);
				// Two files carry the same name: the day tells them apart, in words and not in colour.
				await expect(grid.getByTestId("asset-added")).toHaveCount(3);
				await expect(page.getByTestId("library-total")).toHaveText(
					"4.3 GB in total (2 of them have no known size)",
				);
				await expect(
					page.getByRole("heading", { name: "3 files" }),
				).toBeVisible();
				await noViolation(page);
				await noSidewaysScroll(page);
				await shoot(page, `${section.path.slice(1)}-sizes-${theme}`);

				// The detail of the first file, at two widths: the size, how many projects use the file and that it
				// is counted once.
				await page.route("**/api/v1/assets/sha256*", (route) =>
					route.request().method() === "GET"
						? route.fulfill({
								json: {
									...entry("a", {
										bytes: 4_294_967_296,
										size_text: "4.3 GB",
										state: "ready",
										projects: 3,
										uses: [],
									}),
									text: "",
								},
							})
						: route.continue(),
				);
				for (const width of [1440, 390, 320]) {
					await page.setViewportSize({ width, height: 900 });
					await page.goto(section.path);
					await page
						.getByTestId("asset-grid")
						.locator("li")
						.first()
						.getByRole("button")
						.first()
						.click();
					const detail = page.getByTestId("asset-detail");
					await expect(detail.getByTestId("asset-detail-size")).toContainText(
						"4.3 GB",
					);
					await expect(detail.getByTestId("asset-shared")).toContainText(
						"Used by 3 projects. The file is stored once and counted once in the library total.",
					);
					await noViolation(page);
					await noSidewaysScroll(page);
					await shoot(
						page,
						`${section.path.slice(1)}-detail-${width}-${theme}`,
					);
				}
			});
		}

		test("the populated libraries pass axe and do not scroll sideways at four widths", async ({
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
					// Every filter control (the tag and origin choices, the kind choice of the sounds and the reusable
					// toggle) is a target of at least 32 px (design system, section 7).
					const heights = await page
						.locator("search button")
						.evaluateAll((labels) =>
							labels.map((label) => label.getBoundingClientRect().height),
						);
					expect(heights).toHaveLength(
						section.path === "/sound-assets" ? 4 : 3,
					);
					expect(Math.min(...heights)).toBeGreaterThanOrEqual(32);
					await noViolation(page);
					await noSidewaysScroll(page);
					await shoot(page, `${section.path.slice(1)}-${width}-${theme}`);
				}
			}
		});
	});
}

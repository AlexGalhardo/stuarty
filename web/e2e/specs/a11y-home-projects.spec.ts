import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";

import { horizontalOverflow, undersizedTargets } from "../support/wcag";
import { caller, expect, projectBody, test } from "./helpers";

// Epic 79: the home, the projects, the new project dialog, the page of a project and the brand kit at 1440, 1280 and
// 390 px in both themes. axe finds nothing at WCAG 2.2 AA, nothing scrolls sideways and no target is too small.
const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];

const WIDTHS = [
	{ width: 1440, height: 900 },
	{ width: 1280, height: 800 },
	{ width: 390, height: 844 },
] as const;

async function settle(page: Page): Promise<void> {
	await expect
		.poll(() =>
			page.evaluate(
				() =>
					document
						.getAnimations()
						.filter(
							(a) =>
								a.playState === "running" &&
								a.effect?.getComputedTiming().iterations !== Infinity,
						).length,
			),
		)
		.toBe(0);
}

async function measure(page: Page, where: string): Promise<string[]> {
	await settle(page);
	const found: string[] = [];
	const { violations } = await new AxeBuilder({ page })
		.withTags(TAGS)
		.analyze();
	for (const v of violations) found.push(`${where}: ${v.id}`);
	const overflow = await horizontalOverflow(page);
	if (overflow) found.push(`${where}: ${overflow}`);
	for (const small of await undersizedTargets(page)) {
		found.push(`${where}: ${small.target} ${small.size}`);
	}
	return found;
}

test.describe("a11y-home-projects", () => {
	test.describe.configure({ timeout: 180_000 });

	for (const theme of ["light", "dark"] as const) {
		for (const size of WIDTHS) {
			test(`the screens of epic 79 are clean at ${size.width} px in ${theme}`, async ({
				page,
				request,
			}) => {
				const call = await caller(request);
				const stamp = Date.now();
				const text = (await call("POST", "/projects", {
					...projectBody(`A11y text ${stamp}`),
					kind: "text",
				})) as { project_id: string };
				await call("POST", `/projects/${text.project_id}/items`, {
					kind: "text",
					title: "The post",
					text: "A first paragraph of the post, long enough to show as an excerpt on its card and to be counted.",
					text_kind: "post",
				});
				const video = (await call("POST", "/projects", {
					...projectBody(`A11y video ${stamp}`),
					kind: "video",
				})) as { project_id: string };
				await call("POST", `/projects/${video.project_id}/items`, {
					kind: "timeline",
					title: "Main cut",
				});
				await call("POST", "/projects", projectBody(`A11y later ${stamp}`));

				await page.addInitScript(
					(th) => localStorage.setItem("theme", th),
					theme,
				);
				await page.setViewportSize(size);
				const found: string[] = [];

				for (const { path, heading } of [
					{ path: "/", heading: /^Good (morning|afternoon|evening)/ },
					{ path: "/projects", heading: "Projects" },
					{ path: `/projects/${text.project_id}`, heading: /A11y text/ },
					{ path: `/projects/${video.project_id}`, heading: /A11y video/ },
					{ path: "/settings/brand-kit", heading: "Brand kit" },
				] as const) {
					await page.goto(path);
					await expect(
						page.getByRole("heading", { level: 1, name: heading }),
					).toBeVisible();
					if (path === "/") {
						await expect(page.getByTestId("home-progress")).toBeVisible();
					}
					found.push(...(await measure(page, path)));
				}

				// The dialog, on both of its steps.
				await page.goto("/projects");
				await page.getByRole("button", { name: "New project" }).click();
				await expect(page.getByRole("dialog")).toBeVisible();
				found.push(...(await measure(page, "dialog: kinds")));
				await page.locator('[data-choice="video"]').click();
				found.push(...(await measure(page, "dialog: video")));
				await page
					.getByRole("button", { name: "Continue to the briefing" })
					.click();
				await expect(page.getByLabel("Name", { exact: true })).toBeVisible();
				found.push(...(await measure(page, "dialog: briefing")));

				expect(found).toEqual([]);
			});
		}
	}

	test("the empty home and the empty list are clean at 390 px in dark", async ({
		page,
	}) => {
		await page.addInitScript(() => localStorage.setItem("theme", "dark"));
		await page.setViewportSize({ width: 390, height: 844 });
		// The workspace of the specs is shared and never empty: the list the screens read is answered empty.
		await page.route("**/api/v1/projects", async (route) => {
			if (route.request().method() !== "GET") return route.fallback();
			const response = await route.fetch();
			const body = (await response.json()) as object;
			await route.fulfill({ response, json: { ...body, projects: [] } });
		});
		await page.goto("/");
		await expect(
			page.getByRole("region", { name: "Make your first project" }),
		).toBeVisible();
		expect(await measure(page, "/ (empty)")).toEqual([]);
	});
});

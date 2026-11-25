import type { Page } from "@playwright/test";

import { horizontalOverflow, undersizedTargets } from "../support/wcag";
import { expect, seedProject, test } from "./helpers";

// settle waits for the fonts and the requests of the page, so the next
// navigation does not abort them: an aborted font or fetch is reported by
// Firefox and WebKit as a console error that no person would see.
async function settle(page: Page): Promise<void> {
	await page.evaluate(() => document.fonts.ready.then(() => true));
	await page.waitForLoadState("networkidle");
}

// RF-310 to RF-312 (epic 07): the application opens on the dashboard, the
// sidebar leads to every section, and the editor has a way in and a way back.
const SECTIONS = [
	"projects",
	"edit-video",
	"edit-image",
	"write",
	"visual-assets",
	"video-assets",
	"sound-assets",
	"publish",
	"skills",
	"prompts",
	"memory",
	"connections",
	"webhooks",
] as const;

const LABELS = {
	en: [
		"Projects",
		"Video editor",
		"Image editor",
		"Writing",
		"Images",
		"Videos",
		"Sounds",
		"Publishing",
		"Skills",
		"Prompts",
		"Memory",
		"Connections",
		"Webhooks",
	],
	"pt-BR": [
		"Projetos",
		"Editor de vídeo",
		"Editor de imagem",
		"Escrita",
		"Visuais",
		"Vídeos",
		"Sons",
		"Publicação",
		"Skills",
		"Prompts",
		"Memória",
		"Conexões",
		"Webhooks",
	],
} as const;

test.describe("dashboard-navigation", () => {
	test("the application opens on the dashboard with the thirteen sections, the profile and the theme toggle", async ({
		page,
	}) => {
		await page.goto("/");
		await expect(page).toHaveURL(/\/$/);
		await expect(
			page.getByRole("heading", {
				name: /^Good (morning|afternoon|evening)/,
				level: 1,
			}),
		).toBeVisible();

		const nav = page.getByRole("navigation", { name: "Main navigation" });
		// Home, then the five groups of the shell (epic 75); Approvals carries the count, in a word, when some wait
		await expect(nav.getByRole("link")).toHaveText([
			"Home",
			"Projects",
			"Writing",
			"Video editor",
			"Image editor",
			"Videos",
			"Images",
			"Sounds",
			"Brand kit",
			"Skills",
			"Prompts",
			"Memory",
			/^Approvals/,
			"Publishing",
			"Connections",
			"Webhooks",
		]);

		await expect(page.getByTestId("profile-name")).toHaveText("stuart");
		await expect(page.getByRole("link", { name: "Settings" })).toBeVisible();
		await expect(
			page.getByRole("button", { name: /Switch to (light|dark) theme/ }),
		).toBeVisible();
		// The Chat button of the top bar opens the agent chat (epic 14): closed, no panel.
		await expect(
			page.getByTestId("topbar").getByTestId("chat-toggle"),
		).toBeVisible();
		await expect(page.getByTestId("chat-panel")).toHaveCount(0);
	});

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`every section opens in ${theme} in ${lang} with no console error`, async ({
				page,
				browserName,
			}) => {
				await settle(page);
				const problems: string[] = [];
				page.on("console", (message) => {
					if (message.type() !== "error") return;
					// WebKit reports a violation of style-src-attr every time a native
					// <select> is created (its own user-agent shadow tree, not our markup;
					// Chromium and Firefox stay silent), and its console line does not name
					// the directive, so that line is dropped here. The violations themselves
					// are checked by the page fixture of helpers.ts, across navigations, with
					// only style-src-attr let through in WebKit (cspViolations): a <style> or
					// a script without the nonce still fails (VV-263).
					if (
						browserName === "webkit" &&
						message.text().startsWith("Refused to apply a stylesheet")
					) {
						return;
					}
					problems.push(`${message.text()} ${message.location().url}`);
				});
				page.on("pageerror", (error) => problems.push(error.message));
				await page.addInitScript(
					([l, th]) => {
						localStorage.setItem("vv.lang", l);
						localStorage.setItem("theme", th);
					},
					[lang, theme],
				);

				await page.goto("/");
				await settle(page);
				const dark = theme === "dark";
				await expect(page.locator("html")).toHaveClass(
					dark ? /dark/ : /^((?!dark).)*$/,
				);

				for (const [i, key] of SECTIONS.entries()) {
					await page.getByTestId(`nav-${key}`).click();
					await expect(page).toHaveURL(new RegExp(`/${key}$`));
					await settle(page);
					await expect(page.getByTestId(`nav-${key}`)).toHaveAttribute(
						"aria-current",
						"page",
					);
					await expect(
						page
							.getByRole("navigation", {
								name: /Main navigation|Navegação principal/,
							})
							.locator('[aria-current="page"]'),
					).toHaveCount(1);
					await expect(
						page.getByRole("heading", { level: 1 }).first(),
					).toBeVisible();
					if (key !== "projects") {
						await expect(
							page.getByRole("heading", { name: LABELS[lang][i], level: 1 }),
						).toBeVisible();
					}
				}
				expect(problems).toEqual([]);
			});
		}
	}

	// Every section but Edit Video has its own page now; Edit Video opens from a project, and its section
	// says so and leads there.
	test("a section with no page of its own says so and leads to the projects", async ({
		page,
	}) => {
		await page.goto("/edit-video");
		await expect(page.getByTestId("empty-edit-video")).toContainText(
			"The video editor opens from a project",
		);
		await page.getByRole("link", { name: "Go to Projects" }).click();
		await expect(page).toHaveURL(/\/projects$/);
	});

	// WebKit leaves links out of the Tab order unless a browser option is set
	// (the Option+Tab rule of macOS), so the links cannot be reached by Tab
	// there; that is an engine setting, not a defect of the page.
	test("the sidebar is reached and operated by keyboard alone @chromium-only", async ({
		page,
	}) => {
		await page.goto("/");
		await page.getByRole("link", { name: /Stuarty, dashboard/ }).focus();

		// Home, the thirteen sections and the brand kit and Approvals links, in the order of the groups
		const order = [
			"home",
			"projects",
			"write",
			"edit-video",
			"edit-image",
			"video-assets",
			"visual-assets",
			"sound-assets",
			"brand-kit",
			"skills",
			"prompts",
			"memory",
			"approvals",
			"publish",
			"connections",
			"webhooks",
		];
		const seen: string[] = [];
		for (let i = 0; i < order.length; i++) {
			await page.keyboard.press("Tab");
			seen.push(
				await page.evaluate(
					() => document.activeElement?.getAttribute("data-testid") ?? "",
				),
			);
		}
		expect(seen).toEqual(order.map((key) => `nav-${key}`));

		// Enter on the focused item (Webhooks, the last) opens it.
		await page.keyboard.press("Enter");
		await expect(page).toHaveURL(/\/webhooks$/);
	});

	test("the theme toggle of the sidebar flips and remembers the theme", async ({
		page,
	}) => {
		await page.addInitScript(() => {
			// Only the first load: the reload below must keep what the toggle saved.
			if (localStorage.getItem("theme") === null)
				localStorage.setItem("theme", "light");
		});
		await page.goto("/");
		const html = page.locator("html");
		await expect(html).not.toHaveClass(/dark/);
		await page.getByRole("button", { name: "Switch to dark theme" }).click();
		await expect(html).toHaveClass(/dark/);
		expect(await page.evaluate(() => localStorage.getItem("theme"))).toBe(
			"dark",
		);
		await page.reload();
		await expect(html).toHaveClass(/dark/);
	});

	test("the sidebar collapses to icons, keeps the names and remembers the choice", async ({
		page,
	}) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.goto("/");
		const sidebar = page.getByTestId("sidebar");
		await expect(sidebar).toHaveAttribute("data-state", "expanded");
		await page.getByTestId("sidebar-toggle").click();
		await expect(sidebar).toHaveAttribute("data-state", "collapsed");
		await expect(page.getByTestId("nav-publish")).toBeVisible();
		await expect(page.getByTestId("nav-publish")).toHaveAccessibleName(
			"Publishing",
		);
		await page.reload();
		await expect(sidebar).toHaveAttribute("data-state", "collapsed");
		await page.getByTestId("sidebar-toggle").click();
		await expect(sidebar).toHaveAttribute("data-state", "expanded");
	});

	test("with no choice the sidebar is a rail up to 1280 px and full above it", async ({
		page,
	}) => {
		await page.setViewportSize({ width: 1280, height: 800 });
		await page.goto("/");
		await expect(page.getByTestId("sidebar")).toHaveAttribute(
			"data-form",
			"rail",
		);
		await page.setViewportSize({ width: 1440, height: 900 });
		await expect(page.getByTestId("sidebar")).toHaveAttribute(
			"data-form",
			"full",
		);
	});

	test("on a phone the sidebar is a drawer that the menu button opens, whatever was chosen", async ({
		page,
	}) => {
		await page.addInitScript(() => localStorage.setItem("sidebar", "expanded"));
		await page.setViewportSize({ width: 360, height: 700 });
		await page.goto("/");
		await expect(page.getByTestId("sidebar")).toHaveCount(0);
		const menu = page.getByTestId("menu-toggle");
		await expect(menu).toBeVisible();
		await menu.click();
		const drawer = page.getByTestId("sidebar-drawer");
		await expect(drawer).toBeVisible();
		await expect(drawer.getByTestId("nav-connections")).toBeVisible();
		// the drawer has no collapse button: it would change nothing there
		await expect(drawer.getByTestId("sidebar-toggle")).toHaveCount(0);
		await page.keyboard.press("Escape");
		await expect(drawer).toHaveCount(0);
		await expect(menu).toBeFocused();
		// following a link closes it
		await menu.click();
		await page.getByTestId("nav-publish").click();
		await expect(page).toHaveURL(/\/publish$/);
		await expect(page.getByTestId("sidebar-drawer")).toHaveCount(0);
	});

	// WCAG 1.4.10 and 2.5.8: the home and a section page reflow at 320 CSS px
	// with targets of 24 px, and the drawer is read in the order it is drawn.
	test("the dashboard reflows at 320 px with the navigation before the profile", async ({
		page,
	}) => {
		await page.setViewportSize({ width: 320, height: 256 });
		const found: string[] = [];
		for (const path of ["/", "/publish"]) {
			await page.goto(path);
			await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
			const overflow = await horizontalOverflow(page);
			if (overflow) found.push(`${path}: ${overflow}`);
			for (const small of await undersizedTargets(page))
				found.push(`${path}: ${small.target} ${small.size}`);
		}
		expect(found, "pages that scroll or have small targets").toEqual([]);

		const top = async (testId: string): Promise<number> =>
			(await page.getByTestId(testId).boundingBox())?.y ?? Number.NaN;
		await page.getByTestId("menu-toggle").click();
		expect(await top("nav-projects")).toBeLessThan(await top("profile-name"));
	});

	test("the way into the editor and the way back to the dashboard", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });

		// In: Edit Video leads to the projects, whose card opens the editor.
		await page.goto("/edit-video");
		await expect(page.getByTestId("empty-edit-video")).toContainText(
			"The video editor opens from a project",
		);
		await page.getByRole("link", { name: "Go to Projects" }).click();
		await expect(page).toHaveURL(/\/projects$/);
		await page
			.locator(`[data-project-card='${seeded.id}']`)
			.getByRole("link", { name: /^Open/ })
			.click();
		await expect(page).toHaveURL(new RegExp(`/editor/${seeded.id}$`));

		// Back: the top bar has a link to the dashboard and one to the list.
		await page.getByRole("link", { name: "Dashboard" }).click();
		await expect(page).toHaveURL(/\/$/);
		await expect(
			page.getByRole("heading", {
				name: /^Good (morning|afternoon|evening)/,
				level: 1,
			}),
		).toBeVisible();

		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("link", { name: "Back to projects" }).click();
		await expect(page).toHaveURL(/\/projects$/);
	});
});

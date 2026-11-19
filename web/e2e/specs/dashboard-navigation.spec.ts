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
	"publish",
	"skills",
	"prompts",
	"memory",
	"connections",
] as const;

const LABELS = {
	en: [
		"Projects",
		"Edit Video",
		"Edit Image",
		"Write",
		"Visual Assets",
		"Video Assets",
		"Publish",
		"Skills",
		"Prompts",
		"Memory",
		"Connections",
	],
	"pt-BR": [
		"Projetos",
		"Editar vídeo",
		"Editar imagem",
		"Escrever",
		"Recursos visuais",
		"Recursos de vídeo",
		"Publicar",
		"Habilidades",
		"Instruções",
		"Memória",
		"Conexões",
	],
} as const;

test.describe("dashboard-navigation", () => {
	test("the application opens on the dashboard with the eleven sections, the profile and the theme toggle", async ({
		page,
	}) => {
		await page.goto("/");
		await expect(page).toHaveURL(/\/$/);
		await expect(
			page.getByRole("heading", { name: "Home", level: 1 }),
		).toBeVisible();

		const nav = page.getByRole("navigation", { name: "Main navigation" });
		await expect(nav.getByRole("link")).toHaveText(LABELS.en);

		await expect(page.getByTestId("profile-name")).toHaveText("Local");
		await expect(page.getByRole("link", { name: "Settings" })).toBeVisible();
		await expect(
			page.getByRole("button", { name: /Switch to (light|dark) theme/ }),
		).toBeVisible();
		await expect(page.getByTestId("right-dock")).toBeEmpty();
	});

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`every section opens in ${theme} in ${lang} with no console error`, async ({
				page,
			}) => {
				await settle(page);
				const problems: string[] = [];
				page.on("console", (message) => {
					if (message.type() !== "error") return;
					// The thumbnails of the projects page ask the frame endpoint for every
					// project, and the workspace of the suite holds hundreds, so the
					// heavy limit answers 429. That is the content of the projects page
					// (epic 06), not of the dashboard: only that URL is let through.
					const url = message.location().url;
					if (/\/api\/v1\/projects\/prj_\w+\/frame\?/.test(url)) return;
					problems.push(`${message.text()} ${url}`);
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
					await expect(page.locator('[aria-current="page"]')).toHaveCount(1);
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

	test("a section that is not built says so and leads to the projects", async ({
		page,
	}) => {
		await page.goto("/write");
		await expect(page.getByTestId("empty-write")).toContainText(
			"This section is not available yet",
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

		const seen: string[] = [];
		for (let i = 0; i < SECTIONS.length; i++) {
			await page.keyboard.press("Tab");
			seen.push(
				await page.evaluate(
					() => document.activeElement?.getAttribute("data-testid") ?? "",
				),
			);
		}
		expect(seen).toEqual(SECTIONS.map((key) => `nav-${key}`));

		// Enter on the focused item (Connections, the last) opens it.
		await page.keyboard.press("Enter");
		await expect(page).toHaveURL(/\/connections$/);
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
		await page.goto("/");
		const sidebar = page.getByTestId("sidebar");
		await expect(sidebar).toHaveAttribute("data-state", "expanded");
		await page.getByTestId("sidebar-toggle").click();
		await expect(sidebar).toHaveAttribute("data-state", "collapsed");
		await expect(page.getByTestId("nav-publish")).toBeVisible();
		await expect(page.getByTestId("nav-publish")).toHaveAccessibleName(
			"Publish",
		);
		await page.reload();
		await expect(sidebar).toHaveAttribute("data-state", "collapsed");
		await page.getByTestId("sidebar-toggle").click();
		await expect(sidebar).toHaveAttribute("data-state", "expanded");
	});

	test("the sidebar is collapsed in a narrow window whatever was chosen", async ({
		page,
	}) => {
		await page.addInitScript(() => localStorage.setItem("sidebar", "expanded"));
		await page.setViewportSize({ width: 360, height: 700 });
		await page.goto("/");
		await expect(page.getByTestId("sidebar")).toHaveAttribute(
			"data-state",
			"collapsed",
		);
		await expect(page.getByTestId("nav-connections")).toBeVisible();
		// The toggle could not expand it here, so it is not offered.
		await expect(page.getByTestId("sidebar-toggle")).toBeHidden();
	});

	// WCAG 1.4.10 and 2.5.8: the home and a section page reflow at 320 CSS px
	// with targets of 24 px, and the sidebar strip is read and tabbed in the
	// order it is drawn.
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
			page.getByRole("heading", { name: "Home", level: 1 }),
		).toBeVisible();

		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("link", { name: "Back to projects" }).click();
		await expect(page).toHaveURL(/\/projects$/);
	});
});

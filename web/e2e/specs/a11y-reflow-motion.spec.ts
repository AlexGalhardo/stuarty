import {
	clippedText,
	horizontalOverflow,
	TEXT_SPACING_CSS,
	undersizedTargets,
} from "../support/wcag";
import { expect, seedProject, test } from "./helpers";
import { newImage } from "./image-support";

// Epic 41 task 7 (SDD-012 section 18.10, items R, S, T and M): reflow at 320 CSS px, text spacing and target
// size on every page that is not an editor (wcag22 measures three of them and the editor), and reduced motion.
// The text-spacing style sheet is a user style sheet: the page CSP would refuse it as inline style.
test.use({ bypassCSP: true });
test.describe.configure({ timeout: 180_000 });

const ROUTES = [
	"/",
	"/projects",
	"/projects/:project",
	"/edit-video",
	"/edit-image",
	"/write",
	"/visual-assets",
	"/video-assets",
	"/publish",
	"/approvals",
	"/skills",
	"/prompts",
	"/memory",
	"/connections",
	"/settings",
	"/settings/brand-kit",
] as const;

for (const theme of ["dark", "light"] as const) {
	test.describe(`a11y-reflow-motion ${theme}`, () => {
		test.beforeEach(async ({ page }) => {
			await page.addInitScript((value) => {
				localStorage.setItem("theme", value);
			}, theme);
		});

		async function visit(
			page: import("@playwright/test").Page,
			request: Parameters<typeof seedProject>[0],
			each: (path: string) => Promise<void>,
		): Promise<void> {
			const project = await seedProject(request, {
				name: `A long project name that must wrap and never push the page wider ${Date.now()}`,
				tracks: 0,
			});
			for (const route of ROUTES) {
				const path = route.replace(":project", project.id);
				await page.goto(path);
				await expect(
					page.getByRole("heading", { level: 1 }).first(),
				).toBeVisible();
				await each(path);
			}
		}

		test("every page reflows at 320 CSS px without horizontal scrolling", async ({
			page,
			request,
		}) => {
			await page.setViewportSize({ width: 320, height: 256 });
			const found: string[] = [];
			await visit(page, request, async (path) => {
				const overflow = await horizontalOverflow(page);
				if (overflow) found.push(`${path}: ${overflow}`);
			});
			expect(found, "pages that scroll in two dimensions").toEqual([]);
		});

		test("every page keeps its text under the text spacing of the criterion", async ({
			page,
			request,
		}) => {
			const found: string[] = [];
			await visit(page, request, async (path) => {
				await page.addStyleTag({ content: TEXT_SPACING_CSS });
				for (const clipped of await clippedText(page))
					found.push(`${path}: ${clipped}`);
			});
			expect(found, "text clipped by the spacing").toEqual([]);
		});

		test("every pointer target of every page is 24 by 24 CSS px or spaced", async ({
			page,
			request,
		}) => {
			const found: string[] = [];
			await visit(page, request, async (path) => {
				for (const small of await undersizedTargets(page))
					found.push(`${path}: ${small.target} ${small.size}`);
			});
			expect(found, "undersized targets").toEqual([]);
		});

		test("the image page reflows at 320 CSS px", async ({ page, request }) => {
			const image = await newImage(request, { name: "Reflow spec" });
			await page.setViewportSize({ width: 320, height: 256 });
			await page.goto(`/edit-image/${image.id}`);
			await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
			// The canvas and the layer tree are two-dimensional (WCAG 1.4.10 exception); the page around
			// them wraps.
			const overflow = await horizontalOverflow(page);
			expect(overflow, "image page overflow").toBeNull();
		});
	});
}

// The longest animation of the design system (section 2.13) is 200 ms; with the preference nothing may run
// longer than the one millisecond that keeps the end events of the popups firing.
async function openDelete(
	page: import("@playwright/test").Page,
	label: string,
): Promise<void> {
	await page.goto("/projects");
	await page
		.getByRole("button", { name: new RegExp(`Actions for project.*${label}`) })
		.click();
	await page.getByRole("menuitem", { name: "Delete" }).click();
	await expect(page.getByRole("alertdialog")).toBeVisible();
}

const longest = (page: import("@playwright/test").Page): Promise<number> =>
	page.evaluate(() =>
		Math.max(
			0,
			...document
				.getAnimations()
				.map((a) => Number(a.effect?.getComputedTiming().duration ?? 0)),
		),
	);

// The duration the style sheet gives the opening of the dialog, in ms: it does not depend on whether the
// animation is still running when the page is sampled (WebKit has often finished it).
const dialogAnimationMs = (
	page: import("@playwright/test").Page,
): Promise<number> =>
	page.evaluate(() => {
		const dialog = document.querySelector("[role=alertdialog]") as Element;
		return Math.max(
			...getComputedStyle(dialog)
				.animationDuration.split(",")
				.map((value) =>
					value.trim().endsWith("ms")
						? Number.parseFloat(value)
						: Number.parseFloat(value) * 1000,
				),
		);
	});

test.describe("a11y-reflow-motion no preference", () => {
	test.use({ reducedMotion: "no-preference" });
	// The control: without it the next tests could pass because nothing ever animates.
	test("a dialog animates for longer than a millisecond", async ({
		page,
		request,
	}) => {
		const label = `Motion ${Date.now()}`;
		await seedProject(request, { name: label, tracks: 0 });
		await openDelete(page, label);
		expect(await dialogAnimationMs(page)).toBeGreaterThan(1);
	});
});

test.describe("a11y-reflow-motion reduced motion", () => {
	test.use({ reducedMotion: "reduce" });

	test("a dialog and the buttons run no animation or transition", async ({
		page,
		request,
	}) => {
		const label = `Motion ${Date.now()}`;
		await seedProject(request, { name: label, tracks: 0 });
		await openDelete(page, label);
		expect(await dialogAnimationMs(page)).toBeLessThanOrEqual(1);
		expect(await longest(page)).toBeLessThanOrEqual(1);
		expect(
			await page.evaluate(
				() =>
					getComputedStyle(document.querySelector("button") as Element)
						.transitionDuration,
			),
		).toBe("0s");
	});

	test("the chat panel and the sidebar run no animation", async ({ page }) => {
		await page.goto("/");
		await page.getByTestId("chat-toggle").click();
		await expect(page.getByTestId("chat-panel")).toBeVisible();
		await page.getByRole("button", { name: /^Collapse/ }).click();
		expect(await longest(page)).toBeLessThanOrEqual(1);
	});
});

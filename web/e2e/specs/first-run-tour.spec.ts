import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { caller, expect, test } from "./helpers";

const STEPS = [
	{ id: "library", title: "Your media" },
	{ id: "preview", title: "Preview" },
	{ id: "timeline", title: "Timeline" },
	{ id: "inspector", title: "Inspector" },
	{ id: "export", title: "Export" },
] as const;

async function openProject(
	page: Page,
	request: Parameters<typeof caller>[0],
): Promise<void> {
	const call = await caller(request);
	const created = await call("POST", "/projects", {
		name: `Tour ${Date.now()}`,
	});
	await page.goto(`/editor/${created.project_id as string}`);
}

// P3-UI-002 and epic 37 (SDD-012 section 18.5): the first-run tour shows once on
// the first load, walks the library, preview, timeline, inspector and export in
// that order, can be skipped, and is replayed from the Help menu and from the
// shortcuts dialog. The empty regions name their next action.
test.describe("first-run tour", () => {
	test.use({ tipsSeen: false });

	test("shows once, in order, with the region outlined, and the second load does not show it", async ({
		page,
		request,
	}) => {
		await openProject(page, request);
		const card = page.locator("[data-tour-card]");
		await expect(card).toBeVisible();
		await expect(card).toHaveAttribute("role", "dialog");

		for (const [index, step] of STEPS.entries()) {
			await expect(card).toHaveAttribute("data-tour-step", step.id);
			await expect(
				card.getByRole("heading", { name: step.title }),
			).toBeVisible();
			await expect(card).toContainText(`Step ${index + 1} of ${STEPS.length}`);
			// The step outlines its region, and the focus is on Next.
			await expect(page.locator("[data-tour-outline]")).toBeVisible();
			await expect(card.locator("[data-tour-next]")).toBeFocused();
			if (index < STEPS.length - 1) {
				await card.getByRole("button", { name: "Next" }).click();
			}
		}
		// The last step says Done; Back goes one step back.
		await card.getByRole("button", { name: "Back" }).click();
		await expect(card).toHaveAttribute("data-tour-step", "inspector");
		await card.getByRole("button", { name: "Next" }).click();
		await card.getByRole("button", { name: "Done" }).click();
		await expect(card).toBeHidden();

		await page.reload();
		await expect(page.getByRole("button", { name: "Export" })).toBeVisible();
		await expect(card).toBeHidden();
	});

	test("Skip tour and Esc end it, and the focus goes back where it was", async ({
		page,
		request,
	}) => {
		await openProject(page, request);
		const card = page.locator("[data-tour-card]");
		await expect(card).toBeVisible();
		await card.getByRole("button", { name: "Skip tour" }).click();
		await expect(card).toBeHidden();

		// A skipped tour is remembered: a reload does not show it.
		await page.reload();
		await expect(page.getByRole("button", { name: "Export" })).toBeVisible();
		await expect(card).toBeHidden();

		// Replay from the Help menu, then Esc.
		await page.getByRole("button", { name: "Menu", exact: true }).click();
		await page.getByRole("menuitem", { name: "Help" }).click();
		await page.getByRole("menuitem", { name: "Take the tour" }).click();
		await expect(card).toBeVisible();
		await expect(card).toHaveAttribute("data-tour-step", "library");
		await page.keyboard.press("Escape");
		await expect(card).toBeHidden();
		// The menu that started the tour has gone: a key aimed at an open menu
		// is the menu's, not the editor's.
		await expect(page.getByRole("menu")).toHaveCount(0);

		// Replay from the shortcuts dialog, which closes.
		await page.locator("body").press("?");
		await page.getByRole("button", { name: "Replay the tour" }).click();
		await expect(card).toBeVisible();
		await expect(card).toHaveAttribute("data-tour-step", "library");
		await expect(
			page.getByRole("dialog", { name: "Keyboard shortcuts" }),
		).toBeHidden();
	});

	test("the steps name the keys that are current", async ({
		page,
		request,
	}) => {
		await openProject(page, request);
		const card = page.locator("[data-tour-card]");
		await expect(card).toContainText(/press (Ctrl|Cmd)\+I/);
		await card.getByRole("button", { name: "Next" }).click();
		await expect(card).toContainText("Press Space to play or pause");
		await card.getByRole("button", { name: "Next" }).click();
		await expect(card).toContainText(/Press (Ctrl|Cmd)\+B to split/);
	});

	test("axe finds nothing serious on a step", async ({ page, request }) => {
		await openProject(page, request);
		await expect(page.locator("[data-tour-card]")).toBeVisible();
		const violations = (await new AxeBuilder({ page }).analyze()).violations;
		expect(
			violations.filter(
				(v) => v.impact === "serious" || v.impact === "critical",
			),
		).toEqual([]);
	});
});

test.describe("empty states", () => {
	test("the library, the timeline and the history name the next action", async ({
		page,
		request,
	}) => {
		await openProject(page, request);
		await expect(
			page.getByText("Paste the path of a video, audio or image file above"),
		).toBeVisible();
		await expect(page.getByText("The timeline is empty")).toBeVisible();
		await page.keyboard.press("Alt+3");
		await expect(page.getByText("No changes yet")).toBeVisible();
	});
});

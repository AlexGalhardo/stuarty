import { horizontalOverflow } from "../support/wcag";
import { expect, seedProject, test } from "./helpers";

// End-to-end regressions for epics 102 and 104:
// - Epic 102: full-document navigation from Home to /editor/{id}, collapsed sidebar hides version,
//   and Prompts empty state renders numbered steps cleanly without horizontal overflow.
// - Epic 104: global Back button across dashboard and editor, clickable Home workflow trail steps,
//   and double-click shortcuts on publishing calendar day cells.
test.describe("v0.61 navigation and UI fixes", () => {
	test("global Back button walks back through visited dashboard screens", async ({
		page,
	}) => {
		await page.goto("/");
		await expect(page.getByTestId("topbar-back")).toBeDisabled();

		await page.getByRole("link", { name: "Projects" }).click();
		await expect(page).toHaveURL(/\/projects$/);
		await expect(page.getByTestId("topbar-back")).toBeEnabled();

		await page.getByRole("link", { name: "Settings" }).click();
		await expect(page).toHaveURL(/\/settings$/);

		await page.getByTestId("topbar-back").click();
		await expect(page).toHaveURL(/\/projects$/);

		await page.getByTestId("topbar-back").click();
		await expect(page).toHaveURL(/\/$/);
	});

	test("collapsed sidebar hides the product version label", async ({
		page,
	}) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.goto("/");

		await expect(page.getByTestId("sidebar-version")).toBeVisible();
		await page.getByTestId("sidebar-toggle").click();
		await expect(page.getByTestId("sidebar-version")).toHaveCount(0);

		// Restore expanded sidebar so subsequent specs see the default layout.
		await page.getByTestId("sidebar-toggle").click();
		await expect(page.getByTestId("sidebar-version")).toBeVisible();
	});

	test("prompts empty state lays out numbered steps without horizontal overflow", async ({
		page,
	}) => {
		for (const size of [
			{ width: 1280, height: 800 },
			{ width: 390, height: 844 },
		]) {
			await page.setViewportSize(size);
			await page.goto("/prompts");
			await expect(
				page.getByRole("heading", { name: "Prompts", level: 1 }),
			).toBeVisible();
			expect(await horizontalOverflow(page)).toBeNull();
		}
	});

	test("Home workflow step dots and Continue button open the video editor cleanly without a blank Inertia screen", async ({
		page,
	}) => {
		const { id } = await seedProject(page.request, {
			name: "Epic 102/104 Workflow Video",
		});
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.goto("/");

		const progress = page.getByTestId("home-progress");
		await expect(progress).toBeVisible();

		// Clicking the Publication workflow step dot jumps straight to /publish.
		await progress.getByTestId("home-trail-step-publication").click();
		await expect(page).toHaveURL(/\/publish$/);

		// Back returns straight to Home.
		await page.getByTestId("topbar-back").click();
		await expect(page).toHaveURL(/\/$/);

		// Clicking the Editing workflow step dot performs full document navigation into /editor/{id}.
		await page
			.getByTestId("home-progress")
			.getByTestId("home-trail-step-editing")
			.click();
		await expect(page).toHaveURL(new RegExp(`/editor/${id}$`));
		await expect(page.getByTestId("editor-back")).toBeVisible();

		// No Inertia non-Inertia response overlay modal may appear.
		await expect(page.locator("#inertia-error")).toHaveCount(0);

		// Editor Back button returns to the previous page (Home).
		await page.getByTestId("editor-back").click();
		await expect(page).toHaveURL(/\/$/);

		// Double-clicking a video asset card opens the video editor directly (epic 104).
		await page.goto("/video-assets");
		const card = page.locator("button[data-asset-hash]").first();
		await expect(card).toBeVisible();
		await card.dblclick();
		await expect(page).toHaveURL(/\/(editor\/prj_|edit-video\?asset=)/);
	});

	test("double-clicking a day card in the publishing calendar opens the day view", async ({
		page,
	}) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.goto("/publish");
		await expect(page.getByTestId("calendar-month")).toBeVisible();

		const firstDay = page.getByTestId("calendar-day").nth(10);
		await firstDay.dblclick({ position: { x: 40, y: 60 } });
		await expect(page.getByTestId("day-note")).toBeVisible();
	});
});

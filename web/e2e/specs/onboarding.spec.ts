import AxeBuilder from "@axe-core/playwright";
import { caller, expect, test } from "./helpers";

// P3-UI-002 (SDD-012 section 18.5): the first-run tips show on the first load
// and not on the second, "Show tips again" brings them back, and the empty
// regions name their next action.
test.describe("onboarding", () => {
	test.use({ tipsSeen: false });

	test("tips show once, come back from the shortcuts dialog, empty states name the next action", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			name: `Onboarding ${Date.now()}`,
		});
		await page.goto(`/editor/${created.project_id as string}`);
		const tips = page.getByRole("region", { name: "First steps" });
		await expect(tips).toBeVisible();
		await expect(tips).toContainText("Press ? to see and change");

		const violations = (await new AxeBuilder({ page }).analyze()).violations;
		expect(
			violations.filter(
				(v) => v.impact === "serious" || v.impact === "critical",
			),
		).toEqual([]);

		// Empty states: library, timeline and history.
		await expect(
			page.getByText("Paste the path of a video, audio or image file above"),
		).toBeVisible();
		await expect(page.getByText("The timeline is empty")).toBeVisible();
		await page.keyboard.press("Alt+3");
		await expect(page.getByText("No changes yet")).toBeVisible();

		// The second load does not show them.
		await page.reload();
		await expect(page.getByRole("button", { name: "Export" })).toBeVisible();
		await expect(tips).toBeHidden();

		// Show tips again from the cheat sheet.
		await page.locator("body").press("?");
		await page.getByRole("button", { name: "Show tips again" }).click();
		await expect(tips).toBeVisible();
		await tips.getByRole("button", { name: "Got it" }).click();
		await expect(tips).toBeHidden();
	});
});

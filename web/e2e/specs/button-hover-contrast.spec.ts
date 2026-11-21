import AxeBuilder from "@axe-core/playwright";
import { expect, fillProjectForm, test } from "./helpers";

// Epic 37 review (VV-224, the axe spec of the export dialog failed only when
// the pointer was left on a button): a primary button under the pointer keeps
// the 4.5:1 of WCAG 1.4.3 in both themes. The hover was primary at 80% over the
// surface, 3.7:1 with white text in the light theme.
for (const theme of ["light", "dark"] as const) {
	test(`a hovered primary button keeps its contrast in the ${theme} theme`, async ({
		page,
	}) => {
		await page.addInitScript((value) => {
			localStorage.setItem("theme", value);
		}, theme);
		await page.goto("/projects");
		// The button is enabled, and so takes the pointer, once the form is valid (epic 56).
		await fillProjectForm(page, "Hover contrast");
		const button = page.getByRole("button", { name: "Create project" }).first();
		await expect(button).toBeEnabled();
		const background = (): Promise<string> =>
			button.evaluate((el) => getComputedStyle(el).backgroundColor);
		const resting = await background();
		await button.hover();
		// The hover has taken effect and its transition has finished.
		await expect.poll(background).not.toBe(resting);
		await button.evaluate((el) =>
			Promise.all(el.getAnimations().map((animation) => animation.finished)),
		);
		const result = await new AxeBuilder({ page })
			.withRules(["color-contrast"])
			.include('[data-slot="button"]')
			.analyze();
		expect(
			result.violations.flatMap((v) => v.nodes.map((n) => n.failureSummary)),
		).toEqual([]);
	});
}

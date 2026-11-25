import { expect, test } from "./helpers";

// VV-443 (WCAG 2.4.3): pressing Tab or Shift+Tab repeatedly inside an open modal
// dialog must cycle through the controls of [role=dialog] and never escape onto
// Base UI's focus-guard spans, document.body, or links behind the modal.
test.describe("a11y-dialog-focus-trap", () => {
	test("Tab and Shift+Tab stay inside the New project modal dialog", async ({
		page,
	}) => {
		await page.goto("/projects");
		const openButton = page.getByRole("button", { name: "New project" });
		await expect(openButton).toBeVisible();
		await openButton.focus();
		await page.keyboard.press("Enter");

		const dialog = page.getByRole("dialog");
		await expect(dialog).toBeVisible();

		const outside: string[] = [];
		const readActive = () =>
			page.evaluate(() => {
				const el = document.activeElement as HTMLElement | null;
				const inDialog = Boolean(el?.closest('[role="dialog"]'));
				return {
					inDialog,
					summary: `${el?.tagName ?? "null"}${el?.id ? `#${el.id}` : ""}`,
				};
			});

		for (let i = 0; i < 14; i++) {
			const state = await readActive();
			if (!state.inDialog) outside.push(`Tab[${i}]: ${state.summary}`);
			await page.keyboard.press("Tab");
		}

		for (let i = 0; i < 14; i++) {
			await page.keyboard.press("Shift+Tab");
			const state = await readActive();
			if (!state.inDialog) outside.push(`Shift+Tab[${i}]: ${state.summary}`);
		}

		expect(outside, "focus left [role=dialog]").toEqual([]);

		await page.keyboard.press("Escape");
		await expect(dialog).toHaveCount(0);
	});
});

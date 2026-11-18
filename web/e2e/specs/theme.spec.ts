import { expect, test } from "./helpers";

// D-024 and SDD-012 section 17: dark by default, whatever the system says.
test.describe("theme-default-dark-and-toggle", () => {
	test.describe("operating system in light mode", () => {
		test.use({ colorScheme: "light" });

		test("a first visit renders dark", async ({ page }) => {
			await page.goto("/projects");

			await expect(page.locator("html")).toHaveClass(/\bdark\b/);
			await expect(page.locator("body")).toHaveCSS(
				"background-color",
				"oklch(0.145 0 0)",
			);
		});

		test("the toggle switches to light and the choice survives a reload", async ({
			page,
		}) => {
			await page.goto("/projects");
			await page.getByRole("button", { name: "Switch to light theme" }).click();
			await expect(page.locator("html")).not.toHaveClass(/\bdark\b/);

			await page.reload();
			await expect(page.locator("html")).not.toHaveClass(/\bdark\b/);
		});
	});

	test.describe("operating system in dark mode", () => {
		test.use({ colorScheme: "dark" });

		test("a first visit renders dark", async ({ page }) => {
			await page.goto("/projects");
			await expect(page.locator("html")).toHaveClass(/\bdark\b/);
		});
	});
});

import { expect, test } from "./helpers";

// D-024 and SDD-012 section 17: dark by default, whatever the system says.
test.describe("theme-default-dark-and-toggle", () => {
	test.describe("operating system in light mode", () => {
		test.use({ colorScheme: "light" });

		test("a first visit renders dark", async ({ page }) => {
			await page.goto("/projects");

			await expect(page.locator("html")).toHaveClass(/\bdark\b/);
			// The body paints the --background token of the dark theme; the token
			// is read, not written here, so a new palette does not break the spec.
			const background = await page.evaluate(() => {
				const probe = document.createElement("div");
				probe.style.backgroundColor = "var(--background)";
				document.body.append(probe);
				const color = getComputedStyle(probe).backgroundColor;
				probe.remove();
				return color;
			});
			expect(background).not.toBe("");
			await expect(page.locator("body")).toHaveCSS(
				"background-color",
				background,
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

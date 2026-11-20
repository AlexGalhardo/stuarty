import { randomBytes } from "node:crypto";
import { caller, expect, test } from "./helpers";

// VV-327: a field with a rule says the rule before it is saved, and a field that is a choice is a choice.
test.describe("field hints", () => {
	test("the scope of a memory note is chosen from the workspace and the projects", async ({
		page,
		request,
	}) => {
		const api = await caller(request);
		const tag = randomBytes(3).toString("hex");
		const made = await api("POST", "/projects", { name: `Scope ${tag}` });

		await page.goto("/memory");
		await page.getByRole("button", { name: "New", exact: true }).click();
		const scope = page.getByLabel("Scope");
		await expect(scope).toHaveJSProperty("tagName", "SELECT");
		await expect(scope.locator("option").first()).toHaveText(/Workspace/);
		await expect(
			page.getByText("A note is for the whole workspace"),
		).toBeVisible();

		await page.getByLabel("Name (optional)").fill(`scoped-${tag}`);
		await page
			.getByLabel("The fact, in one line")
			.fill("Scoped to one project");
		await page.getByLabel("Text", { exact: true }).fill("Body.");
		await scope.selectOption(made.project_id as string);
		await page.getByRole("button", { name: "Save" }).click();
		await expect(page.getByText("Saved.")).toBeVisible();
	});

	test("the brand kit says how a colour is named before it is saved", async ({
		page,
	}) => {
		await page.goto("/settings/brand-kit");
		await page.getByRole("button", { name: "Add colour" }).click();
		const name = page.getByLabel("Name of colour 1");
		await expect(name).toHaveAttribute("placeholder", "brand_red");
		await expect(name).toHaveAccessibleDescription(
			/lower case letters, digits and underscores/,
		);
	});
});

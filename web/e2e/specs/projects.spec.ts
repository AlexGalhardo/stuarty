import { expect, listProjects, test } from "./helpers";

// RF-001 and RF-002: the page creates a project with the chosen size preset
// and opens it in the editor. 1:1 is 1080 by 1080 (RF-002); the check goes
// through the HTTP API.
test.describe("projects-page", () => {
	test("creating a project creates exactly one project with the chosen preset", async ({
		page,
		request,
	}) => {
		const name = `E2E ${Date.now()}`;

		await page.goto("/projects");
		await page.getByLabel("Name").fill(name);
		await page.getByRole("radio", { name: /^1:1/ }).check();
		await page.getByRole("button", { name: "Create project" }).click();
		await expect(page).toHaveURL(/\/editor\/prj_[0-9a-hjkmnp-tv-z]{10}$/);

		// Specs share one workspace and run in parallel: the name is unique, so
		// "exactly one" is counted by name.
		const created = (await listProjects(request)).filter(
			(p) => p.name === name,
		);
		expect(created).toHaveLength(1);
		expect([created[0].width, created[0].height]).toEqual([1080, 1080]);

		await page.goto("/projects");
		await expect(
			page.getByRole("link", { name: new RegExp(`Open.*${name}`) }),
		).toBeVisible();
	});

	test("the default preset is 9:16", async ({ page, request }) => {
		const name = `E2E default ${Date.now()}`;

		await page.goto("/projects");
		await page.getByLabel("Name").fill(name);
		await page.getByRole("button", { name: "Create project" }).click();
		await expect(page).toHaveURL(/\/editor\/prj_/);

		const created = (await listProjects(request)).filter(
			(p) => p.name === name,
		);
		expect(created).toHaveLength(1);
		expect([created[0].width, created[0].height]).toEqual([1080, 1920]);
	});

	test("an open link goes to the editor", async ({ page }) => {
		const name = `E2E open ${Date.now()}`;

		await page.goto("/projects");
		await page.getByLabel("Name").fill(name);
		await page.getByRole("button", { name: "Create project" }).click();
		await expect(page).toHaveURL(/\/editor\/prj_/);
		const id = page.url().split("/").pop();

		await page.goto("/projects");
		await page.getByRole("link", { name: new RegExp(`Open.*${name}`) }).click();
		await expect(page).toHaveURL(new RegExp(`/editor/${id}$`));
	});
});

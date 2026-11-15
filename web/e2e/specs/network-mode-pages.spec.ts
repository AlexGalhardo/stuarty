import { expect, seedProject, test } from "./helpers";

// P2-UI-029 (SDD-012 sections 2.1 and 22.2, RF-005, RF-203): the trash of the
// projects page, and the pages of network mode. The e2e server runs in local
// mode, so what is asserted here is the trash and the 404 of the keys page.
test.describe("network-mode-pages", () => {
	test("a project goes to the trash and comes back", async ({
		page,
		request,
	}) => {
		const label = `E2E trash ${Date.now()}`;
		const seeded = await seedProject(request, { name: label, tracks: 0 });

		await page.goto("/projects");
		const openLink = page.getByRole("link", {
			name: new RegExp(`Open.*${label}`),
		});
		await expect(openLink).toBeVisible();
		await page
			.getByRole("button", { name: new RegExp(`Move to trash.*${label}`) })
			.click();
		await expect(openLink).toHaveCount(0);
		const trashed = page.locator(`[data-trashed-id='${seeded.id}']`);
		await expect(trashed).toBeVisible();

		await trashed.getByRole("button", { name: /^Restore/ }).click();
		await expect(trashed).toHaveCount(0);
		await expect(openLink).toBeVisible();
	});

	test("/settings/keys answers 404 in local mode", async ({ request }) => {
		const response = await request.get("/settings/keys", {
			maxRedirects: 0,
		});
		expect(response.status()).toBe(404);
	});

	test("the network-mode login, keys and logout need a server in network mode", async () => {
		test.fixme(
			true,
			"the e2e config has no network-mode server, and /login and /settings/keys are not routed yet",
		);
	});
});

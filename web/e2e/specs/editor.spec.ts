import { caller, expect, seedProject, test } from "./helpers";

// P1-UI-001 and P1-UI-002 asserts that need the server: the route of
// /editor/{project_id} (P1-SRV-003) and the HTTP API and event stream
// (P1-SRV-005). The unit tests of resources/js/editor cover the same logic
// against the fixture server.
test.describe("editor-route", () => {
	test("an unknown project answers 404", async ({ page }) => {
		const response = await page.goto("/editor/prj_0000000000");
		expect(response?.status()).toBe(404);
	});

	test("reloading restores the same document", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 2, perTrack: 3 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(6);
		await page.reload();
		await expect(page.locator("[data-item-id]")).toHaveCount(6);
	});
});

test.describe("editor-live-edit", () => {
	test("an edit made by another client appears without reload and names the actor", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);

		// The CLI and this call are the same client to the server: a bearer token
		// and the label of Vukem-Video-Client, so the actor is local:cli.
		const cli = await caller(request, "cli");
		const head = (await cli("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		const started = Date.now();
		await cli("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "clip.add",
					args: {
						track_id: seeded.trackIds[0],
						asset_id: seeded.assetId,
						start: "5s",
						source_in: "0s",
						duration: "1s",
					},
				},
			],
		});
		await expect(page.locator("[data-item-id]")).toHaveCount(2, {
			timeout: 2000,
		});
		console.log(
			`live edit visible ${Date.now() - started} ms after the CLI call`,
		);
		// The indicator shows local:cli as "cli" (SDD-012 section 5.8 item 6).
		await expect(page.getByText("cli is editing")).toBeVisible();
	});
});

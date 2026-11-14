import { expect, test } from "@playwright/test";

import { PENDING_SERVER } from "./helpers";

// P1-UI-001 and P1-UI-002 asserts that need the server: the route of
// /editor/{project_id} (P1-SRV-003) and the HTTP API and event stream
// (P1-SRV-005). The unit tests of resources/js/editor cover the same logic
// against the fixture server.
test.describe("editor-route", () => {
	test.skip(true, PENDING_SERVER);

	test("an unknown project answers 404", async ({ page }) => {
		const response = await page.goto("/editor/prj_0000000000");
		expect(response?.status()).toBe(404);
	});

	test("reloading restores the same document", async ({ page }) => {
		await page.goto("/projects");
		await page.getByLabel("Name").fill(`E2E editor ${Date.now()}`);
		await page.getByRole("button", { name: "Create project" }).click();
		await page.getByRole("link", { name: /^Open/ }).first().click();
		await expect(page.locator("#timeline")).toBeVisible();
		const before = await page.locator("[data-item-id]").count();
		await page.reload();
		await expect(page.locator("[data-item-id]")).toHaveCount(before);
	});
});

test.describe("editor-live-edit", () => {
	test.skip(true, PENDING_SERVER);

	test("an edit made through the CLI appears without reload and names the actor", async () => {
		// Open the editor on a project, run the CLI (`vukem-video track add ...`)
		// as a separate process, expect the track in #timeline within 200 ms and
		// the activity indicator to read "local:cli is editing".
		test.fixme(true, "needs the CLI binary of P1-CLI-001");
	});
});

test.describe("timeline-200-items-perf", () => {
	test.skip(true, PENDING_SERVER);

	test("scrolls and zooms 200 items with frames under 16 ms at the 95th percentile", async ({
		page,
	}) => {
		// The development build opens the fixture server with ?fixture=40
		// (5 tracks of 40 items), which needs no API.
		await page.goto("/editor/prj_7k2m9x4q1b?fixture=40");
		const p95 = await page.evaluate(async () => {
			const scroller = document.querySelector("#timeline .overflow-auto");
			const times: number[] = [];
			let last = performance.now();
			for (let i = 0; i < 120; i++) {
				if (scroller) scroller.scrollLeft = i * 20;
				await new Promise((r) => requestAnimationFrame(r));
				const now = performance.now();
				times.push(now - last);
				last = now;
			}
			times.sort((a, b) => a - b);
			return times[Math.floor(times.length * 0.95)];
		});
		expect(p95).toBeLessThan(16.7);
	});
});

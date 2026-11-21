import { mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { caller, expect, makeProject, test } from "./helpers";

// VV-197: the library never asks for the thumbnail of an asset whose import
// failed; the card shows the error instead.
test.describe("failed asset card", () => {
	test("a file that is not media makes no thumbnail request and shows the error", async ({
		page,
		request,
	}) => {
		const dir = join(tmpdir(), `vv-failed-${Date.now()}`);
		mkdirSync(dir, { recursive: true });
		const fake = join(dir, "fake.mp4");
		writeFileSync(fake, "this is not a video");
		const call = await caller(request);
		const created = await makeProject(call, "Failed asset");
		const id = created.project_id as string;
		await call("POST", `/projects/${id}/media`, {
			files: [{ path: fake }],
			wait_ms: 20000,
		});

		const thumbnails: string[] = [];
		page.on("request", (r) => {
			if (r.url().includes("/thumbnail")) thumbnails.push(r.url());
		});
		const refused: string[] = [];
		page.on("response", (r) => {
			if (r.status() === 404) refused.push(r.url());
		});
		await page.goto(`/editor/${id}`);
		const card = page.locator("[data-asset-id]").first();
		await expect(card).toBeVisible();
		await expect(card).toHaveAttribute("data-state", "failed");
		await expect(card.getByRole("alert")).toBeVisible();
		// Wait for the page to stop making requests (a condition, not a duration) before asserting none was a thumbnail.
		await page.waitForLoadState("networkidle");
		expect(thumbnails).toEqual([]);
		expect(refused.filter((u) => u.includes("/media/"))).toEqual([]);
	});
});

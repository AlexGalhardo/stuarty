import { randomBytes } from "node:crypto";

import { zipEntry } from "../support/zip";
import { caller, expect, seedProject, solidClip, test } from "./helpers";

// Epic 60 (D-125, SDD-011 section 14.8, SDD-012 section 6.4): an imported file is
// renamed by a right click or the context-menu key, in the editor and in the
// library; the name is the one the library lists and the one a bundle export
// carries, and the file in the store keeps its hash.
test.describe.configure({ timeout: 180_000 });

// A colour of its own per run: the workspace is shared, so a clip of another
// spec is never renamed, and a second run does not start from a renamed file.
const colour = (): string => `0x${randomBytes(3).toString("hex")}`;

test.describe("asset rename", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test("right click in the editor renames the file; the library and a bundle export show the new name", async ({
		page,
		request,
	}) => {
		const clip = colour();
		const original = `solid-${clip}.mp4`;
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			files: [solidClip(clip)],
		});
		const call = await caller(request);
		await page.goto(`/editor/${seeded.id}`);
		const card = page.locator(`[data-asset-id="${seeded.assetId}"]`);
		await expect(card).toContainText(original);

		// A refusal of the service is shown as it came, and nothing changes.
		await card.click({ button: "right" });
		await page.getByRole("menuitem", { name: "Rename" }).click();
		const field = page
			.getByRole("dialog")
			.getByLabel("File name", { exact: true });
		await expect(field).toHaveValue(original);
		await field.fill("a/b");
		await field.press("Enter");
		await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
			/path separator|name/i,
		);
		await expect(card).toContainText(original);

		// A good name: the extension is kept.
		await field.fill("Beach day");
		await field.press("Enter");
		await expect(page.getByRole("dialog")).toHaveCount(0);
		await expect(card).toContainText("Beach day.mp4");
		await expect(page.locator("[data-item-id]").first()).toHaveAccessibleName(
			/Beach day\.mp4/,
		);

		// The library lists it under the new name, and finds it by it.
		const listed = (await call(
			"GET",
			"/assets?text=" + encodeURIComponent("Beach day"),
		)) as { assets: { name: string; hash: string }[] };
		const mine = listed.assets.find((a) => a.name === "Beach day.mp4");
		expect(mine, "the library lists the renamed file").toBeDefined();
		const hash = mine?.hash ?? "";

		// A bundle export carries the new name.
		const job = (await call("POST", `/projects/${seeded.id}/bundle`, {})) as {
			job_id: string;
		};
		await expect
			.poll(
				async () =>
					((await call("GET", `/jobs/${job.job_id}`)) as { state: string })
						.state,
				{ timeout: 90_000 },
			)
			.toBe("succeeded");
		const file = await page.request.get(`/api/v1/jobs/${job.job_id}/file`);
		expect(file.ok()).toBe(true);
		const manifest = JSON.parse(
			zipEntry(Buffer.from(await file.body()), "manifest.json").toString(
				"utf8",
			),
		) as { media: { hash: string; name: string }[] };
		expect(manifest.media.find((m) => m.hash === hash)?.name).toBe(
			"Beach day.mp4",
		);
	});

	test("the context-menu key renames from the keyboard", async ({
		page,
		request,
	}) => {
		const clip = colour();
		const seeded = await seedProject(request, {
			tracks: 0,
			files: [solidClip(clip)],
		});
		await page.goto(`/editor/${seeded.id}`);
		const card = page.locator(`[data-asset-id="${seeded.assetId}"]`);
		await expect(card).toContainText(`solid-${clip}.mp4`);

		await card.getByRole("button", { name: /^Actions for / }).focus();
		await page.keyboard.press("ContextMenu");
		await page.getByRole("menuitem", { name: "Rename" }).click();
		await page
			.getByRole("dialog")
			.getByLabel("File name", { exact: true })
			.fill("Keyboard name");
		await page.keyboard.press("Enter");
		await expect(card).toContainText("Keyboard name.mp4");
	});

	test("right click on a card of the library page renames the file", async ({
		page,
		request,
	}) => {
		const clip = colour();
		const seeded = await seedProject(request, {
			tracks: 0,
			files: [solidClip(clip)],
		});
		const call = await caller(request);
		const found = (await call("GET", `/assets?project=${seeded.id}`)) as {
			assets: { hash: string }[];
		};
		const hash = found.assets[0]?.hash ?? "";
		expect(hash).toMatch(/^sha256:[0-9a-f]{64}$/);

		await page.goto("/video-assets");
		const card = page.locator(`[data-asset-hash="${hash}"]`);
		await expect(card).toBeVisible();
		await card.click({ button: "right" });
		await page.getByRole("menuitem", { name: "Rename" }).click();
		await page
			.getByRole("dialog")
			.getByLabel("File name", { exact: true })
			.fill("From the library");
		await page.keyboard.press("Enter");
		await expect(page.locator(`[data-asset-hash="${hash}"]`)).toContainText(
			"From the library.mp4",
		);
	});
});

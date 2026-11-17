import type { Page } from "@playwright/test";

import {
	audioClip,
	caller,
	expect,
	seedProject,
	test,
	testClip,
} from "./helpers";

// TASK-0286 (SDD-012 section 6.4): the Audio and Text tabs of the material
// rail. Each card sends one command; the lists come from the core.
async function sent(page: Page, op: string, action: () => Promise<void>) {
	const request = page.waitForRequest(
		(r) =>
			r.method() === "POST" &&
			/\/edit$/.test(new URL(r.url()).pathname) &&
			(r.postData() ?? "").includes(`"${op}"`),
	);
	await action();
	return request;
}

test.describe("library audio and text panels", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test("the Text tab adds text and shapes and applies a preset", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("tab", { name: "Text" }).click();
		const panel = page.getByRole("region", { name: "Text" });
		await expect(panel).toBeVisible();

		await sent(page, "text.add", () =>
			panel.getByRole("button", { name: "Add text" }).click(),
		);
		await sent(page, "shape.add", () =>
			panel.getByRole("button", { name: "Ellipse" }).click(),
		);

		// Nothing selected: the preset is disabled and says why.
		const pop = panel.getByRole("button", { name: "Pop" });
		await expect(pop).toHaveAttribute("aria-disabled", "true");
		await expect(pop).toHaveAccessibleDescription("Select an item first.");

		await page.locator(`[data-item-id="${seeded.itemIds[0]}"]`).click();
		await expect(pop).not.toHaveAttribute("aria-disabled", "true");
		const req = await sent(page, "text.apply_preset", () => pop.click());
		expect(req.postData()).toContain(seeded.itemIds[0]);

		// The fonts come from the core and apply to one selected text item.
		const font = panel.getByRole("list").last().getByRole("button").first();
		await expect(font).toHaveAttribute("aria-disabled", "true");
	});

	test("the Audio tab lists audio assets and sends ducking and beats", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			files: [testClip(), audioClip()],
			tracks: 1,
			audioTracks: 1,
			perTrack: 0,
		});
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		const out = (await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "clip.add",
					args: {
						track_id: seeded.trackIds[0],
						asset_id: seeded.assetIds[0],
						start: "0s",
						source_in: "0s",
						duration: "2s",
					},
				},
				{
					op: "audio.add",
					args: {
						track_id: seeded.audioTrackIds[0],
						asset_id: seeded.assetIds[1],
						start: "0s",
					},
				},
			],
		})) as { effective: { created?: string[] }[] };
		const [clip, tone] = out.effective.map((e) => e.created?.[0]);
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("tab", { name: "Audio" }).click();
		const panel = page.getByRole("region", { name: "Audio" });
		await expect(panel.locator("[data-asset-id]")).toHaveCount(1);

		const duck = panel.getByRole("button", { name: "Duck under voice" });
		const beats = panel.getByRole("button", { name: "Mark beats" });
		await expect(duck).toHaveAttribute("aria-disabled", "true");
		await expect(beats).toHaveAttribute("aria-disabled", "true");

		// The first item selected is the music, the others are speech.
		await page.locator(`[data-item-id="${tone}"]`).click();
		await expect(beats).not.toHaveAttribute("aria-disabled", "true");
		await expect(duck).toHaveAttribute("aria-disabled", "true");
		await page
			.locator(`[data-item-id="${clip}"]`)
			.click({ modifiers: ["Control"] });
		await expect(duck).not.toHaveAttribute("aria-disabled", "true");
		const req = await sent(page, "audio.duck", () => duck.click());
		expect(req.postData()).toContain(String(clip));
		await page.locator(`[data-item-id="${tone}"]`).click();
		await sent(page, "audio.mark_beats", () => beats.click());
	});
});

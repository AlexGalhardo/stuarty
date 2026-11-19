import type { Page } from "@playwright/test";
import { audioClip, expect, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const edits: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			edits.push(r.postDataJSON());
		}
	});
	return edits;
}

async function center(page: Page, selector: string, nth = 0) {
	const box = await page.locator(selector).nth(nth).boundingBox();
	if (box === null) throw new Error(`${selector} is not laid out`);
	return { x: box.x + box.width / 2, y: box.y + box.height / 2, box };
}

async function dragCardTo(
	page: Page,
	card: string,
	to: { x: number; y: number },
): Promise<void> {
	const from = await center(page, card);
	await page.mouse.move(from.x, from.y);
	await page.mouse.down();
	await page.mouse.move(to.x, to.y, { steps: 10 });
}

// P2-UI-014 (SDD-012 sections 8.2 and 13.2): a library card dragged onto a row
// sends one clip.add or audio.add for the row under the pointer; a card that is
// not ready cannot start a drag; an asset in use opens the dialog of its items.
test.describe("library-drag", () => {
	test("dropping a ready video asset sends one clip.add on the track under the pointer", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 2, perTrack: 0 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-track-id]")).toHaveCount(2);
		const card = `[data-asset-id="${seeded.assetId}"]`;
		await expect(page.locator(card)).toHaveAttribute("data-draggable", "true");
		const edits = recordEdits(page);

		const target = seeded.trackIds[1] as string;
		const row = await center(page, `[data-track-id="${target}"]`);
		await dragCardTo(page, card, { x: row.box.x + 240, y: row.y });
		await expect(page.getByTestId("ghost-layer")).toBeVisible();
		await expect(page.locator("[data-ghost-track]")).toHaveAttribute(
			"data-ghost-track",
			target,
		);
		await page.mouse.up();

		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toHaveLength(1);
		expect(edits[0]?.operations[0]?.op).toBe("clip.add");
		expect(edits[0]?.operations[0]?.args.track_id).toBe(target);
		expect(edits[0]?.operations[0]?.args.asset_id).toBe(seeded.assetId);
		expect(String(edits[0]?.operations[0]?.args.start)).toMatch(/^\d+f$/);
		await expect(page.getByTestId("ghost-layer")).toHaveCount(0);
	});

	test("dropping an audio asset on an audio row sends one audio.add; Add to timeline creates the track first when there is none", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 0,
			audioTracks: 1,
			files: [audioClip(0)],
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-track-id]")).toHaveCount(2);
		const card = `[data-asset-id="${seeded.assetId}"]`;
		await expect(page.locator(card)).toHaveAttribute("data-draggable", "true");
		const edits = recordEdits(page);

		const row = await center(
			page,
			`[data-track-id="${seeded.audioTrackIds[0]}"]`,
		);
		await dragCardTo(page, card, { x: row.box.x + 120, y: row.y });
		await page.mouse.up();
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations.map((o) => o.op)).toEqual(["audio.add"]);
		expect(edits[0]?.operations[0]?.args.track_id).toBe(
			seeded.audioTrackIds[0],
		);

		// A project with an audio card and no audio track.
		const bare = await seedProject(request, {
			tracks: 1,
			perTrack: 0,
			files: [audioClip(2)],
		});
		await page.goto(`/editor/${bare.id}?debug=1`);
		const sent = recordEdits(page);
		await page
			.locator(`[data-asset-id="${bare.assetId}"]`)
			.getByRole("button", { name: "Add to timeline" })
			.click();
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		expect(sent).toHaveLength(1);
		expect(sent[0]?.operations.map((o) => o.op)).toEqual([
			"track.add",
			"audio.add",
		]);
		expect(String(sent[0]?.operations[0]?.args.id)).toMatch(/^trk_/);
		expect(sent[0]?.operations[1]?.args.track_id).toBe(
			sent[0]?.operations[0]?.args.id,
		);
	});

	test("a card that is not ready cannot start a drag", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await page.route("**/media*", async (route) => {
			if (route.request().method() !== "GET") return route.continue();
			const response = await route.fetch();
			const body = (await response.json()) as {
				assets: { state: string; progress: number }[];
			};
			for (const asset of body.assets) {
				asset.state = "ingesting";
				asset.progress = 0.4;
			}
			await route.fulfill({ response, json: body });
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const card = `[data-asset-id="${seeded.assetId}"]`;
		await expect(page.locator(card)).toHaveAttribute("data-state", "ingesting");
		await expect(page.locator(card)).toHaveAttribute("data-draggable", "false");
		const edits = recordEdits(page);

		const row = await center(page, "[data-track-id]");
		await dragCardTo(page, card, { x: row.box.x + 200, y: row.y });
		await expect(page.getByTestId("ghost-layer")).toHaveCount(0);
		await page.mouse.up();
		expect(edits).toHaveLength(0);
	});

	test("removing an asset in use shows its items as links that select them", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		// The card of the library, not the menu of a track header, comes first.
		await expect(page.locator("[data-asset-id]")).toHaveCount(1);
		await page
			.getByRole("button", { name: /^Actions for/ })
			.first()
			.click();
		await page.getByRole("menuitem", { name: "Remove from project" }).click();

		const dialog = page.getByRole("dialog");
		await expect(dialog).toContainText("still in use");
		const used = seeded.itemIds[1] as string;
		await dialog.locator(`[data-used-by="${used}"]`).click();
		await expect(dialog).toHaveCount(0);
		await expect(page.locator(`[data-item-id="${used}"]`)).toHaveAttribute(
			"aria-selected",
			"true",
		);
	});

	test("Move to trash and Restore move a card in and out of the Trash group", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const card = `[data-asset-id="${seeded.assetId}"]`;
		await page
			.locator(card)
			.getByRole("button", { name: /^Actions for/ })
			.click();
		await page.getByRole("menuitem", { name: "Move to trash" }).click();
		const group = page.getByTestId("trash-group");
		await expect(group).toBeVisible();
		await expect(group).not.toHaveAttribute("open", "");
		await group.locator("summary").click();
		await group
			.locator(card)
			.getByRole("button", { name: /^Actions for/ })
			.click();
		await page.getByRole("menuitem", { name: "Restore" }).click();
		await expect(page.getByTestId("trash-group")).toHaveCount(0);
		await expect(page.locator(card)).toHaveAttribute("data-draggable", "true");
	});
});

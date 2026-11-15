import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

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

async function headerOrder(page: Page): Promise<string[]> {
	return page
		.locator("[data-header-for]")
		.evaluateAll((els) =>
			els.map((el) => (el as HTMLElement).dataset.headerFor ?? ""),
		);
}

// P2-UI-023 (SDD-012 sections 7.4 and 8.2): the header of a track edits it,
// and each control sends one batch.
test.describe("tracks", () => {
	test("reordering by the handle sends one track.move and the order follows the core", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 3, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-header-for]")).toHaveCount(3);
		const before = await headerOrder(page);
		const edits = recordEdits(page);
		const answered = page.waitForResponse(
			(r) => r.url().endsWith("/edit") && r.status() === 200,
		);

		const handle = page.locator(`[data-track-handle="${before[0]}"]`);
		const last = page.locator(`[data-header-for="${before[2]}"]`);
		const from = await handle.boundingBox();
		const to = await last.boundingBox();
		if (from === null || to === null) throw new Error("no boxes");
		await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
		await page.mouse.down();
		await page.mouse.move(from.x + 4, from.y + 40, { steps: 4 });
		// The last header can end below the window; Firefox drops pointer positions
		// outside it, so the drag stops at the window edge, past the header middle.
		const bottom = Math.min(
			to.y + to.height - 4,
			(page.viewportSize()?.height ?? 720) - 2,
		);
		await page.mouse.move(from.x + 4, bottom, { steps: 8 });
		await expect(page.getByTestId("track-ghost")).toBeVisible();
		await page.mouse.up();
		await answered;

		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toHaveLength(1);
		expect(edits[0]?.operations[0]?.op).toBe("track.move");
		expect(edits[0]?.operations[0]?.args).toEqual({
			track_id: before[0],
			below: before[2],
		});
		await expect(page.getByTestId("track-ghost")).toHaveCount(0);
		await expect
			.poll(() => headerOrder(page))
			.toEqual([before[1], before[2], before[0]]);
	});

	test("Esc during the drag sends nothing", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 2, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-header-for]")).toHaveCount(2);
		const before = await headerOrder(page);
		const edits = recordEdits(page);
		const from = await page
			.locator(`[data-track-handle="${before[0]}"]`)
			.boundingBox();
		const to = await page
			.locator(`[data-header-for="${before[1]}"]`)
			.boundingBox();
		if (from === null || to === null) throw new Error("no boxes");
		await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
		await page.mouse.down();
		await page.mouse.move(from.x + 4, to.y + to.height - 4, { steps: 8 });
		await page.keyboard.press("Escape");
		await page.mouse.up();
		expect(edits).toHaveLength(0);
		expect(await headerOrder(page)).toEqual(before);
	});

	test("removing a non-empty track asks first and resends with with_items", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 2, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-header-for]")).toHaveCount(2);
		const target = (await headerOrder(page))[0] ?? "";
		const edits = recordEdits(page);

		await page
			.locator(`[data-header-for="${target}"]`)
			.getByRole("button", { name: /^Actions for track/ })
			.click();
		await page.getByRole("menuitem", { name: "Remove track" }).click();

		const dialog = page.getByRole("alertdialog");
		await expect(dialog).toBeVisible();
		await expect(dialog).toContainText("2 items");
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations[0]?.op).toBe("track.remove");
		expect(edits[0]?.operations[0]?.args).toEqual({ track_id: target });

		await dialog.getByRole("button", { name: "Cancel" }).click();
		expect(edits).toHaveLength(1);
		await expect(page.locator(`[data-header-for="${target}"]`)).toHaveCount(1);

		await page
			.locator(`[data-header-for="${target}"]`)
			.getByRole("button", { name: /^Actions for track/ })
			.click();
		await page.getByRole("menuitem", { name: "Remove track" }).click();
		await page
			.getByRole("alertdialog")
			.getByRole("button", { name: "Remove track and items" })
			.click();
		await expect(page.locator(`[data-header-for="${target}"]`)).toHaveCount(0);
		expect(edits).toHaveLength(3);
		expect(edits[2]?.operations).toHaveLength(1);
		expect(edits[2]?.operations[0]?.args).toEqual({
			track_id: target,
			with_items: true,
		});
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
	});

	test("an empty track is removed at once, without a dialog", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 2, perTrack: 0 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-header-for]")).toHaveCount(2);
		const target = (await headerOrder(page))[0] ?? "";
		const edits = recordEdits(page);
		await page
			.locator(`[data-header-for="${target}"]`)
			.getByRole("button", { name: /^Actions for track/ })
			.click();
		await page.getByRole("menuitem", { name: "Remove track" }).click();
		await expect(page.locator(`[data-header-for="${target}"]`)).toHaveCount(0);
		expect(edits).toHaveLength(1);
		await expect(page.getByRole("alertdialog")).toHaveCount(0);
	});

	test("rename, hide and mute each send one track.update", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const header = page.locator("[data-header-for]");
		await expect(header).toHaveCount(1);
		const id = (await headerOrder(page))[0] ?? "";
		const edits = recordEdits(page);

		await header.getByText("Video 1").dblclick();
		const field = header.getByRole("textbox", { name: "Track name" });
		await field.fill("Main");
		await field.press("Enter");
		await expect(header).toContainText("Main");

		await header.getByRole("button", { name: "Hide track" }).click();
		await expect(
			header.getByRole("button", { name: "Show track" }),
		).toHaveAttribute("aria-pressed", "true");
		await header.getByRole("button", { name: "Mute track" }).click();
		await expect(
			header.getByRole("button", { name: "Unmute track" }),
		).toBeVisible();
		expect(edits.map((e) => e.operations)).toEqual([
			[{ op: "track.update", args: { track_id: id, name: "Main" } }],
			[{ op: "track.update", args: { track_id: id, hidden: true } }],
			[{ op: "track.update", args: { track_id: id, muted: true } }],
		]);
	});

	test("the menu adds a track next to this one and moves it up and down", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 2, perTrack: 0 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-header-for]")).toHaveCount(2);
		const [first, second] = await headerOrder(page);
		const edits = recordEdits(page);
		const open = (id: string) =>
			page
				.locator(`[data-header-for="${id}"]`)
				.getByRole("button", { name: /^Actions for track/ })
				.click();

		await open(first ?? "");
		await page.getByRole("menuitem", { name: "Add audio track below" }).click();
		await expect(page.locator("[data-header-for]")).toHaveCount(3);
		expect(edits[0]?.operations[0]).toEqual({
			op: "track.add",
			args: { kind: "audio", below: first },
		});
		const added = (await headerOrder(page))[1] ?? "";
		expect(await headerOrder(page)).toEqual([first, added, second]);

		await open(second ?? "");
		await page.getByRole("menuitem", { name: "Move up" }).click();
		await expect.poll(() => headerOrder(page)).toEqual([first, second, added]);
		expect(edits[1]?.operations).toEqual([
			{ op: "track.move", args: { track_id: second, above: added } },
		]);
		await open(first ?? "");
		await expect(
			page.getByRole("menuitem", { name: "Move up" }),
		).toBeDisabled();
		await page.keyboard.press("Escape");
	});

	test("the track panel sets the caption style merged, or replaced when asked", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const header = page.locator("[data-header-for]");
		await expect(header).toHaveCount(1);
		const id = (await headerOrder(page))[0] ?? "";
		await header.click();
		const edits = recordEdits(page);

		const size = page.getByLabel("Size", { exact: true });
		await size.fill("72");
		await size.press("Enter");
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{
				op: "track.set_caption_style",
				args: { track_id: id, style: { size: 72 } },
			},
		]);

		await page.getByLabel("Replace the whole style").check();
		const stroke = page.getByLabel("Stroke width", { exact: true });
		await stroke.fill("3");
		await stroke.press("Enter");
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations).toEqual([
			{
				op: "track.set_caption_style",
				args: { track_id: id, style: { stroke_width: 3 }, replace: true },
			},
		]);
	});
});

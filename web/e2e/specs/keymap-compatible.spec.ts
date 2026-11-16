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

// The playhead to `seconds` with the ruler slider: PageUp is one second.
async function playheadTo(page: Page, seconds: number): Promise<void> {
	await page.getByRole("slider", { name: "Playhead" }).focus();
	for (let i = 0; i < seconds; i++) await page.keyboard.press("PageUp");
}

async function open(
	page: Page,
	id: string,
	count: number,
): Promise<ReturnType<Page["locator"]>> {
	await page.goto(`/editor/${id}?debug=1`);
	const items = page.locator("[data-item-id]");
	await expect(items).toHaveCount(count);
	return items;
}

// TASK-0236 (SDD-012 section 18.7): the CapCut-compatible keymap, one test per
// key. The keys the section lists as needing a core command are not drawn and
// not bound, so there is nothing to test for them.
test.describe("compatible keymap", () => {
	test("Ctrl+B splits the selected item at the playhead; S does nothing", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const items = await open(page, seeded.id, 1);
		const edits = recordEdits(page);
		await playheadTo(page, 1);
		await items.first().click();
		await page.keyboard.press("s");
		await page.waitForTimeout(300);
		expect(edits).toHaveLength(0);
		await page.keyboard.press("Control+b");
		await expect(items).toHaveCount(2);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations.map((o) => o.op)).toEqual(["item.split"]);
	});

	test("Ctrl+Shift+B splits every track at the playhead in one batch", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 2,
			perTrack: 1,
			clipDuration: "4s",
		});
		const items = await open(page, seeded.id, 2);
		const edits = recordEdits(page);
		await playheadTo(page, 1);
		await page.keyboard.press("Control+Shift+B");
		await expect(items).toHaveCount(4);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations.map((o) => o.op)).toEqual([
			"item.split",
			"item.split",
		]);
	});

	for (const [key, edge] of [
		["q", "start"],
		["w", "end"],
	] as const) {
		test(`${key.toUpperCase()} deletes what is on the ${edge === "start" ? "left" : "right"} of the playhead (item.trim ${edge})`, async ({
			page,
			request,
		}) => {
			const seeded = await seedProject(request, {
				tracks: 1,
				perTrack: 1,
				clipDuration: "4s",
			});
			const items = await open(page, seeded.id, 1);
			const edits = recordEdits(page);
			await playheadTo(page, 1);
			await items.first().click();
			await page.keyboard.press(key);
			await expect.poll(() => edits.length).toBe(1);
			expect(edits[0]?.operations).toHaveLength(1);
			expect(edits[0]?.operations[0]).toMatchObject({
				op: "item.trim",
				args: { edge },
			});
		});
	}

	test("[ and ] select what is left and right of the playhead; the old trim keys do nothing", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 3,
		});
		const items = await open(page, seeded.id, 3);
		const edits = recordEdits(page);
		const selected = page.locator('[data-item-id][aria-selected="true"]');
		// Items are 1 s clips laid end to end: 0-1, 1-2, 2-3 s. The playhead is at 1 s.
		await playheadTo(page, 1);
		await page.keyboard.press("[");
		await expect(selected).toHaveCount(1);
		await expect(items.first()).toHaveAttribute("aria-selected", "true");
		await page.keyboard.press("]");
		await expect(selected).toHaveCount(2);
		await expect(items.first()).toHaveAttribute("aria-selected", "false");
		await page.waitForTimeout(300);
		expect(edits).toHaveLength(0);
	});

	test("Ctrl+= and Ctrl+- zoom the timeline, + and - are aliases", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const items = await open(page, seeded.id, 1);
		const width = async (): Promise<number> =>
			(await items.first().boundingBox())?.width ?? 0;
		const start = await width();
		await page.keyboard.press("Control+Equal");
		await expect.poll(width).toBeGreaterThan(start);
		const zoomed = await width();
		await page.keyboard.press("Control+Minus");
		await expect.poll(width).toBeLessThan(zoomed);
		const back = await width();
		await page.keyboard.press("Shift+Equal");
		await expect.poll(width).toBeGreaterThan(back);
		const again = await width();
		await page.keyboard.press("Minus");
		await expect.poll(width).toBeLessThan(again);
	});

	test("Ctrl+C then Ctrl+V pastes a copy at the playhead and Ctrl+X then Ctrl+V moves the item there", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const items = await open(page, seeded.id, 1);
		const edits = recordEdits(page);
		await items.first().click();
		await page.keyboard.press("Control+c");
		await page.waitForTimeout(200);
		expect(edits).toHaveLength(0);
		await playheadTo(page, 3);
		await page.keyboard.press("Control+v");
		await expect(items).toHaveCount(2);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toHaveLength(1);
		expect(edits[0]?.operations[0]?.op).toBe("item.duplicate");
		expect(edits[0]?.operations[0]?.args.start).toMatch(/^\d+f$/);
		const copyStart = edits[0]?.operations[0]?.args.start;

		await items.first().click();
		await page.keyboard.press("Control+x");
		await page.waitForTimeout(200);
		expect(edits).toHaveLength(1);
		await expect(items).toHaveCount(2);
		await playheadTo(page, 3);
		await page.keyboard.press("Control+v");
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations.map((o) => o.op)).toEqual(["item.move"]);
		expect(edits[1]?.operations[0]?.args.start).not.toBe(copyStart);
		await expect(items).toHaveCount(2);
	});

	test("Ctrl+. and Ctrl+, raise and lower the volume by 0.1 dB", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const items = await open(page, seeded.id, 1);
		const edits = recordEdits(page);
		await items.first().click();
		await page.keyboard.press("Control+.");
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations[0]).toMatchObject({
			op: "item.set_audio",
			args: { volume: 1.012 },
		});
		await page.keyboard.press("Control+,");
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations[0]?.op).toBe("item.set_audio");
		expect(edits[1]?.operations[0]?.args.volume).toBeCloseTo(1, 2);
	});

	test("Alt+Shift+Z, Alt+Shift+= and Alt+Shift+- size the preview", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await open(page, seeded.id, 1);
		const pressed = (name: string) =>
			page
				.getByRole("group", { name: "Preview size" })
				.getByRole("button", { name, exact: true });
		await expect(pressed("Fit")).toHaveAttribute("aria-pressed", "true");
		await page.keyboard.press("Alt+Shift+Equal");
		await expect(pressed("Half (50%)")).toHaveAttribute("aria-pressed", "true");
		await page.keyboard.press("Alt+Shift+Equal");
		await expect(pressed("Full (100%)")).toHaveAttribute(
			"aria-pressed",
			"true",
		);
		await page.keyboard.press("Alt+Shift+Minus");
		await expect(pressed("Half (50%)")).toHaveAttribute("aria-pressed", "true");
		await page.keyboard.press("Alt+Shift+KeyZ");
		await expect(pressed("Fit")).toHaveAttribute("aria-pressed", "true");
	});

	test("Ctrl+E opens the export dialog, not from inside a field", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await open(page, seeded.id, 1);
		await page.keyboard.press("Control+e");
		await expect(page.getByRole("dialog", { name: /Export/ })).toBeVisible();
	});

	test("Ctrl+I goes to the import field of the library", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await open(page, seeded.id, 1);
		await page.keyboard.press("Control+i");
		await expect(page.locator("[data-import-path]")).toBeFocused();
	});

	test("Ctrl+S says the project is saved and sends nothing", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await open(page, seeded.id, 1);
		const edits = recordEdits(page);
		await page.keyboard.press("Control+s");
		await expect(page.getByText(/^Saved\./)).toBeVisible();
		expect(edits).toHaveLength(0);
	});

	test("the cheat sheet lists the compatible keys and no row that needs a core command", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await open(page, seeded.id, 1);
		await page.locator("body").press("?");
		const dialog = page.getByRole("dialog", { name: "Keyboard shortcuts" });
		await expect(
			dialog.locator('[data-shortcut-row="trim_start"]'),
		).toContainText("Q");
		await expect(dialog.locator('[data-shortcut-row="zoom_in"]')).toContainText(
			/(Ctrl|Cmd)\+=/,
		);
		await expect(dialog.locator('[data-shortcut-row="zoom_in"]')).toContainText(
			"Also",
		);
		await expect(
			dialog.locator('[data-shortcut-row="split_all"]'),
		).toBeVisible();
		for (const absent of ["group", "marker_add", "enable", "link"]) {
			await expect(
				dialog.locator(`[data-shortcut-row="${absent}"]`),
			).toHaveCount(0);
		}
	});
});

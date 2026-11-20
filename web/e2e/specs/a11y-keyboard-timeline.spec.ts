import { unnamedControls } from "../support/keyboard";
import { caller, expect, seedProject, test, testClip } from "./helpers";

// Epic 41 task 4 (SDD-012 section 18.10): the timeline is a custom widget, so it has a documented keyboard
// model, a text alternative of the selection in the polite region, and a list view that mirrors the canvas as
// a table. The spec drives them with the keyboard and reads the accessibility tree.
test.describe("a11y-keyboard-timeline", () => {
	async function seedTwoClips(
		request: Parameters<typeof seedProject>[0],
	): Promise<{ id: string; call: Awaited<ReturnType<typeof caller>> }> {
		const seeded = await seedProject(request, {
			files: [testClip()],
			tracks: 1,
			perTrack: 0,
		});
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: ["0s", "3s"].map((start) => ({
				op: "clip.add",
				args: {
					track_id: seeded.trackIds[0],
					asset_id: seeded.assetIds[0],
					start,
					source_in: "0s",
					duration: "2s",
				},
			})),
		});
		return { id: seeded.id, call };
	}

	test("arrows move between items, Enter selects and says what is selected, Alt+Arrow nudges", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedTwoClips(request);
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		const live = page.locator("[data-live-region]");

		await items.first().focus();
		await page.keyboard.press("ArrowRight");
		await expect(items.nth(1)).toBeFocused();
		await page.keyboard.press("ArrowLeft");
		await expect(items.first()).toBeFocused();

		await page.keyboard.press("Enter");
		await expect(items.first()).toHaveAttribute("aria-selected", "true");
		await expect(live).toHaveText(
			/^Selected .*, Clip, 00:00\.000 to 00:02\.000, track Video 1$/,
		);

		await page.keyboard.press("Shift+ArrowRight");
		await expect(live).toHaveText("2 items selected");

		await page.keyboard.press("Escape");
		await expect(live).toHaveText("Selection cleared");

		// Nudge: one frame right, announced.
		await items.nth(1).focus();
		await page.keyboard.press("Enter");
		const before = await items.nth(1).boundingBox();
		await page.keyboard.press("Alt+ArrowRight");
		await expect(live).toHaveText("Moved, 1 item");
		await expect
			.poll(async () => (await items.nth(1).boundingBox())?.x ?? 0)
			.toBeGreaterThan(before?.x ?? 0);
	});

	test("the list view mirrors the canvas as a table and selects from it", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedTwoClips(request);
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		const live = page.locator("[data-live-region]");

		const open = page.getByTestId("timeline-table-open");
		await open.focus();
		await page.keyboard.press("Enter");
		const dialog = page.getByRole("dialog", { name: "Timeline as a table" });
		await expect(dialog).toBeVisible();

		const table = dialog.getByRole("table", { name: /Timeline items, 2 rows/ });
		await expect(table.getByRole("columnheader")).toHaveText([
			"Name",
			"Type",
			"Track",
			"Start",
			"End",
			"Duration",
			"State",
		]);
		const rows = table.getByRole("row");
		await expect(rows).toHaveCount(3);
		await expect(rows.nth(1).getByRole("cell").nth(2)).toHaveText("00:00.000");
		await expect(rows.nth(2).getByRole("cell").nth(2)).toHaveText("00:03.000");

		await dialog.getByTestId("timeline-table-select").nth(1).focus();
		await page.keyboard.press("Enter");
		await expect(dialog).toHaveCount(0);
		await expect(page.locator("[data-item-id]").nth(1)).toHaveAttribute(
			"aria-selected",
			"true",
		);
		await expect(live).toHaveText(/00:03\.000 to 00:05\.000, track Video 1$/);
		// Closing a dialog gives the focus back to its opener.
		await expect(open).toBeFocused();

		// Escape closes it too.
		await page.keyboard.press("Enter");
		await expect(dialog).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(dialog).toHaveCount(0);
		await expect(open).toBeFocused();
	});

	test("every control of the timeline has a name and a role in the accessibility tree", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedTwoClips(request);
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		const tree = await page
			.getByRole("region", { name: "Timeline", exact: true })
			.ariaSnapshot();
		expect(tree).toContain("grid ");
		expect(tree).toMatch(/gridcell "Clip, /);
		expect(tree).toMatch(/slider "Playhead"/);
		expect(tree).toMatch(/rowheader/);
		expect(unnamedControls(tree)).toEqual([]);
		const editor = await page.locator("body").ariaSnapshot();
		expect(unnamedControls(editor)).toEqual([]);
	});
});

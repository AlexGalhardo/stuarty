import { caller, expect, seedProject, test } from "./helpers";

// P2-UI-030 (SDD-012 sections 18.1, 18.2 and 18.4 item 3): split, trim, nudge,
// delete, undo and export with the keyboard alone. The page counts the pointer
// events it receives; the spec asserts there were none.
test.describe("keyboard-only-edit", () => {
	test("split, trim, nudge, delete, undo and export without a pointer", async ({
		page,
		request,
	}) => {
		test.setTimeout(180_000);
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.addInitScript(() => {
			const counts = { pointer: 0 };
			(
				window as unknown as { __pointerEvents: typeof counts }
			).__pointerEvents = counts;
			for (const type of [
				"pointerdown",
				"pointerup",
				"mousedown",
				"mouseup",
				"dblclick",
				"contextmenu",
			]) {
				window.addEventListener(type, () => (counts.pointer += 1), true);
			}
			// A click from the keyboard (Enter on a button) has detail 0.
			window.addEventListener(
				"click",
				(event) => {
					if (event.detail > 0) counts.pointer += 1;
				},
				true,
			);
		});
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(1);
		const live = page.locator("[data-live-region]");

		// Playhead to 1 s with the ruler slider: PageUp is one second.
		await page.getByRole("slider", { name: "Playhead" }).focus();
		await page.keyboard.press("PageUp");

		// Split: Ctrl+B on the clip.
		await items.first().focus();
		await page.keyboard.press("Enter");
		await page.keyboard.press("Control+b");
		await expect(items).toHaveCount(2);
		await expect(live).toHaveText("Split, 1 item");

		// Trim: the playhead to 2 s, the second item selected, Q deletes what is left of the playhead.
		await page.getByRole("slider", { name: "Playhead" }).focus();
		await page.keyboard.press("PageUp");
		await items.nth(1).focus();
		await page.keyboard.press("Enter");
		const before = await items.nth(1).boundingBox();
		await page.keyboard.press("q");
		// Q trims with ripple on the track (SDD-012 section 18.7), so the item
		// keeps its place and gets shorter by the second that was left of the
		// playhead.
		await expect
			.poll(async () => (await items.nth(1).boundingBox())?.width ?? 0)
			.toBeLessThan((before?.width ?? 0) - 1);

		// Nudge: Alt+Right moves the focused item by one frame.
		await items.nth(1).focus();
		await page.keyboard.press("Alt+ArrowRight");
		await expect(live).toHaveText("Moved, 1 item");

		// Delete the selected item, then undo it.
		await page.keyboard.press("Delete");
		await expect(items).toHaveCount(1);
		await expect(live).toHaveText("Deleted, 1 item");
		await page.keyboard.press("Control+z");
		await expect(items).toHaveCount(2);
		await expect(live).toHaveText("Undone");

		// Export: the button and the dialog are reached by focus and Enter.
		await page.getByRole("button", { name: "Export" }).focus();
		await page.keyboard.press("Enter");
		const dialog = page.getByRole("dialog");
		await expect(dialog).toBeVisible();
		// The dialog lints the project first and keeps the button disabled until it
		// answers; focus() on a disabled button does nothing and the Enter that
		// follows would land on another control of the dialog (the close button),
		// so wait for the button to be enabled.
		const start = dialog.getByRole("button", { name: "Start export" });
		await expect(start).toBeEnabled();
		await start.focus();
		await expect(start).toBeFocused();
		await page.keyboard.press("Enter");
		await expect(dialog.locator("[data-phase='succeeded']")).toBeVisible({
			timeout: 120_000,
		});
		await page.keyboard.press("Escape");
		await expect(dialog).toHaveCount(0);

		const pointerEvents = await page.evaluate(
			() =>
				(window as unknown as { __pointerEvents: { pointer: number } })
					.__pointerEvents.pointer,
		);
		expect(pointerEvents, "pointer events dispatched").toBe(0);
	});

	test("five remote edits in one second are announced once, and F6 walks the regions", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);

		const live = page.locator("[data-live-region]");
		await live.evaluate((el) => {
			const log: string[] = [];
			(window as unknown as { __live: string[] }).__live = log;
			new MutationObserver(() => log.push(el.textContent ?? "")).observe(el, {
				childList: true,
				characterData: true,
				subtree: true,
			});
		});
		const agent = await caller(request, "claude-code");
		for (let i = 0; i < 5; i++) {
			const head = (await agent("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			await agent("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations: [
					{
						op: "item.rename",
						args: { item_id: seeded.itemIds[0], name: `Remote ${i}` },
					},
				],
			});
		}
		await expect(live).toHaveText("claude-code changed the project");
		await page.waitForTimeout(500);
		const log = await page.evaluate(
			() => (window as unknown as { __live: string[] }).__live,
		);
		expect(log.filter((text) => text !== "")).toHaveLength(1);

		// F6 visits the regions in order: top bar, library, preview, timeline.
		await page.locator("body").focus();
		const visited: string[] = [];
		for (let i = 0; i < 5; i++) {
			await page.keyboard.press("F6");
			visited.push(
				await page.evaluate(
					() =>
						document.activeElement?.closest("[data-landmark]")?.tagName ?? "",
				),
			);
		}
		expect(visited.slice(0, 4)).toEqual(["HEADER", "ASIDE", "MAIN", "SECTION"]);
		expect(visited[4]).toBe("ASIDE");
	});
});

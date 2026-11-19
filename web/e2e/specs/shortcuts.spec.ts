import { expect, seedProject, test } from "./helpers";

// P3-UI-001 (SDD-012 section 18.2): the cheat sheet opens with ?, rebinding
// Mod+B to X makes X split (one batch) and Mod+B do nothing, and the binding
// survives a reload; a key another row holds is refused; reset brings Mod+B back.
test.describe("shortcuts", () => {
	test("rebinding Mod+B to X splits with X, keeps across a reload, conflicts are refused", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const batches: string[][] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				const body = r.postDataJSON() as { operations: { op: string }[] };
				batches.push(body.operations.map((o) => o.op));
			}
		});
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(1);

		// The cheat sheet is generated from the table and opens with ?.
		await page.locator("body").press("?");
		const dialog = page.getByRole("dialog", { name: "Keyboard shortcuts" });
		await expect(dialog).toBeVisible();
		await expect(dialog.locator('[data-shortcut-row="split"]')).toContainText(
			/(Ctrl|Cmd)\+B/,
		);
		await expect(dialog.locator('[data-shortcut-row="undo"]')).toContainText(
			/(Ctrl|Cmd)\+Z/,
		);

		// A key that another row holds is refused and names that row.
		const split = dialog.locator('[data-shortcut-key="split"]');
		await split.click();
		// The button says it waits for a key, to a screen reader too.
		await expect(split).toHaveAccessibleName("Press a key");
		await split.press("c");
		await expect(dialog.getByRole("alert")).toContainText("Add caption");
		await expect(split).toContainText(/(Ctrl|Cmd)\+B/);

		// Mod+B to X.
		await split.click();
		await split.press("x");
		await expect(split).toContainText("X");
		await page.keyboard.press("Escape");
		await expect(dialog).toBeHidden();

		await page.getByRole("slider", { name: "Playhead" }).focus();
		await page.keyboard.press("PageUp");
		await items.first().focus();
		await page.keyboard.press("Enter");

		await page.keyboard.press("Control+b");
		await page.waitForTimeout(500);
		await expect(items).toHaveCount(1);
		expect(batches).toHaveLength(0);

		await page.keyboard.press("x");
		await expect(items).toHaveCount(2);
		expect(batches).toHaveLength(1);

		// The binding survives a reload.
		await page.reload();
		await expect(items).toHaveCount(2);
		await page.locator("body").press("?");
		await expect(
			page
				.getByRole("dialog", { name: "Keyboard shortcuts" })
				.locator('[data-shortcut-key="split"]'),
		).toContainText("X");

		// Reset to defaults.
		await page.getByRole("button", { name: "Reset to defaults" }).click();
		await expect(
			page
				.getByRole("dialog", { name: "Keyboard shortcuts" })
				.locator('[data-shortcut-key="split"]'),
		).toContainText(/(Ctrl|Cmd)\+B/);
	});

	test("every row has a key button, the dialog is searchable and a conflicting key is refused across scopes", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.locator("body").press("?");
		const dialog = page.getByRole("dialog", { name: "Keyboard shortcuts" });
		await expect(dialog).toBeVisible();

		// The search field has the focus and filters by action and by key.
		const search = dialog.getByRole("searchbox", { name: "Search shortcuts" });
		await expect(search).toBeFocused();
		const rows = dialog.locator("[data-shortcut-row]");
		const all = await rows.count();
		expect(all).toBeGreaterThan(70);
		await search.fill("split");
		await expect(dialog.locator('[data-shortcut-row="split"]')).toBeVisible();
		await expect(dialog.locator('[data-shortcut-row="undo"]')).toHaveCount(0);
		const mod = await page.evaluate(() =>
			/Mac|iPhone|iPad/.test(navigator.userAgent) ? "Cmd" : "Ctrl",
		);
		await search.fill(`${mod}+Z`);
		await expect(dialog.locator('[data-shortcut-row="undo"]')).toBeVisible();
		await search.fill("no such shortcut anywhere");
		await expect(dialog.getByText("No shortcut matches.")).toBeVisible();
		await expect(rows).toHaveCount(0);
		await search.fill("");
		await expect(rows).toHaveCount(all);

		// Every row that is not fixed can be remapped (the fixed ones are the
		// wheel, the grid keys and Esc).
		await expect(dialog.locator("[data-shortcut-key]")).toHaveCount(all - 4);

		// A transport row (the player scope) cannot take a key the timeline uses,
		// as one press would run both; a free key is taken.
		const play = dialog.locator('[data-shortcut-key="play_pause"]');
		await play.click();
		await play.press("b");
		await expect(dialog.getByRole("alert")).toContainText("Blade tool");
		await expect(play).toContainText("Space");
		await play.click();
		await play.press("y");
		await expect(play).toContainText("Y");

		// A global key refuses a timeline row, naming the action in conflict;
		// Tab and the keys of the browser are reserved.
		const text = dialog.locator('[data-shortcut-key="add_text"]');
		await text.click();
		await text.press("Control+e");
		await expect(dialog.getByRole("alert")).toContainText("Export");
		await expect(text).toContainText("T");
		await text.click();
		await text.press("Tab");
		await expect(dialog.getByRole("alert")).toContainText("reserved");
	});

	test("the classic preset moves the keys, is kept across a reload and Ctrl+B stops splitting", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const batches: string[][] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				const body = r.postDataJSON() as { operations: { op: string }[] };
				batches.push(body.operations.map((o) => o.op));
			}
		});
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(1);

		await page.locator("body").press("?");
		const dialog = page.getByRole("dialog", { name: "Keyboard shortcuts" });
		await dialog.getByLabel("Preset").selectOption("classic");
		await expect(dialog.locator('[data-shortcut-key="split"]')).toContainText(
			"S",
		);
		await expect(
			dialog.locator('[data-shortcut-key="tool_blade"]'),
		).toContainText("C");
		await page.keyboard.press("Escape");
		await expect(dialog).toBeHidden();

		await page.getByRole("slider", { name: "Playhead" }).focus();
		await page.keyboard.press("PageUp");
		await items.first().focus();
		await page.keyboard.press("Enter");

		await page.keyboard.press("Control+b");
		await expect(items).toHaveCount(1);
		expect(batches).toHaveLength(0);
		await page.keyboard.press("s");
		await expect(items).toHaveCount(2);

		// The preset is kept, and the short video one comes back from the dialog.
		await page.reload();
		await page.locator("body").press("?");
		await expect(dialog.getByLabel("Preset")).toHaveValue("classic");
		await dialog.getByLabel("Preset").selectOption("short");
		await expect(dialog.locator('[data-shortcut-key="split"]')).toContainText(
			/(Ctrl|Cmd)\+B/,
		);
	});

	test("the slip chords work with the focus on the page and not on an item (VV-215)", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const ops: string[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				const body = r.postDataJSON() as { operations: { op: string }[] };
				ops.push(...body.operations.map((o) => o.op));
			}
		});
		await page.goto(`/editor/${seeded.id}`);
		const item = page.locator("[data-item-id]").first();
		await expect(item).toBeVisible();
		await item.click();
		// Take the focus off the item: nothing in a field and nothing on the grid.
		await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());
		await page.keyboard.press("Alt+.");
		await expect.poll(() => ops).toContain("item.slip");
	});
});

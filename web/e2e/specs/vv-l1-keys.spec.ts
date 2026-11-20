import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

function recordEdits(page: Page): string[][] {
	const edits: string[][] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			edits.push(
				(r.postDataJSON() as { operations: { op: string }[] }).operations.map(
					(o) => o.op,
				),
			);
		}
	});
	return edits;
}

// VV-089 to VV-091 (round L1): Escape closes the command palette from the field, from the form and
// from the page, the documented keys act, and the item menu copies and pastes the look.
test.describe("palette Escape, documented keys and the attributes menu", () => {
	// Parallel imports of the same clip race in the ingest, so these run one after the other.
	test.describe.configure({ mode: "serial" });

	test("Escape closes the palette and leaves the controls reachable", async ({
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
		const search = page.getByRole("combobox", { name: "Search commands" });
		await expect(
			page.getByRole("button", { name: /^Commands \(Ctrl\+K\)/ }),
		).toBeEnabled();
		const back = page.getByRole("button", { name: "Back to the list" });

		await page.keyboard.press("Control+k");
		await expect(search).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(search).toBeHidden();

		await page.keyboard.press("Control+k");
		await search.fill("marker");
		await page.keyboard.press("Enter");
		await expect(back).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(back).toBeHidden();
		await expect(search).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(search).toBeHidden();

		await page.getByRole("button", { name: /^Commands \(Ctrl\+K\)/ }).click();
		await expect(search).toBeVisible();
		await page.mouse.click(5, 5);
		await page.keyboard.press("Escape");
		await expect(search).toBeHidden();
		await page
			.getByRole("button", { name: /^Export/ })
			.click({ timeout: 3000 });
	});

	test("Q, Ctrl+C then Ctrl+V, Shift+B and Ctrl+F act on the selected clip", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "8s",
		});
		const edits = recordEdits(page);
		await page.goto(`/editor/${seeded.id}`);
		const item = page.locator("[data-item-id]").first();
		await expect(item).toBeVisible();
		await item.click();
		const box = await item.boundingBox();
		if (box === null) throw new Error("no item box");
		await page.mouse.click(box.x + box.width * 0.4, box.y - 25);

		await page.keyboard.press("q");
		await expect.poll(() => edits.flat()).toContain("item.trim");
		await page.keyboard.press("Control+c");
		await page.keyboard.press("Control+v");
		await expect.poll(() => edits.flat()).toContain("item.duplicate");
		// Mod+F is taken outside a field only (SDD-012 section 18.8), so it goes before Shift+B moves
		// the focus into the speed field.
		await page.keyboard.press("Control+f");
		await expect(
			page.getByRole("search", { name: "Find in the project" }),
		).toBeVisible();
		await page.keyboard.press("Escape");
		await page.keyboard.press("Shift+B");
		await expect(
			page
				.locator(
					"[data-section='speed'] [data-speed-curve] input:not([disabled])",
				)
				.first(),
		).toBeFocused();
	});

	test("Copy attributes then Paste attributes in the item menu sends item.paste_attributes", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 2,
			perTrack: 1,
			clipDuration: "4s",
		});
		const edits = recordEdits(page);
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		const first = page.locator("[data-item-id]").nth(0);
		const second = page.locator("[data-item-id]").nth(1);
		await first.click({ button: "right" });
		await page.getByRole("menuitem", { name: "Copy attributes" }).click();
		await second.click({ button: "right" });
		await page.getByRole("menuitem", { name: "Paste attributes" }).click();
		await expect.poll(() => edits.flat()).toContain("item.paste_attributes");
	});

	// VV-127: the current time is a field; a typed time moves the playhead.
	test("typing a time in the current time field seeks the playhead", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "8s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const field = page.getByRole("textbox", { name: "Current time" });
		await field.fill("5");
		await field.press("Enter");
		const playhead = page.getByRole("slider", { name: "Playhead" });
		await expect(playhead).toHaveAttribute("aria-valuenow", "150");
		await field.fill("nonsense");
		await field.press("Enter");
		await expect(field).toHaveAttribute("aria-invalid", "true");
		await expect(playhead).toHaveAttribute("aria-valuenow", "150");
	});

	// VV-124: the pages link an icon, so no request ends in a 404.
	test("the projects page has no failing request", async ({
		page,
		request,
	}) => {
		const failed: string[] = [];
		page.on("response", (r) => {
			if (r.status() >= 400) failed.push(`${r.status()} ${r.url()}`);
		});
		await page.goto("/projects");
		await page.waitForLoadState("networkidle");
		expect((await request.get("/icon.png")).status()).toBe(200);
		expect(failed).toEqual([]);
	});

	// VV-117: Inspect shows the content hash, which the brand kit logo takes.
	test("Inspect on an asset card shows its hash", async ({ page, request }) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		// The menu of the asset card, in the library: the tracks have an
		// "Actions for track" button too, which comes first while the library is
		// still listing its files.
		await page
			.getByRole("region", { name: "Media" })
			.getByRole("button", { name: /^Actions for/ })
			.first()
			.click();
		await page.getByRole("menuitem", { name: "Inspect" }).click();
		await expect(page.getByTestId("asset-hash")).toHaveText(
			/^sha256:[0-9a-f]{64}$/,
		);
	});
});

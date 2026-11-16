import type { Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";

// VV-052: Ctrl/Cmd+F opens an in-editor find bar over the names, the text and
// the caption words of the open project (SDD-012 section 18.8). It sends no
// edit: it selects an item and moves the playhead.
async function seed(
	request: Parameters<typeof seedProject>[0],
): Promise<{ id: string; clip: string; text: string; caption: string }> {
	const seeded = await seedProject(request, {
		tracks: 1,
		perTrack: 1,
		clipDuration: "1s",
	});
	const call = await caller(request);
	const head = (await call("GET", `/projects/${seeded.id}`)) as {
		revision: number;
	};
	const clip = seeded.itemIds[0] as string;
	const out = (await call("POST", `/projects/${seeded.id}/edit`, {
		base_revision: head.revision,
		operations: [
			{ op: "item.rename", args: { item_id: clip, name: "Beach intro" } },
			{
				op: "text.add",
				args: {
					track_id: seeded.trackIds[0],
					start: "2s",
					duration: "2s",
					text: "Sunset title",
				},
			},
			{
				op: "caption.add",
				args: {
					track_id: seeded.trackIds[0],
					start: "5s",
					duration: "4s",
					text: "alpha bright morning",
				},
			},
		],
	})) as { created: string[] };
	// `created` is not in the order of the operations: pick the ids by type.
	const doc = (await call("GET", `/projects/${seeded.id}?detail=document`)) as {
		document: { items: Record<string, { type: string }> };
	};
	const byType = (type: string): string =>
		out.created.find((id) => doc.document.items[id]?.type === type) as string;
	return {
		id: seeded.id,
		clip,
		text: byType("text"),
		caption: byType("caption"),
	};
}

async function startFrame(page: Page, itemId: string): Promise<number> {
	const cell = page.locator(`[data-item-id="${itemId}"]`);
	return Number(
		await cell.evaluate((el) =>
			(el as HTMLElement).style.getPropertyValue("--f"),
		),
	);
}

test.describe("find bar", () => {
	test("Ctrl+F finds a name, a text and a caption word; Enter and Shift+Enter walk the matches; Esc returns focus", async ({
		page,
		request,
	}) => {
		const seeded = await seed(request);
		const posted: string[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				posted.push(r.url());
			}
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(3);
		const playhead = page.getByRole("slider", { name: "Playhead" });
		await playhead.focus();

		await page.keyboard.press("Control+f");
		const bar = page.getByRole("search", { name: "Find in the project" });
		await expect(bar).toBeVisible();
		const box = bar.getByRole("searchbox");
		await expect(box).toBeFocused();

		await box.fill("sunset");
		await expect(bar.getByRole("status")).toContainText("1 matches");
		await page.keyboard.press("Enter");
		const textCell = page.locator(`[data-item-id="${seeded.text}"]`);
		await expect(textCell).toHaveAttribute("aria-selected", "true");
		await expect(playhead).toHaveAttribute(
			"aria-valuenow",
			String(await startFrame(page, seeded.text)),
		);

		await box.fill("BEACH");
		await page.keyboard.press("Enter");
		await expect(
			page.locator(`[data-item-id="${seeded.clip}"]`),
		).toHaveAttribute("aria-selected", "true");
		await expect(playhead).toHaveAttribute(
			"aria-valuenow",
			String(await startFrame(page, seeded.clip)),
		);

		await box.fill("morn");
		await page.keyboard.press("Enter");
		const captionCell = page.locator(`[data-item-id="${seeded.caption}"]`);
		await expect(captionCell).toHaveAttribute("aria-selected", "true");
		const captionStart = await startFrame(page, seeded.caption);
		expect(
			Number(await playhead.getAttribute("aria-valuenow")),
		).toBeGreaterThan(captionStart);

		// Two matches wrap in both directions.
		await box.fill("e");
		const count = await bar.getByRole("listitem").count();
		expect(count).toBeGreaterThan(1);
		await page.keyboard.press("Enter");
		await expect(bar.getByRole("status")).toHaveText(`1 of ${count}`);
		await page.keyboard.press("Shift+Enter");
		await expect(bar.getByRole("status")).toHaveText(`${count} of ${count}`);
		await page.keyboard.press("Enter");
		await expect(bar.getByRole("status")).toHaveText(`1 of ${count}`);

		await box.fill("zzzz");
		await expect(bar.getByRole("status")).toHaveText("No match");

		await page.keyboard.press("Escape");
		await expect(bar).toHaveCount(0);
		await expect(playhead).toBeFocused();
		expect(posted).toEqual([]);
	});

	test("a key typed in the bar never edits the timeline", async ({
		page,
		request,
	}) => {
		const seeded = await seed(request);
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(3);
		await page.locator(`[data-item-id="${seeded.clip}"]`).click();
		await page.keyboard.press("Control+f");
		const box = page.getByRole("searchbox");
		await box.fill("x");
		await page.keyboard.press("Delete");
		await page.keyboard.press("Backspace");
		await expect(page.locator("[data-item-id]")).toHaveCount(3);
	});
});

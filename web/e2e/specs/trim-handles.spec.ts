import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

async function handle(page: Page, edge: "start" | "end") {
	const box = await page
		.locator(`[data-item-id] [data-trim-handle="${edge}"]`)
		.first()
		.boundingBox();
	if (box === null) throw new Error("handle is not laid out");
	return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

async function stored(
	page: Page,
): Promise<{ start: number; duration: number }> {
	return page.evaluate(() => {
		const doc = window.__vv?.document as {
			items: Record<string, { start: number; duration: number }>;
		};
		const item = Object.values(doc.items)[0];
		return { start: item?.start ?? -1, duration: item?.duration ?? -1 };
	});
}

// P2-UI-011 (SDD-012 section 8.4, Trim): a handle drag sends one item.trim; the
// ghost is clamped to the limits of the core; a trim the core refuses puts the
// item back and says what the source has.
test.describe("trim-handles", () => {
	test("the end handle sends one item.trim and the item takes the answer", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const before = await stored(page);
		const posted: { operations: { op: string; args: { edge: string } }[] }[] =
			[];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				posted.push(r.postDataJSON());
			}
		});

		const from = await handle(page, "end");
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x - 10, from.y, { steps: 6 });
		await expect(page.getByTestId("ghost-layer")).toBeVisible();
		await page.mouse.up();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toHaveLength(1);
		expect(posted[0]?.operations[0]).toMatchObject({
			op: "item.trim",
			args: { edge: "end" },
		});
		await expect
			.poll(async () => (await stored(page)).duration)
			.toBeLessThan(before.duration);
	});

	test("the ghost stops at the limit the core supplied", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const from = await handle(page, "end");
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 4000, from.y, { steps: 6 });
		const ghost = page.locator("[data-ghost-id]");
		await expect(ghost).toBeVisible();
		const right = (await ghost.boundingBox())?.width ?? 0;
		await page.mouse.move(from.x + 6000, from.y, { steps: 3 });
		expect((await ghost.boundingBox())?.width).toBe(right);
		await page.keyboard.press("Escape");
		await page.mouse.up();
	});

	test("a trim past the end of the source shows what the source has and leaves the item unchanged", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		// Without limits the UI sends what the pointer asked for and the core judges.
		await page.route("**/snap-points*", async (route) => {
			const response = await route.fetch();
			const body = await response.json();
			for (const limit of Object.values(body.limits ?? {}) as {
				trim_end_max: number;
			}[]) {
				limit.trim_end_max = 1_000_000;
			}
			await route.fulfill({ response, json: body });
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const before = await stored(page);
		const posted: unknown[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				posted.push(r.postDataJSON());
			}
		});

		const from = await handle(page, "end");
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 3000, from.y, { steps: 8 });
		await page.mouse.up();

		await expect(page.getByRole("alert")).toContainText("The source has only");
		await expect(page.getByTestId("ghost-layer")).toHaveCount(0);
		expect(posted).toHaveLength(1);
		expect(await stored(page)).toEqual(before);
	});
});

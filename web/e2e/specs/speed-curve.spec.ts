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

// TASK-0280 (SDD-012 section 12.1, SDD-002 C.25): the Speed section has the
// Reverse toggle and a curve of 2 to 8 points, each one command.
test.describe("speed-curve", () => {
	test("Reverse, a curve and Remove curve each send one clip.set_speed", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] ?? "";
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator("[data-item-id]").click();
		await expect(page.getByRole("heading", { name: /^Clip/ })).toBeVisible();
		const edits = recordEdits(page);

		await page.getByLabel("Reverse", { exact: true }).check();
		await expect.poll(() => edits.length).toBe(1);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await expect
			.poll(async () =>
				page.evaluate(
					(id) =>
						(
							window.__vv?.document as {
								items: Record<string, { reverse?: boolean }>;
							}
						).items[id]?.reverse,
					itemId,
				),
			)
			.toBe(true);

		await page.getByRole("button", { name: "Add a speed curve" }).click();
		await expect(page.locator("[data-curve-point]")).toHaveCount(2);
		await page.getByRole("button", { name: "Add point" }).click();
		await expect(page.locator("[data-curve-point]")).toHaveCount(3);
		const last = page.getByLabel("Point 3 speed", { exact: true });
		await last.fill("2");
		await last.press("Enter");
		// Editing the draft sends nothing: only Apply does.
		expect(edits).toHaveLength(1);
		await page.getByRole("button", { name: "Apply curve" }).click();
		await expect.poll(() => edits.length).toBe(2);

		expect(edits[0]?.operations).toEqual([
			{ op: "clip.set_speed", args: { item_id: itemId, reverse: true } },
		]);
		expect(edits[1]?.operations).toEqual([
			{
				op: "clip.set_speed",
				args: {
					item_id: itemId,
					curve: [
						{ t: 0, speed: "1x" },
						{ t: 0.5, speed: "1x" },
						{ t: 1, speed: "2x" },
					],
				},
			},
		]);
		// The core stored it: the mean of the ramp and the point list come from the
		// read model, not from the draft.
		await expect(page.getByText("Mean speed 1.25x")).toBeVisible();
		await expect(page.locator("[data-speed-chart]")).toBeVisible();

		await page.getByRole("button", { name: "Remove curve" }).click();
		await expect.poll(() => edits.length).toBe(3);
		const removal = edits[2]?.operations[0];
		expect(removal?.op).toBe("clip.set_speed");
		expect(removal?.args.speed).toMatchObject({
			num: expect.any(Number),
			den: expect.any(Number),
		});
		await expect(page.locator("[data-speed-curve]")).toHaveCount(0);
		await expect(
			page.getByRole("button", { name: "Add a speed curve" }),
		).toBeVisible();
	});

	test("a curve the core refuses shows its message under the buttons", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator("[data-item-id]").click();
		await page.getByRole("button", { name: "Add a speed curve" }).click();
		// The last point is at 100%: a middle point past it is out of order.
		await page.getByRole("button", { name: "Add point" }).click();
		const middle = page.getByLabel("Point 2 time", { exact: true });
		await middle.fill("100");
		await middle.press("Enter");
		await page.getByRole("button", { name: "Apply curve" }).click();
		await expect(page.locator("[data-speed-curve] [role=alert]")).toBeVisible();
	});
});

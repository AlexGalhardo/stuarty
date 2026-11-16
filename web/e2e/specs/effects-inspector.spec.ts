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

// TASK-0280 (SDD-012 section 12.1): the Effects section sends the whole list
// as one clip.set_effects at each change, and the Transitions section has the
// colour of a dip and the direction of a whip or a wipe.
test.describe("effects-inspector", () => {
	test("add, edit and remove effects, each change one clip.set_effects", async ({
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

		await page
			.getByLabel("Add effect", { exact: true })
			.selectOption("vignette");
		await expect.poll(() => edits.length).toBe(1);
		await expect(page.locator("[data-effect=vignette]")).toBeVisible();
		const amount = page.getByLabel("Vignette amount", { exact: true });
		await expect(amount).toHaveValue("0.5");
		await amount.fill("0.8");
		await amount.press("Enter");
		await expect.poll(() => edits.length).toBe(2);
		await page.getByLabel("Add effect", { exact: true }).selectOption("glow");
		await expect.poll(() => edits.length).toBe(3);
		await page.getByRole("button", { name: "Remove Vignette" }).click();
		await expect.poll(() => edits.length).toBe(4);

		expect(edits.map((e) => e.operations)).toEqual([
			[
				{
					op: "clip.set_effects",
					args: {
						item_id: itemId,
						effects: [{ kind: "vignette", amount: 0.5, softness: 0.5 }],
					},
				},
			],
			[
				{
					op: "clip.set_effects",
					args: {
						item_id: itemId,
						effects: [{ kind: "vignette", amount: 0.8, softness: 0.5 }],
					},
				},
			],
			[
				{
					op: "clip.set_effects",
					args: {
						item_id: itemId,
						effects: [
							{ kind: "vignette", amount: 0.8, softness: 0.5 },
							{ kind: "glow", amount: 0.5, radius: 12, threshold: 0.6 },
						],
					},
				},
			],
			[
				{
					op: "clip.set_effects",
					args: {
						item_id: itemId,
						effects: [
							{ kind: "glow", amount: 0.5, radius: 12, threshold: 0.6 },
						],
					},
				},
			],
		]);
		await expect(page.locator("[data-effect=vignette]")).toHaveCount(0);
		await expect(page.locator("[data-effect=glow]")).toBeVisible();
	});

	test("a value out of range shows its range under the field and sends nothing", async ({
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
		const edits = recordEdits(page);
		await page.getByLabel("Add effect", { exact: true }).selectOption("grain");
		await expect.poll(() => edits.length).toBe(1);
		const seed = page.getByLabel("Grain seed", { exact: true });
		await expect(seed).toHaveValue("0");
		await seed.fill("70000");
		await seed.press("Enter");
		await expect(
			page.locator("[data-effect=grain] [role=alert]"),
		).toBeVisible();
		expect(edits).toHaveLength(1);
	});

	test("a dip has its colour and a wipe its direction", async ({
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
		const inEdge = page.locator("[data-transition-edge=in]");

		await expect(inEdge.getByLabel("In colour")).toHaveCount(0);
		await inEdge
			.getByLabel("In transition", { exact: true })
			.selectOption("dip");
		await expect.poll(() => edits.length).toBe(1);
		const colour = inEdge.getByLabel("In colour").first();
		await expect(colour).toHaveValue("#000000");
		await colour.fill("#336699");
		await colour.press("Enter");
		await expect.poll(() => edits.length).toBe(2);

		await inEdge
			.getByLabel("In transition", { exact: true })
			.selectOption("wipe");
		await expect.poll(() => edits.length).toBe(3);
		await expect(inEdge.getByLabel("In colour")).toHaveCount(0);
		await inEdge.getByLabel("In direction", { exact: true }).selectOption("up");
		await expect.poll(() => edits.length).toBe(4);

		expect(edits[0]?.operations[0]?.args).toMatchObject({
			item_id: itemId,
			edge: "in",
			kind: "dip",
		});
		expect(edits[0]?.operations[0]?.args).not.toHaveProperty("color");
		expect(edits[1]?.operations[0]?.args).toMatchObject({
			kind: "dip",
			color: "#336699",
		});
		// A change of kind starts from the defaults: no colour goes with a wipe.
		expect(edits[2]?.operations[0]?.args).not.toHaveProperty("color");
		expect(edits[3]?.operations[0]?.args).toMatchObject({
			kind: "wipe",
			direction: "up",
		});
		expect(edits[3]?.operations[0]?.args).not.toHaveProperty("color");
	});
});

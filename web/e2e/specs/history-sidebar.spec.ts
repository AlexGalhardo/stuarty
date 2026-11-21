import { caller, expect, seedProject, test } from "./helpers";

// Epic 60 (D-125, SDD-012 sections 6.1 and 15): the history is a sidebar right
// next to the library, collapsible by its button and by Alt+3, newest step on
// top with its actor and what changed, and open or collapsed as the person left
// it.
test.describe("history sidebar", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test("opens, collapses, lists the newest step first and is remembered", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const agent = await caller(request, "claude-code");
		const rename = async (name: string): Promise<void> => {
			const head = (await agent("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			await agent("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations: [
					{
						op: "item.rename",
						args: { item_id: seeded.itemIds[0], name },
					},
				],
			});
		};
		await rename("First");
		await rename("Second");

		await page.goto(`/editor/${seeded.id}`);
		const sidebar = page.getByRole("complementary", {
			name: "History sidebar",
		});
		const list = sidebar.getByRole("region", { name: "History" });

		// Collapsed at first: a rail with the button, no list.
		await expect(sidebar).toBeVisible();
		await expect(list).toHaveCount(0);
		expect((await sidebar.boundingBox())?.width).toBeCloseTo(40, 0);

		// The button opens it, right beside the library.
		await sidebar.getByRole("button", { name: "Show history" }).click();
		await expect(list).toBeVisible();
		await expect(
			sidebar.getByRole("button", { name: "Hide history" }),
		).toHaveAttribute("aria-expanded", "true");
		const library = await page
			.locator("aside[data-landmark='2']")
			.boundingBox();
		const opened = await sidebar.boundingBox();
		expect(opened?.x).toBeCloseTo((library?.x ?? 0) + (library?.width ?? 0), 0);
		expect(opened?.width).toBeCloseTo(320, 0);

		// Newest first, each step with its actor and what it did.
		await expect(list.locator("[data-revision]").first()).toBeVisible();
		const revisions = await list
			.locator("[data-revision]")
			.evaluateAll((rows) =>
				rows.map((row) => Number(row.getAttribute("data-revision"))),
			);
		expect(revisions.length).toBeGreaterThanOrEqual(3);
		expect(revisions).toEqual([...revisions].sort((a, b) => b - a));
		const newest = list.locator("[data-revision]").first();
		await expect(newest).toHaveAttribute("data-actor", "local:claude-code");
		await expect(newest).toContainText("claude-code");
		await expect(newest).toContainText(/Second|item\.rename|rename/i);

		// A step made while it is open comes on top.
		await rename("Third");
		await expect
			.poll(async () =>
				Number(
					await list
						.locator("[data-revision]")
						.first()
						.getAttribute("data-revision"),
				),
			)
			.toBeGreaterThan(revisions[0] ?? 0);

		// Alt+3 collapses it, and the choice survives a reload.
		await page.keyboard.press("Alt+3");
		await expect(list).toHaveCount(0);
		expect((await sidebar.boundingBox())?.width).toBeCloseTo(40, 0);
		await page.reload();
		await expect(sidebar).toBeVisible();
		await expect(list).toHaveCount(0);

		await sidebar.getByRole("button", { name: "Show history" }).click();
		await page.reload();
		await expect(list).toBeVisible();
	});
});

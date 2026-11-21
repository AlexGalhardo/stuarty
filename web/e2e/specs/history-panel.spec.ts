import { caller, expect, seedProject, test } from "./helpers";

// P2-UI-006 (SDD-012 section 15): the History sidebar lists the steps newest first
// with the actor of each, prepends a new step from its event before the page is
// refetched, and undo reverts only the step of the actor that presses it.
test.describe("history-panel", () => {
	test("steps of the UI and of an agent show their actors, a new step is prepended, undo reverts the UI step only", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);

		// A step of the UI: rename the clip in the inspector.
		await page.locator("[data-item-id]").click();
		const name = page.getByLabel("Name", { exact: true });
		await name.fill("From the UI");
		await name.press("Enter");
		await expect(
			page.getByRole("heading", { name: /From the UI/ }),
		).toBeVisible();

		// A step of an agent over HTTP.
		const agent = await caller(request, "claude-code");
		const head = (await agent("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await agent("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "item.rename",
					args: { item_id: seeded.itemIds[0], name: "From the agent" },
				},
			],
		});
		await expect(
			page.getByRole("heading", { name: /From the agent/ }),
		).toBeVisible();

		await page.keyboard.press("Alt+3");
		const list = page.getByRole("region", { name: "History" });
		await expect(list.locator("[data-actor]").first()).toHaveAttribute(
			"data-actor",
			"local:claude-code",
		);
		await expect(list.locator('[data-actor="local:ui"]').first()).toContainText(
			"You",
		);
		await expect(
			list.locator('[data-actor="local:claude-code"]').first(),
		).toContainText("claude-code");

		// A new step is prepended from its event while the refetch is held.
		let release: () => void = () => {};
		const held = new Promise<void>((resolve) => {
			release = resolve;
		});
		await page.route("**/history*", async (route) => {
			await held;
			await route.continue();
		});
		const rows = await list.locator("[data-revision]").count();
		const next = (await agent("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await agent("POST", `/projects/${seeded.id}/edit`, {
			base_revision: next.revision,
			operations: [
				{
					op: "item.rename",
					args: { item_id: seeded.itemIds[0], name: "Again" },
				},
			],
		});
		await expect(list.locator("[data-revision]")).toHaveCount(rows + 1);
		release();

		// Undo reverts the UI step; the agent steps stay.
		await page.getByRole("button", { name: "Undo" }).click();
		await expect(list.locator('[data-undone="true"]')).toHaveCount(1);
		const doc = (await agent(
			"GET",
			`/projects/${seeded.id}?detail=document`,
		)) as { document: { items: Record<string, { name?: string }> } };
		expect(Object.values(doc.document.items)[0]?.name).toBe("Again");

		// Alt+3 collapses the sidebar and leaves the library as it is; Alt+1 hides the library.
		await expect(page.getByRole("tab", { name: "Media" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
		await page.keyboard.press("Alt+3");
		await expect(list).toHaveCount(0);
		await page.keyboard.press("Alt+1");
		// The panel is hidden and the rail stays (docs/design-system.md 3.4.4).
		await expect(page.getByRole("tab", { name: "Media" })).toHaveAttribute(
			"aria-selected",
			"false",
		);
		await expect(page.getByRole("region", { name: "Media" })).toHaveCount(0);
	});
});

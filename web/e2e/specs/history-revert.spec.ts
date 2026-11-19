import { caller, expect, seedProject, test } from "./helpers";

// Epic 36 (RF-120, RF-121): a person names the state of the project, lets an agent
// work, reads in words what changed between two steps, and goes back in one step
// with the dry run as the confirmation. The revert is a new step, so Undo takes it
// back. Everything is reachable from the keyboard.
test.describe("history-revert", () => {
	test("name a state, compare two steps, revert to it by name, undo the revert", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		const agent = await caller(request, "claude-code");

		const rename = async (name: string): Promise<void> => {
			const head = (await agent("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			await agent("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations: [{ op: "item.rename", args: { item_id: itemId, name } }],
			});
		};
		const itemName = async (): Promise<string | undefined> => {
			const doc = (await agent(
				"GET",
				`/projects/${seeded.id}?detail=document`,
			)) as { document: { items: Record<string, { name?: string }> } };
			return doc.document.items[itemId]?.name;
		};

		await rename("Intro");
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.keyboard.press("Alt+3");
		const history = page.getByRole("region", { name: "History", exact: true });
		await expect(history.locator("[data-revision]").first()).toBeVisible();

		// Name the current state; the field and the button are plain controls.
		await history.getByLabel("Name this state").fill("before the agent");
		await history.getByLabel("Name this state").press("Enter");
		const checkpoints = history.getByRole("region", { name: "Checkpoints" });
		await expect(
			checkpoints.locator('[data-checkpoint="before the agent"]'),
		).toBeVisible();

		// The agent works: its first edit of the day made an automatic checkpoint too.
		await rename("Agent cut");
		await rename("Agent cut 2");
		await expect(
			history.locator('[data-actor="local:claude-code"]').first(),
		).toBeVisible();
		await expect.poll(itemName).toBe("Agent cut 2");
		await expect(checkpoints.getByText("Automatic").first()).toBeVisible();

		// Tick the newest step and the one before the agent: the diff reads in words.
		const rows = history.locator("[data-revision]");
		const newest = Number(await rows.first().getAttribute("data-revision"));
		await history.getByLabel(`Compare r${newest}`).click();
		await history.getByLabel(`Compare r${newest - 2}`).click();
		const diff = history.getByTestId("history-diff");
		await expect(diff).toBeVisible();
		await expect(diff).toContainText("name");
		await expect(diff).toContainText("Agent cut 2");

		// Revert to the checkpoint from the keyboard: Escape leaves everything as it was.
		const goBack = checkpoints.getByRole("button", {
			name: "Revert to before the agent",
		});
		await goBack.focus();
		await page.keyboard.press("Enter");
		const dialog = page.getByRole("alertdialog");
		await expect(dialog).toContainText("Go back to before the agent?");
		await expect(dialog.getByTestId("revert-preview")).toContainText("name");
		await page.keyboard.press("Escape");
		await expect(dialog).toBeHidden();
		expect(await itemName()).toBe("Agent cut 2");

		// Open it again and confirm: the project goes back in one new step.
		await goBack.click();
		await expect(dialog.getByTestId("revert-preview")).toBeVisible();
		await dialog.getByRole("button", { name: "Revert", exact: true }).click();
		await expect(dialog).toBeHidden();
		await expect.poll(itemName).toBe("Intro");
		await expect(rows.first()).toContainText("Reverted to");

		// The revert is a step of the person: Undo takes it back.
		await page.getByRole("button", { name: "Undo" }).click();
		await expect.poll(itemName).toBe("Agent cut 2");
	});

	test("a step can be reverted to from its row, and the newest has no revert", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		const agent = await caller(request, "claude-code");
		const head = (await agent("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await agent("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{ op: "item.rename", args: { item_id: itemId, name: "Changed" } },
			],
		});

		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.keyboard.press("Alt+3");
		const history = page.getByRole("region", { name: "History", exact: true });
		const rows = history.locator("[data-revision]");
		await expect(rows.nth(1)).toBeVisible();
		const newest = Number(await rows.first().getAttribute("data-revision"));

		await expect(
			history.getByRole("button", { name: `Revert to r${newest}` }),
		).toHaveCount(0);
		await history
			.getByRole("button", { name: `Revert to r${newest - 1}` })
			.click();
		const dialog = page.getByRole("alertdialog");
		await expect(dialog.getByTestId("revert-preview")).toContainText("name");
		await dialog.getByRole("button", { name: "Revert", exact: true }).click();
		await expect(dialog).toBeHidden();

		const doc = (await agent(
			"GET",
			`/projects/${seeded.id}?detail=document`,
		)) as { document: { items: Record<string, { name?: string }> } };
		expect(doc.document.items[itemId]?.name).not.toBe("Changed");
	});

	test("a change by an agent after the preview is a conflict, never reverted unseen", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		const agent = await caller(request, "claude-code");
		const rename = async (name: string): Promise<void> => {
			const head = (await agent("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			await agent("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations: [{ op: "item.rename", args: { item_id: itemId, name } }],
			});
		};
		const itemName = async (): Promise<string | undefined> => {
			const doc = (await agent(
				"GET",
				`/projects/${seeded.id}?detail=document`,
			)) as { document: { items: Record<string, { name?: string }> } };
			return doc.document.items[itemId]?.name;
		};
		await rename("Changed");

		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.keyboard.press("Alt+3");
		const history = page.getByRole("region", { name: "History", exact: true });
		const rows = history.locator("[data-revision]");
		await expect(rows.nth(1)).toBeVisible();
		const newest = Number(await rows.first().getAttribute("data-revision"));

		await history
			.getByRole("button", { name: `Revert to r${newest - 1}` })
			.click();
		const dialog = page.getByRole("alertdialog");
		await expect(dialog.getByTestId("revert-preview")).toContainText("name");

		// The agent renames the item again while the person reads the preview.
		await rename("Agent late");
		await dialog.getByRole("button", { name: "Revert", exact: true }).click();
		await expect(dialog.getByRole("alert")).toBeVisible();
		await expect(dialog).toBeVisible();
		expect(await itemName()).toBe("Agent late");
	});
});

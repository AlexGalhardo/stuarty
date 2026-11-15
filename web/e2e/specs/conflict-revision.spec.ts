import type { Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";

async function center(page: Page): Promise<{ x: number; y: number }> {
	const box = await page.locator("[data-item-id]").first().boundingBox();
	if (box === null) throw new Error("item is not laid out");
	return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

async function agentRename(
	request: Parameters<typeof caller>[0],
	projectId: string,
	itemId: string,
	name: string,
): Promise<void> {
	const agent = await caller(request, "claude-code");
	const head = (await agent("GET", `/projects/${projectId}`)) as {
		revision: number;
	};
	await agent("POST", `/projects/${projectId}/edit`, {
		base_revision: head.revision,
		operations: [{ op: "item.rename", args: { item_id: itemId, name } }],
	});
}

// P2-UI-012 (SDD-012 sections 4.5 and 5.8, SDD-006 section 7.3): a gesture is
// judged against the revision of the press; nothing is ever resent on its own.
test.describe("conflict-revision", () => {
	test("an agent edit between press and release: one conflicted request, the notice names the actor, Apply again applies", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0];
		if (itemId === undefined) throw new Error("no item");
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const posted: {
			base_revision: number;
			idempotency_key: string;
			operations: unknown[];
		}[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				posted.push(r.postDataJSON());
			}
		});
		const pressRevision = await page.evaluate(() => window.__vv?.revision ?? 0);

		const from = await center(page);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await agentRename(request, seeded.id, itemId, "Changed by the agent");
		await expect(page.getByLabel(/Changed by the agent/)).toBeVisible();
		await page.mouse.move(from.x + 60, from.y, { steps: 6 });
		await page.mouse.up();

		await expect(page.getByRole("alert")).toContainText("claude-code");
		await expect(page.getByRole("alert")).toContainText("Changed by the agent");
		await expect(page.getByRole("alert")).toContainText(
			"Your change was not applied",
		);
		await expect(page.getByTestId("ghost-layer")).toHaveCount(0);
		expect(posted).toHaveLength(1);
		expect(posted[0]?.base_revision).toBe(pressRevision);

		const current = await page.evaluate(() => window.__vv?.revision ?? 0);
		await page.getByRole("button", { name: "Apply again" }).click();
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.base_revision).toBe(current);
		expect(posted[1]?.idempotency_key).not.toBe(posted[0]?.idempotency_key);
		expect(posted[1]?.operations).toEqual(posted[0]?.operations);
		await expect
			.poll(() => page.evaluate(() => window.__vv?.revision ?? 0))
			.toBe(current + 1);
	});

	test("an agent edit of the dragged item cancels the drag, names the actor and sends nothing", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0];
		if (itemId === undefined) throw new Error("no item");
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const posted: string[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				posted.push(r.url());
			}
		});

		const from = await center(page);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 40, from.y, { steps: 4 });
		await expect(page.getByTestId("ghost-layer")).toBeVisible();
		await agentRename(request, seeded.id, itemId, "Dragged and changed");

		await expect(page.getByTestId("ghost-layer")).toHaveCount(0);
		await expect(
			page.getByText(/claude-code changed what you were dragging/),
		).toBeVisible();
		await page.mouse.up();
		expect(posted).toHaveLength(0);
	});
});

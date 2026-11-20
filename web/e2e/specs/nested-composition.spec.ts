import { caller, expect, seedProject, test } from "./helpers";

interface Doc {
	revision?: number;
	items: Record<string, { type: string; timeline?: string }>;
	tracks: Record<string, { kind: string }>;
}

// Epic 32: a timeline used as a clip in another one. Create it from the
// selection, edit the nested timeline, and see the parent change without an
// edit of the parent; open the nested timeline with its way back.
test.describe("nested composition", () => {
	test("create from selection, edit the child, open it and come back", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		const call = await caller(request);

		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);

		const first = page.locator("[data-item-id]").first();
		const moved = await first.getAttribute("data-item-id");
		await first.click();
		await first.click({ button: "right" });
		await page
			.getByRole("menuitem", { name: "Create composition from selection" })
			.click();

		const composition = await page.waitForFunction(() => {
			const doc = window.__vv?.document as Doc | undefined;
			const found = Object.entries(doc?.items ?? {}).find(
				([, item]) => item.type === "composition",
			);
			return found === undefined
				? null
				: { id: found[0], timeline: found[1].timeline };
		});
		const made = (await composition.jsonValue()) as {
			id: string;
			timeline: string;
		};
		await expect(page.locator(`[data-item-id="${made.id}"]`)).toBeVisible();

		const planOf = async (): Promise<{ revision: number; layers: number }> => {
			const plan = (await call(
				"GET",
				`/projects/${seeded.id}/plan?from=0&until=30&timeline_id=${seeded.id}`,
			)) as { revision: number; layers: unknown[] };
			return { revision: plan.revision, layers: plan.layers.length };
		};
		const before = await planOf();

		// An edit of the nested timeline only: a text on a new track of it.
		const child = (await call(
			"GET",
			`/projects/${made.timeline}?detail=document&timeline_id=${made.timeline}`,
		)) as { revision: number };
		const added = (await call("POST", `/projects/${made.timeline}/edit`, {
			timeline_id: made.timeline,
			base_revision: child.revision,
			operations: [
				{ op: "track.add", args: { kind: "visual", name: "Title" } },
			],
		})) as { revision: number; created: string[] };
		const track = added.created[0];
		if (track === undefined) throw new Error("no track was created");
		// The open editor of the parent hears nested.updated and asks for its plan
		// again, which now holds the text of the child (SDD-006 section 3.3.1).
		const replanned = page.waitForResponse(async (response) => {
			if (!response.url().includes(`/projects/${seeded.id}/plan`)) return false;
			const body = (await response.json()) as { layers: { item: string }[] };
			return body.layers.some(
				(layer) =>
					layer.item.startsWith(`${made.id}/`) &&
					layer.item !== `${made.id}/${moved}`,
			);
		});
		await call("POST", `/projects/${made.timeline}/edit`, {
			timeline_id: made.timeline,
			base_revision: added.revision,
			operations: [
				{
					op: "text.add",
					args: { track_id: track, start: "0s", duration: "1s", text: "Hello" },
				},
			],
		});

		const after = await planOf();
		expect(after.revision).toBe(before.revision);
		expect(after.layers).toBe(before.layers + 1);
		await replanned;

		// Double click opens the nested timeline; the breadcrumb comes back.
		await page.locator(`[data-item-id="${made.id}"]`).dblclick();
		await expect(page).toHaveURL(
			(url) =>
				url.pathname === `/editor/${made.timeline}` &&
				url.searchParams.get("from") === seeded.id,
		);
		const back = page.locator('[data-breadcrumb="composition"]');
		await expect(back).toBeVisible();
		await back.click();
		await expect(page).toHaveURL(new RegExp(`/editor/${seeded.id}$`));
	});
});

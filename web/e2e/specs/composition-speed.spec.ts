import { caller, expect, seedProject, test } from "./helpers";
import { openInspectorTab } from "./sections";

interface Doc {
	revision: number;
	items: Record<
		string,
		{
			type: string;
			duration: number;
			timeline?: string;
			speed?: { num: number; den: number };
		}
	>;
}

// Epic 53 (SDD-012 section 12.1, SDD-002 C.25): the Speed section of a
// composition item has a constant speed and no Reverse and no curve. Setting 2x
// sends one clip.set_speed; the item gets half as long, and the plan shows the
// nested timeline at twice the rate.
test.describe("composition-speed", () => {
	test("the speed of a composition item is set in the inspector and the plan follows", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const call = await caller(request);
		const head = (await call(
			"GET",
			`/projects/${seeded.id}?detail=document&timeline_id=${seeded.id}`,
		)) as { revision: number };
		await call("POST", `/projects/${seeded.id}/edit`, {
			timeline_id: seeded.id,
			base_revision: head.revision,
			operations: [
				{
					op: "composition.create_from_selection",
					args: { item_ids: seeded.itemIds, name: "Scene" },
				},
			],
		});

		const documentOf = async (): Promise<Doc> =>
			(
				(await call(
					"GET",
					`/projects/${seeded.id}?detail=document&timeline_id=${seeded.id}`,
				)) as { document: Doc }
			).document;
		const before = await documentOf();
		const [compId, composition] =
			Object.entries(before.items).find(([, i]) => i.type === "composition") ??
			[];
		if (compId === undefined || composition === undefined) {
			throw new Error("no composition was made");
		}
		const length = composition.duration;

		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${compId}"]`).click();
		await expect(
			page.getByRole("heading", { name: /^Composition/ }),
		).toBeVisible();

		await openInspectorTab(page, "Speed");
		// A composition has a constant speed only.
		await expect(page.getByLabel("Reverse", { exact: true })).toHaveCount(0);
		await expect(
			page.getByRole("button", { name: "Add a speed curve" }),
		).toHaveCount(0);

		const field = page.getByRole("textbox", { name: "Speed", exact: true });
		await field.fill("2");
		await field.press("Enter");

		await expect
			.poll(async () => (await documentOf()).items[compId]?.speed)
			.toEqual({ num: 2, den: 1 });
		const after = await documentOf();
		expect(after.items[compId]?.duration).toBe(Math.ceil(length / 2));

		// The plan reads the nested timeline at twice the rate: the source time of
		// the layer moves by two frames (at 30 fps) from one frame to the next.
		const plan = (await call(
			"GET",
			`/projects/${seeded.id}/plan?from=10&until=12&timeline_id=${seeded.id}`,
		)) as { layers: { item: string; source_seconds?: number[] }[] };
		const nested = plan.layers.find((l) => l.item.startsWith(`${compId}/`));
		const seconds = nested?.source_seconds ?? [];
		expect(seconds).toHaveLength(2);
		expect((seconds[1] ?? 0) - (seconds[0] ?? 0)).toBeCloseTo(2 / 30, 2);
	});
});

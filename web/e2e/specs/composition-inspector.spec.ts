import { caller, expect, seedProject, test } from "./helpers";
import { openInspectorTab } from "./sections";

interface Doc {
	items: Record<
		string,
		{
			type: string;
			opacity?: number;
			volume?: number;
			audio_muted?: boolean;
			transition_in?: { kind: string };
		}
	>;
}

// VV-288 (SDD-012 section 12.1): the inspector of a composition item has the
// Transform and Transitions sections of a visual item and an Audio tab with the
// volume, the fades and the mute, but not the noise reduction and the voice
// filter, which the core refuses on a composition. Each field sends one command.
test.describe("composition-inspector", () => {
	test("the panel of a composition has transform, transitions and audio", async ({
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
		const compId = Object.entries((await documentOf()).items).find(
			([, i]) => i.type === "composition",
		)?.[0];
		if (compId === undefined) throw new Error("no composition was made");

		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${compId}"]`).click();
		await expect(
			page.getByRole("heading", { name: /^Composition/ }),
		).toBeVisible();

		await openInspectorTab(page, "Video");
		await expect(
			page.getByRole("button", { name: "Transform", exact: true }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Transitions", exact: true }),
		).toBeVisible();
		const opacity = page.getByLabel("Opacity", { exact: true });
		await opacity.fill("0.5");
		await opacity.press("Enter");
		await expect
			.poll(async () => (await documentOf()).items[compId]?.opacity)
			.toBe(0.5);

		await openInspectorTab(page, "Audio");
		await expect(page.getByLabel("Noise reduction")).toHaveCount(0);
		await expect(page.getByLabel("Voice filter")).toHaveCount(0);
		const volume = page.getByLabel("Volume", { exact: true });
		await volume.fill("2");
		await volume.press("Enter");
		await expect
			.poll(async () => (await documentOf()).items[compId]?.volume)
			.toBe(2);
		await page.getByLabel("Mute audio").check();
		await expect
			.poll(async () => (await documentOf()).items[compId]?.audio_muted)
			.toBe(true);
	});
});

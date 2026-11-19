import type { Page } from "@playwright/test";
import { expect, seedProject, silenceClip, test } from "./helpers";
import { openInspectorTab } from "./sections";

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

async function open(page: Page, projectId: string): Promise<void> {
	await page.goto(`/editor/${projectId}?debug=1`);
	await expect(page.locator("[data-item-id]")).toHaveCount(1);
	await page.locator("[data-item-id]").click();
	await expect(page.getByRole("heading", { name: /^Clip/ })).toBeVisible();
}

// P2-UI-025 (SDD-012 sections 12.1 and 12.3): the audio, framing, speed, colour
// and silences sections of a clip. Each commit is one command with only what
// changed.
test.describe("clip-sections", () => {
	test("audio, framing and speed each send one command with the changed field", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] ?? "";
		await open(page, seeded.id);
		const edits = recordEdits(page);

		await openInspectorTab(page, "Audio");
		const volume = page.getByLabel("Volume", { exact: true });
		await volume.fill("2");
		await volume.press("Enter");
		await expect.poll(() => edits.length).toBe(1);
		const fade = page.getByLabel("Fade in", { exact: true });
		await fade.fill("0.5s");
		await fade.press("Enter");
		await expect.poll(() => edits.length).toBe(2);
		await page.getByLabel("Mute audio").check();
		await expect.poll(() => edits.length).toBe(3);

		await openInspectorTab(page, "Video");
		await page.getByLabel("Fit", { exact: true }).selectOption("contain");
		await expect.poll(() => edits.length).toBe(4);
		const left = page.getByLabel("Crop left", { exact: true });
		await left.fill("0.1");
		await left.press("Enter");
		await expect.poll(() => edits.length).toBe(5);

		await openInspectorTab(page, "Speed");
		// The tab panel is named by its tab, so it is left out of the label match.
		const speed = page
			.getByRole("tabpanel")
			.getByLabel("Speed", { exact: true });
		await speed.fill("1.5");
		await speed.press("Enter");
		await expect.poll(() => edits.length).toBe(6);
		await page
			.locator("section", {
				has: page.getByRole("button", { name: "Speed", exact: true }),
			})
			.getByLabel("Ripple later items")
			.check();
		await speed.fill("2");
		await speed.press("Enter");
		await expect.poll(() => edits.length).toBe(7);

		expect(edits.map((e) => e.operations)).toEqual([
			[{ op: "item.set_audio", args: { item_id: itemId, volume: 2 } }],
			[{ op: "item.set_audio", args: { item_id: itemId, fade_in: "0.5s" } }],
			[{ op: "item.set_audio", args: { item_id: itemId, audio_muted: true } }],
			[{ op: "clip.set_framing", args: { item_id: itemId, fit: "contain" } }],
			[
				{
					op: "clip.set_framing",
					args: { item_id: itemId, crop: { left: 0.1 } },
				},
			],
			[{ op: "clip.set_speed", args: { item_id: itemId, speed: "1.5x" } }],
			[
				{
					op: "clip.set_speed",
					args: { item_id: itemId, speed: "2x", ripple: "track" },
				},
			],
		]);
		await expect(speed).toHaveValue("2");
	});

	test("the blend mode select sends clip.set_blend and shows the stored mode", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] ?? "";
		await open(page, seeded.id);
		const edits = recordEdits(page);

		const mode = page.getByLabel("Blend mode", { exact: true });
		await expect(mode).toHaveValue("normal");
		await mode.selectOption("multiply");
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{ op: "clip.set_blend", args: { item_id: itemId, mode: "multiply" } },
		]);
		await expect(mode).toHaveValue("multiply");
	});

	test("the mask controls send clip.set_mask with the whole mask and null for none", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] ?? "";
		await open(page, seeded.id);
		const edits = recordEdits(page);
		// The editor builds each mask command from the read model (SDD-012 section
		// 1, T3), so the next edit waits until the revision of the last one is
		// shown: a command sent before it would carry the old mask.
		const revision = page.locator("[data-connection] .font-mono");
		const applied = async (change: () => Promise<void>): Promise<void> => {
			const before = await revision.textContent();
			await change();
			await expect(revision).not.toHaveText(before ?? "");
		};
		const mask = {
			shape: "ellipse",
			x: 0.1,
			y: 0.1,
			width: 0.8,
			height: 0.8,
			feather: 0,
			invert: false,
		};

		const shape = page.getByLabel("Mask shape", { exact: true });
		await expect(shape).toHaveValue("none");
		await expect(page.getByLabel("Mask feather")).toHaveCount(0);
		await applied(() => shape.selectOption("ellipse").then(() => undefined));
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{ op: "clip.set_mask", args: { item_id: itemId, mask } },
		]);
		await expect(shape).toHaveValue("ellipse");

		const feather = page.getByLabel("Mask feather", { exact: true });
		await feather.fill("12");
		await applied(() => feather.press("Enter"));
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations).toEqual([
			{
				op: "clip.set_mask",
				args: { item_id: itemId, mask: { ...mask, feather: 12 } },
			},
		]);

		await applied(() =>
			page.getByLabel("Invert mask", { exact: true }).check(),
		);
		await expect.poll(() => edits.length).toBe(3);
		expect(edits[2]?.operations).toEqual([
			{
				op: "clip.set_mask",
				args: {
					item_id: itemId,
					mask: { ...mask, feather: 12, invert: true },
				},
			},
		]);

		await shape.selectOption("none");
		await expect.poll(() => edits.length).toBe(4);
		expect(edits[3]?.operations).toEqual([
			{ op: "clip.set_mask", args: { item_id: itemId, mask: null } },
		]);
		await expect(shape).toHaveValue("none");
	});

	test("a colour slider drag sends one command, on release", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] ?? "";
		await open(page, seeded.id);
		const edits = recordEdits(page);

		await openInspectorTab(page, "Adjust");
		const slider = page.getByLabel("Brightness", { exact: true });
		await slider.scrollIntoViewIfNeeded();
		const box = await slider.boundingBox();
		if (box === null) throw new Error("no slider box");
		const y = box.y + box.height / 2;
		await page.mouse.move(box.x + box.width / 2, y);
		await page.mouse.down();
		await page.mouse.move(box.x + box.width * 0.7, y, { steps: 6 });
		await page.mouse.move(box.x + box.width * 0.85, y, { steps: 6 });
		// Nothing was sent while the pointer is held.
		expect(edits).toHaveLength(0);
		await page.mouse.up();

		await expect.poll(() => edits.length).toBe(1);
		await page.waitForTimeout(300);
		expect(edits).toHaveLength(1);
		const op = edits[0]?.operations[0];
		expect(op?.op).toBe("clip.set_color");
		expect(Object.keys(op?.args ?? {}).sort()).toEqual([
			"brightness",
			"item_id",
		]);
		expect(op?.args.item_id).toBe(itemId);
		expect(op?.args.brightness).toBeGreaterThan(0.3);
	});

	test("remove silences is one history step whose text is the core description", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			files: [silenceClip()],
			tracks: 1,
			perTrack: 1,
			clipDuration: "10s",
		});
		await open(page, seeded.id);
		await page.keyboard.press("Alt+3");
		const list = page.getByRole("region", { name: "History" });
		const before = await list.locator("[data-revision]").count();
		const edits = recordEdits(page);
		const answered = page.waitForResponse(
			(r) => r.url().endsWith("/edit") && r.status() === 200,
		);
		// The row carries the description of the event until the refetch of the page
		// replaces it with the operation names (250 ms later): hold the refetch back,
		// so that the check does not race it.
		await page.route("**/history?*", async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 4000));
			await route.continue();
		});

		await openInspectorTab(page, "Audio");
		await page.getByRole("button", { name: "Remove silences" }).click();
		const body = (await (await answered).json()) as {
			revision: number;
			description: string;
		};

		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toEqual([
			{
				op: "clip.remove_silences",
				args: {
					item_id: seeded.itemIds[0],
					min_silence: "0.4s",
					padding: "0.1s",
					ripple: "track",
				},
			},
		]);
		expect(edits[0]?.operations[0]?.args).not.toHaveProperty("resolved");
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await expect(list.locator("[data-revision]")).toHaveCount(before + 1);
		const row = list.locator(`[data-revision="${body.revision}"]`);
		await row.getByRole("button").click();
		const first = body.description.split("\n")[0] ?? "";
		expect(first).not.toBe("");
		await expect(row).toContainText(first);
	});
});

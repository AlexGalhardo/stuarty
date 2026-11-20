import type { Page } from "@playwright/test";

import { audioClip, caller, expect, seedProject, test } from "./helpers";

// Epic 51 (SDD-004 section 7.1): the timeline preview plays the processed sound of an item. The page asks the server
// for the file by the key the plan hands out, a window first and then the whole item, and the inspector says
// "processing" and then "ready". A changed parameter is a new key and a new file.
const KEY_URL = /\/audio-previews\/([0-9a-f]{64})/;

async function seedTone(request: Parameters<typeof seedProject>[0]) {
	const seeded = await seedProject(request, {
		files: [audioClip()],
		tracks: 0,
		audioTracks: 1,
		perTrack: 0,
	});
	const call = await caller(request);
	const head = (await call("GET", `/projects/${seeded.id}`)) as {
		revision: number;
	};
	const out = (await call("POST", `/projects/${seeded.id}/edit`, {
		base_revision: head.revision,
		operations: [
			{
				op: "audio.add",
				args: {
					track_id: seeded.audioTrackIds[0],
					asset_id: seeded.assetIds[0],
					start: "0s",
				},
			},
		],
	})) as { effective: { created?: string[] }[] };
	return { id: seeded.id, tone: out.effective[0]?.created?.[0] };
}

async function filesOf(page: Page): Promise<string[]> {
	const keys: string[] = [];
	page.on("response", (r) => {
		const match = KEY_URL.exec(r.url());
		if (match?.[1] !== undefined && [200, 206].includes(r.status())) {
			keys.push(match[1]);
		}
	});
	return keys;
}

test.describe("processed audio preview", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test("an item with a chain plays its processed file, and a new parameter is a new file", async ({
		page,
		request,
	}) => {
		const s = await seedTone(request);
		const call = await caller(request);
		const head = (await call("GET", `/projects/${s.id}`)) as {
			revision: number;
		};
		await call("POST", `/projects/${s.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "audio.apply_preset",
					args: { item_id: s.tone, preset: "voice" },
				},
			],
		});

		const files = await filesOf(page);
		await page.goto(`/editor/${s.id}`);
		await page.locator(`[data-item-id="${s.tone}"]`).click();
		const strip = page.getByRole("tablist", { name: "Properties" });
		await strip.getByRole("tab", { name: "Audio" }).click();
		const section = page.locator('[data-section="audio-processors"]');
		await expect(section).toBeVisible();

		// The file is rendered by the server: the inspector says so, then that the timeline plays it.
		const state = section.locator("[data-processed-state]");
		await expect(state).toHaveAttribute("data-processed-state", "ready", {
			timeout: 60_000,
		});
		await expect(state).toHaveText("The timeline plays the processed sound.");
		await expect.poll(() => files.length).toBeGreaterThan(0);
		const first = files[0];
		expect(first).toMatch(/^[0-9a-f]{64}$/);

		// The key is the one the plan hands out for the item.
		const plan = (await call(
			"GET",
			`/projects/${s.id}/plan?from=0&until=30`,
		)) as {
			audio: { item: string; processed?: { key: string } }[];
		};
		expect(plan.audio.find((a) => a.item === s.tone)?.processed?.key).toBe(
			first,
		);

		// A byte range of the file is served, and only the file of this project.
		const fetched = (project: string) =>
			page.evaluate(
				async ([id, key]) => {
					const r = await fetch(
						`/api/v1/projects/${id}/audio-previews/${key}`,
						{
							credentials: "same-origin",
							headers: { Range: "bytes=0-3" },
						},
					);
					return { status: r.status, type: r.headers.get("Content-Type") };
				},
				[project, first] as const,
			);
		// The first file the page got may be the window; the whole item follows it.
		await expect
			.poll(async () => (await fetched(s.id)).status, { timeout: 60_000 })
			.toBe(206);
		expect((await fetched(s.id)).type).toBe("audio/wav");
		expect((await fetched("prj_zzzzzzzzzz")).status).toBeGreaterThanOrEqual(
			400,
		);

		// One more processor is a new key: the page asks for it, and the old file is not asked again as the item's.
		const next = page.waitForResponse(
			(r) => {
				const match = KEY_URL.exec(r.url());
				return (
					match?.[1] !== undefined &&
					match[1] !== first &&
					[200, 206].includes(r.status())
				);
			},
			{ timeout: 60_000 },
		);
		await section.getByLabel("Processor", { exact: true }).selectOption("gate");
		await section.getByRole("button", { name: "Add processor" }).click();
		const response = await next;
		expect(KEY_URL.exec(response.url())?.[1]).not.toBe(first);
		await expect(state).toHaveAttribute("data-processed-state", "ready", {
			timeout: 60_000,
		});
	});
});

import type { Page } from "@playwright/test";

import { audioClip, caller, expect, seedProject, test } from "./helpers";

// Epic 34 (SDD-004 section 7.1): the audio processor chain of an item and of a track in the inspector. Each control
// sends one command; the chain, the ranges and the presets come from the core.
async function sent(page: Page, op: string, action: () => Promise<void>) {
	const request = page.waitForRequest(
		(r) =>
			r.method() === "POST" &&
			/\/edit$/.test(new URL(r.url()).pathname) &&
			(r.postData() ?? "").includes(`"${op}"`),
	);
	await action();
	return request;
}

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
	return {
		id: seeded.id,
		tone: out.effective[0]?.created?.[0],
		track: seeded.audioTrackIds[0],
	};
}

async function chainOf(
	call: Awaited<ReturnType<typeof caller>>,
	id: string,
	item: string | undefined,
): Promise<{ kind: string; disabled?: boolean }[]> {
	const doc = (await call("GET", `/projects/${id}?detail=document`)) as {
		document?: { items: Record<string, { processors?: { kind: string }[] }> };
	};
	return doc.document?.items[item ?? ""]?.processors ?? [];
}

test.describe("audio processors", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test("an audio item gets a preset, a processor, a new order and a measure", async ({
		page,
		request,
	}) => {
		const s = await seedTone(request);
		const call = await caller(request);
		await page.goto(`/editor/${s.id}`);
		await page.locator(`[data-item-id="${s.tone}"]`).click();
		const strip = page.getByRole("tablist", { name: "Properties" });
		await strip.getByRole("tab", { name: "Audio" }).click();
		const section = page.locator('[data-section="audio-processors"]');
		await expect(section).toBeVisible();

		// A preset replaces the chain: the "voice" chain has five processors, the limiter last.
		await section.getByLabel("Preset").selectOption("voice");
		await sent(page, "audio.apply_preset", () =>
			section.getByRole("button", { name: "Apply preset" }).click(),
		);
		await expect(section.locator("[data-processor-index]")).toHaveCount(5);
		await expect(section.locator("[data-processor-index='4']")).toHaveAttribute(
			"data-processor-kind",
			"limiter",
		);
		const stored = await chainOf(call, s.id, s.tone);
		expect(stored.map((p) => p.kind)).toEqual([
			"noise_reduction",
			"eq",
			"deesser",
			"compressor",
			"limiter",
		]);

		// A processor added goes to the end; the buttons move and remove it.
		await section.getByLabel("Processor", { exact: true }).selectOption("gate");
		await sent(page, "audio.processor_add", () =>
			section.getByRole("button", { name: "Add processor" }).click(),
		);
		await expect(section.locator("[data-processor-index]")).toHaveCount(6);
		await sent(page, "audio.processor_move", () =>
			section.getByRole("button", { name: "Noise gate 6: Move up" }).click(),
		);
		await expect(section.locator("[data-processor-index='4']")).toHaveAttribute(
			"data-processor-kind",
			"gate",
		);

		// A parameter is one command with the number the person typed; the core checks its range.
		const card = section.locator("[data-processor-index='4']");
		await card.locator("summary").click();
		const threshold = card.getByLabel("Noise gate 5: Threshold");
		await threshold.fill("-42");
		const req = await sent(page, "audio.processor_set", () =>
			threshold.press("Enter"),
		);
		expect(req.postData()).toContain("-42");
		expect(req.postData()).toContain("threshold_db");

		await sent(page, "audio.processor_remove", () =>
			section.getByRole("button", { name: "Noise gate 5: Remove" }).click(),
		);
		await expect(section.locator("[data-processor-index]")).toHaveCount(5);

		// The meters come from the core: before and after the chain.
		await section.getByRole("button", { name: "Measure the sound" }).click();
		const meters = section.locator("[data-meters]");
		// The core decodes and mixes the item twice: seconds, longer while the other specs run.
		await expect(meters.getByRole("row", { name: /Before/ })).toBeVisible({
			timeout: 60_000,
		});
		await expect(meters.getByRole("row", { name: /After/ })).toBeVisible();

		// The processed sound is an audio-only render of the item (format wav) by the mixer of the export, then played
		// from the file of the job (SDD-004 section 7.1).
		const render = page.waitForRequest(
			(r) => r.method() === "POST" && (r.postData() ?? "").includes('"wav"'),
		);
		await section
			.getByRole("button", { name: "Listen to the processed sound" })
			.click();
		await render;
		const player = section.getByLabel("Processed sound");
		await expect(player).toBeVisible({ timeout: 90_000 });
		await expect(player).toHaveAttribute("src", /.+/);
	});

	test("a track has a chain of its own", async ({ page, request }) => {
		const s = await seedTone(request);
		await page.goto(`/editor/${s.id}`);
		await page.getByText("Audio 1", { exact: false }).first().click();
		const section = page.locator('[data-section="audio-processors"]');
		await expect(section).toBeVisible();
		await section
			.getByLabel("Processor", { exact: true })
			.selectOption("limiter");
		await sent(page, "audio.processor_add", () =>
			section.getByRole("button", { name: "Add processor" }).click(),
		);
		await expect(section.locator("[data-processor-index]")).toHaveCount(1);
		// A track has no single clip to mark a noise profile on: the marked range is not offered.
		await section
			.getByLabel("Processor", { exact: true })
			.selectOption("noise_reduction");
		await sent(page, "audio.processor_add", () =>
			section.getByRole("button", { name: "Add processor" }).click(),
		);
		const nr = section.locator("[data-processor-kind='noise_reduction']");
		await nr.locator("summary").click();
		await expect(nr.getByLabel(/Strength/)).toBeVisible();
		await expect(nr.getByLabel(/Noise profile starts/)).toHaveCount(0);
	});
});

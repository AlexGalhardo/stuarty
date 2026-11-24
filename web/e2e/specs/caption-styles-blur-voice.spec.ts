import AxeBuilder from "@axe-core/playwright";
import type { Locator, Page } from "@playwright/test";

import { caller, expect, seedProject, test } from "./helpers";

// Epic 71 (SDD-012 section 12.1, SDD-002 C.88 to C.91): the caption style gallery of a track, the Blur a region
// section of a clip and the studio voice action. Each is reached and used with the keyboard, sends one command,
// and has no serious or critical axe violation in either theme, at a wide window and at the narrowest one the editor opens in.

const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];

async function seed(request: Parameters<typeof seedProject>[0]) {
	const seeded = await seedProject(request, { tracks: 2, perTrack: 0 });
	const call = await caller(request);
	const head = (await call("GET", `/projects/${seeded.id}`)) as {
		revision: number;
	};
	const [video, captions] = seeded.trackIds;
	const out = (await call("POST", `/projects/${seeded.id}/edit`, {
		base_revision: head.revision,
		operations: [
			{
				op: "clip.add",
				args: {
					track_id: video,
					asset_id: seeded.assetId,
					start: "0s",
					source_in: "0s",
					duration: "4s",
				},
			},
			{
				op: "caption.add",
				args: {
					track_id: captions,
					start: "0s",
					duration: "2s",
					text: "Caption words",
				},
			},
		],
	})) as { effective: { created?: string[] }[] };
	return {
		id: seeded.id,
		call,
		clip: out.effective[0]?.created?.[0] ?? "",
		captions: captions ?? "",
	};
}

// The specimen of a caption style is a picture of the look, drawn in the colours of the style over the dark
// canvas, with the name of the style as the text of its tile: it is incidental text (WCAG 1.4.3), and axe takes
// the outline of a style for its text colour, so it is left out of the scan. Everything else of the tile is in.
async function serious(page: Page, include: string): Promise<string[]> {
	const { violations } = await new AxeBuilder({ page })
		.include(include)
		.exclude("[data-caption-sample]")
		.withTags(TAGS)
		.analyze();
	return violations
		.filter((v) => v.impact === "serious" || v.impact === "critical")
		.map(
			(v) =>
				`${v.id}: ${v.nodes.map((n) => `${n.target.join(" ")} ${n.failureSummary}`).join(", ")}`,
		);
}

// Moves the focus with Tab until the locator has it, as a keyboard user would.
async function tabTo(page: Page, target: Locator): Promise<void> {
	for (let i = 0; i < 200; i++) {
		if (await target.evaluate((el) => el === document.activeElement)) return;
		await page.keyboard.press("Tab");
	}
	throw new Error("the control is not reachable with Tab");
}

for (const theme of ["dark", "light"] as const) {
	// 1024 is the narrowest window the editor opens in; below it the page asks for a wider one.
	for (const width of [1440, 1024] as const) {
		test.describe(`epic 71 panels, ${theme}, ${width} px`, () => {
			// The dark theme is the default whatever the system says (SDD-012 section 17 item 1).
			test.use({ viewport: { width, height: 900 } });

			test("the gallery, the blur form and the voice action work from the keyboard", async ({
				page,
				request,
			}, testInfo) => {
				test.setTimeout(180_000);
				await page.addInitScript((value) => {
					if (value === "light") localStorage.setItem("theme", "light");
				}, theme);
				const s = await seed(request);
				await page.goto(`/editor/${s.id}`);
				const html = page.locator("html");
				if (theme === "dark") await expect(html).toHaveClass(/\bdark\b/);
				else await expect(html).not.toHaveClass(/\bdark\b/);
				const shot = async (name: string, of: Locator): Promise<void> => {
					await of.screenshot({
						path: testInfo.outputPath(`${name}-${theme}-${width}.png`),
					});
				};

				// The gallery of the caption track: 14 tiles, each named; Enter on one applies it.
				await page.locator(`[data-header-for="${s.captions}"]`).click();
				const gallery = page.getByTestId("caption-style-gallery");
				await expect(gallery.locator("button[data-style-id]")).toHaveCount(14);
				const tile = gallery.getByRole("button", {
					name: "Use the Bold pop style",
				});
				await tabTo(page, tile);
				await page.keyboard.press("Enter");
				await expect
					.poll(async () => {
						const doc = (await s.call(
							"GET",
							`/projects/${s.id}?detail=document`,
						)) as {
							document?: {
								tracks: Record<
									string,
									{ caption_style?: { font_family?: string } }
								>;
							};
						};
						return doc.document?.tracks[s.captions]?.caption_style?.font_family;
					})
					.toBe("Anton");
				await gallery.scrollIntoViewIfNeeded();
				await shot("caption-gallery", gallery);
				expect(
					await serious(page, '[data-testid="caption-style-gallery"]'),
				).toEqual([]);

				// The blur form of the clip: a region at the top edge is blurred and the editor says it is near the edge.
				await page.locator(`[data-item-id="${s.clip}"]`).click();
				const strip = page.getByRole("tablist", { name: "Properties" });
				await strip.getByRole("tab", { name: "Adjust" }).click();
				const blur = page.locator('[data-section="blur-region"]');
				await expect(blur).toBeVisible();
				const top = blur.getByLabel("Top", { exact: true });
				await top.fill("0");
				await top.press("Enter");
				const apply = blur.getByRole("button", { name: "Blur this region" });
				await tabTo(page, apply);
				await page.keyboard.press("Enter");
				await expect(blur.getByRole("status")).toContainText(
					"close to an edge",
				);
				await blur.scrollIntoViewIfNeeded();
				await shot("blur-region", blur);
				expect(await serious(page, '[data-section="blur-region"]')).toEqual([]);

				// The studio voice action of the clip.
				await page.locator(`[data-item-id="${s.clip}"]`).click();
				await strip.getByRole("tab", { name: "Audio" }).click();
				const voice = page.getByTestId("voice-preset");
				await expect(voice).toBeVisible();
				await voice.getByLabel("Studio voice").selectOption("broadcast");
				const voiceApply = voice.getByRole("button", {
					name: "Apply studio voice",
				});
				await tabTo(page, voiceApply);
				await page.keyboard.press("Enter");
				await expect(
					page.locator(
						'[data-section="audio-processors"] [data-processor-index]',
					),
				).toHaveCount(6);
				await voice.scrollIntoViewIfNeeded();
				await shot("voice-preset", voice);
				expect(await serious(page, '[data-testid="voice-preset"]')).toEqual([]);
			});
		});
	}
}

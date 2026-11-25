import { mkdirSync } from "node:fs";
import { join } from "node:path";

import type { Page, Route } from "@playwright/test";

import { caller, expect, seedProject, test } from "./helpers";

// Epic 99 group B: the video editor screens of the final design (05, 25 to 30). Each test opens one screen on a real
// project and asserts what the frame shows. With E99_CAPTURE_DIR set it also writes the captures of the screens, in
// both themes, named as the frames are.
const CAPTURES = process.env.E99_CAPTURE_DIR;

test.use({ viewport: { width: 1440, height: 900 } });

async function capture(
	page: Page,
	frame: string,
	theme: "light" | "dark",
): Promise<void> {
	if (CAPTURES === undefined) return;
	mkdirSync(CAPTURES, { recursive: true });
	await page.screenshot({
		path: join(
			CAPTURES,
			`${frame} _ ${theme === "light" ? "claro" : "escuro"}.png`,
		),
	});
}

async function seed(request: Parameters<typeof seedProject>[0]) {
	const seeded = await seedProject(request, { tracks: 2, perTrack: 0 });
	const call = await caller(request);
	const head = (await call("GET", `/projects/${seeded.id}`)) as {
		revision: number;
	};
	const [video, captions] = seeded.trackIds;
	const keyframe = (frame: number, value: number) => ({
		op: "keyframe.set",
		args: {
			item_id: "itm_zm00000001",
			property: "scale",
			at: `${frame}f`,
			value,
		},
	});
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
					id: "itm_zm00000001",
				},
			},
			{
				op: "caption.add",
				args: {
					track_id: captions,
					start: "0s",
					duration: "2s",
					text: "a luz vem de frente",
				},
			},
			keyframe(0, 1),
			keyframe(6, 1.6),
			keyframe(24, 1.6),
			keyframe(36, 1),
			keyframe(60, 1),
			keyframe(66, 1.8),
			keyframe(90, 1.8),
			keyframe(100, 1),
		],
	})) as { effective: { created?: string[] }[] };
	return {
		id: seeded.id,
		assetId: seeded.assetId,
		call,
		clip: out.effective[0]?.created?.[0] ?? "itm_zm00000001",
	};
}

const json = (route: Route, body: unknown, status = 200): Promise<void> =>
	route.fulfill({ status, contentType: "application/json", json: body });

for (const theme of ["light", "dark"] as const) {
	test.describe(`the video editor screens, ${theme} @chromium-only`, () => {
		test.beforeEach(async ({ page }) => {
			await page.addInitScript((value) => {
				if (value === "light") localStorage.setItem("theme", "light");
			}, theme);
		});

		test("05: the product mark opens the home, and the bar names the project", async ({
			page,
			request,
		}) => {
			const s = await seed(request);
			await page.goto(`/editor/${s.id}`);
			const mark = page.getByTestId("product-mark");
			await expect(mark).toBeVisible();
			await expect(mark).toHaveAttribute("href", "/");
			await expect(mark.locator("img")).toHaveAttribute("src", "/icon.png");
			await page.locator(`[data-item-id="${s.clip}"]`).click();
			await capture(page, "05 Editor de vídeo", theme);
		});

		test("25: the zoom lane and the list of zooms come from the service", async ({
			page,
			request,
		}) => {
			const s = await seed(request);
			await page.goto(`/editor/${s.id}`);
			await page.locator(`[data-item-id="${s.clip}"]`).click();
			await page.getByRole("tab", { name: "Effects" }).click();
			const inspector = page.getByRole("complementary", { name: "Inspector" });
			await expect(
				inspector.getByRole("heading", { name: "Zoom on click" }),
			).toBeVisible();
			await expect(inspector.getByText("2 zooms on lane Z1")).toBeVisible();
			const list = inspector.getByTestId("zoom-list");
			await expect(list.getByText("Zoom 1")).toBeVisible();
			await expect(list.getByText("Zoom 2")).toBeVisible();
			const lane = page.getByTestId("zoom-lane");
			await expect(lane.locator("[data-zoom]")).toHaveCount(2);
			await expect(inspector.getByRole("switch")).toBeChecked();

			// Off clears the keyframes; the list and the lane follow what the service reads.
			await inspector.getByRole("switch").click();
			await expect(inspector.getByText("0 zooms on lane Z1")).toBeVisible();
			await expect(page.getByTestId("zoom-lane")).toHaveCount(0);
			const doc = (await s.call(
				"GET",
				`/projects/${s.id}?detail=document`,
			)) as {
				document: { items: Record<string, { keyframes?: unknown }> };
			};
			expect(doc.document.items[s.clip]?.keyframes ?? {}).toEqual({});

			// Zooms made by hand count the same way: the keyframes bring the lane back.
			await s.call("POST", `/projects/${s.id}/edit`, {
				base_revision: (
					(await s.call("GET", `/projects/${s.id}`)) as { revision: number }
				).revision,
				operations: [
					{
						op: "keyframe.set",
						args: { item_id: s.clip, property: "scale", at: "0f", value: 1 },
					},
					{
						op: "keyframe.set",
						args: { item_id: s.clip, property: "scale", at: "6f", value: 1.6 },
					},
					{
						op: "keyframe.set",
						args: { item_id: s.clip, property: "scale", at: "24f", value: 1.6 },
					},
					{
						op: "keyframe.set",
						args: { item_id: s.clip, property: "scale", at: "36f", value: 1 },
					},
				],
			});
			await expect(inspector.getByText("1 zoom on lane Z1")).toBeVisible();
			await capture(page, "25 Editor de vídeo_ acabamento", theme);
		});

		test("28: the captions inspector has the Style, Text and Time tabs", async ({
			page,
			request,
		}) => {
			const s = await seed(request);
			await page.goto(`/editor/${s.id}`);
			await page.getByRole("tab", { name: "Captions" }).click();
			const inspector = page.getByRole("complementary", { name: "Inspector" });
			const tabs = inspector.getByRole("tablist", { name: "Properties" });
			await expect(tabs.getByRole("tab", { name: "Style" })).toBeVisible();
			await expect(tabs.getByRole("tab", { name: "Text" })).toBeVisible();
			await expect(tabs.getByRole("tab", { name: "Time" })).toBeVisible();
			await expect(
				inspector.getByTestId("caption-style-gallery").locator("button"),
			).toHaveCount(14);
			await tabs.getByRole("tab", { name: "Text" }).click();
			await expect(
				inspector.getByText("Select a caption on the timeline to edit it."),
			).toBeVisible();
			await tabs.getByRole("tab", { name: "Style" }).click();
			await capture(page, "28 Editor de vídeo_ legendas", theme);
		});

		test("29: the blur screen has the X, Y, W, H grid and the strengths of the core", async ({
			page,
			request,
		}) => {
			const s = await seed(request);
			await page.goto(`/editor/${s.id}`);
			await page.locator(`[data-item-id="${s.clip}"]`).click();
			await page
				.getByRole("tablist", { name: "Properties" })
				.getByRole("tab", { name: "Adjust" })
				.click();
			const blur = page.getByTestId("blur-region");
			await expect(blur).toBeVisible();
			await expect(blur.locator('[data-grid="region"] input')).toHaveCount(4);
			const strengths = blur.getByRole("group", { name: "Strength" });
			await expect(
				strengths.getByRole("button", { name: "Strong" }),
			).toBeVisible();
			await strengths.getByRole("button", { name: "Medium" }).click();
			await blur.scrollIntoViewIfNeeded();
			await capture(page, "29 Editor de vídeo_ desfoque", theme);
			let sent: { operations: { op: string; args: Record<string, unknown> }[] };
			await page.route("**/api/v1/projects/*/edit**", async (route) => {
				sent = route.request().postDataJSON();
				await route.continue();
			});
			await blur.getByRole("button", { name: "Add blur" }).click();
			await expect.poll(() => sent?.operations[0]?.op).toBe("clip.blur_region");
			expect(sent?.operations[0]?.args.strength).toBe("medium");
			expect(sent?.operations[0]?.args.radius).toBeUndefined();
		});

		test("30: the studio voice stage shows the sound before and after", async ({
			page,
			request,
		}) => {
			const s = await seed(request);
			await page.goto(`/editor/${s.id}`);
			await page.locator(`[data-item-id="${s.clip}"]`).click();
			await page.locator('[data-rail-tab="audio"]').click();
			await page.getByTestId("open-voice-stage").click();
			const stage = page.locator('[data-stage="voice"]');
			await expect(stage).toBeVisible();
			await expect(stage.locator('[data-card="before"]')).toBeVisible();
			await expect(stage.locator('[data-card="after"]')).toBeVisible();
			await expect(
				stage.locator('[data-card="before"] [data-wave="drawn"]'),
			).toBeVisible({ timeout: 30_000 });
			// The inspector is the chain, with the studio voice preset that makes it.
			const inspector = page.getByRole("complementary", { name: "Inspector" });
			await expect(inspector.getByTestId("voice-preset")).toBeVisible();
			await inspector
				.getByRole("button", { name: "Apply studio voice" })
				.click();
			await expect
				.poll(
					async () =>
						JSON.stringify(
							await s.call("GET", `/projects/${s.id}?detail=document`),
						),
					{ timeout: 20_000 },
				)
				.toContain("processors");
			await expect(
				stage.locator('[data-card="after"] [data-wave="drawn"]'),
			).toBeVisible({ timeout: 60_000 });
			await capture(page, "30 Editor de vídeo_ voz de estúdio", theme);
			await stage.getByRole("button", { name: "Back to the editor" }).click();
			await expect(stage).toHaveCount(0);
		});

		test("26: the transcript sits beside the preview and the review is at the right", async ({
			page,
			request,
		}) => {
			const s = await seed(request);
			const words = [
				"Você",
				"gasta",
				"em",
				"câmera",
				"né",
				"e",
				"esquece",
				"o",
				"que",
				"aparece.",
				"O",
				"primeiro",
				"erro",
				"é",
				"é",
				"gravar",
				"contra",
				"a",
				"janela.",
			];
			await page.route("**/api/v1/projects/*/transcript**", (route) => {
				const url = route.request().url();
				if (url.includes("/transcript/fillers")) {
					return json(route, {
						project_id: s.id,
						revision: 1,
						asset_id: s.assetId,
						language: "pt",
						rule: "pt",
						filler_rules: true,
						text: "",
						suggestions: [
							{
								kind: "filler",
								strength: "strong",
								from_word: 4,
								to_word: 4,
								text: "né",
								start_ms: 3000,
								end_ms: 3200,
							},
							{
								kind: "repetition",
								strength: "strong",
								from_word: 13,
								to_word: 14,
								text: "é é",
								start_ms: 12000,
								end_ms: 12400,
							},
							{
								kind: "filler",
								strength: "weak",
								from_word: 6,
								to_word: 6,
								text: "esquece",
								start_ms: 5000,
								end_ms: 5200,
							},
						],
					});
				}
				return json(route, {
					project_id: s.id,
					revision: 1,
					asset_id: s.assetId,
					language: "pt",
					skipped_items: [],
					text: "",
					words: words.map((text, index) => ({
						index,
						text,
						start: index * 6,
						end: index * 6 + 5,
						ranges: [
							{
								item_id: s.clip,
								track_id: "trk",
								start: index * 6,
								end: index * 6 + 5,
							},
						],
					})),
				});
			});
			await page.goto(`/editor/${s.id}`);
			await page.locator(`[data-item-id="${s.clip}"]`).click();
			await page.getByRole("tab", { name: "Captions" }).click();
			await page.getByRole("button", { name: "Edit by text" }).click();
			const stage = page.locator('[data-stage="transcript"]');
			await expect(stage.locator("[data-word-index]")).toHaveCount(
				words.length,
			);
			// The transcript is the wide region, and the preview is still on the page beside it.
			const box = await stage.boundingBox();
			expect(box?.width ?? 0).toBeGreaterThan(500);
			await expect(page.getByRole("main", { name: "Preview" })).toBeVisible();
			const review = page.getByRole("complementary", { name: "Inspector" });
			await expect(
				review.getByRole("heading", { name: "Review before removing" }),
			).toBeVisible();
			await review
				.getByRole("button", { name: "Find fillers and repetitions" })
				.click();
			await expect(review.locator("[data-fillers] li")).toHaveCount(3);
			await expect(review.getByText("2 of 3")).toBeVisible();
			await expect(stage.locator('[data-mark="filler"]').first()).toBeVisible();
			await expect(stage.locator('[data-mark="repetition"]')).toHaveCount(2);
			await capture(page, "26 Editor de vídeo_ editar pelo texto", theme);
		});

		test("27: the vertical cuts are cards on the stage with the framing review", async ({
			page,
			request,
		}) => {
			const s = await seed(request);
			const mediaFile = { hash: `sha256:${"a".repeat(64)}`, asset_id: "ast_x" };
			await page.route("**/api/v1/system/tools", (route) =>
				json(route, {
					installed: [],
					offers: [
						{
							name: "face",
							files: [],
							download_bytes: 0,
							dataset: "",
							published: true,
							installed: true,
						},
					],
					state: "idle",
					done_bytes: 0,
					total_bytes: 0,
					text: "",
				}),
			);
			await page.route("**/api/v1/projects/*/jobs**", (route) => {
				if (route.request().method() !== "POST") return route.fallback();
				return json(route, {
					job_id: "job_e2e0000002",
					kind: "face_track",
					project_id: s.id,
					state: "succeeded",
					progress: 1,
					actor: "local:ui",
					created_at: new Date().toISOString(),
					text: "face_track succeeded",
					result: {
						face_track: {
							asset_id: s.assetId,
							samples: 10,
							faces: 10,
							provider: "cpu",
							fps: 5,
						},
					},
				});
			});
			const proposal = (
				n: number,
				start: number,
				end: number,
				title: string,
			) => ({
				n,
				start_ms: start,
				end_ms: end,
				words: 60 + n,
				face_share: 960 - n * 10,
				score: 1,
				title,
				batch: [],
			});
			await page.route("**/api/v1/projects/*/cuts**", (route) =>
				json(route, {
					project_id: s.id,
					revision: 1,
					asset_id: s.assetId,
					media_file: mediaFile,
					text: "3 vertical cuts",
					proposals: [
						proposal(1, 42_000, 98_000, "Gancho: a luz vem de frente"),
						proposal(2, 185_000, 218_000, "Erro 1: contra a janela"),
						proposal(3, 561_000, 590_000, "Dica final: um palmo"),
					],
				}),
			);
			await page.goto(`/editor/${s.id}`);
			await page.locator(`[data-item-id="${s.clip}"]`).click();
			await page
				.getByRole("tablist", { name: "Properties" })
				.getByRole("tab", { name: "Text" })
				.click();
			await page.getByRole("button", { name: "Open vertical cuts" }).click();
			await page.getByRole("button", { name: "Find vertical cuts" }).click();
			const cards = page.getByRole("list", { name: "Proposed cuts" });
			await expect(cards.locator("[data-proposal]")).toHaveCount(3);
			await expect(
				page.getByRole("heading", { name: "3 cuts proposed" }),
			).toBeVisible();
			const review = page.getByRole("complementary", { name: "Inspector" });
			await expect(
				review.getByRole("heading", { name: "Cut in review" }),
			).toBeVisible();
			await expect(review.getByText("clip.face_frame")).toBeVisible();
			await expect(review.getByText("caption.apply_style")).toBeVisible();
			// Refusing a card removes it from the stage and nothing is sent.
			await cards
				.locator('[data-proposal="3"]')
				.getByRole("button", { name: "Refuse" })
				.click();
			await expect(cards.locator("[data-proposal]")).toHaveCount(2);
			await page.reload();
			await page.locator(`[data-item-id="${s.clip}"]`).click();
			await page
				.getByRole("tablist", { name: "Properties" })
				.getByRole("tab", { name: "Text" })
				.click();
			await page.getByRole("button", { name: "Open vertical cuts" }).click();
			await page.getByRole("button", { name: "Find vertical cuts" }).click();
			await expect(cards.locator("[data-proposal]")).toHaveCount(3);
			await page.waitForFunction(() =>
				Array.from(
					document.querySelectorAll<HTMLImageElement>("[data-proposal] img"),
				).every((image) => image.complete),
			);
			await capture(page, "27 Editor de vídeo_ cortes verticais", theme);
		});
	});
}

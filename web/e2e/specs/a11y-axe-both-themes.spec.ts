import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { NETWORK_ADMIN, NETWORK_URL } from "../support/network";
import {
	audioClip,
	caller,
	expect,
	seedProject,
	test,
	testClip,
} from "./helpers";
import { newImage } from "./image-support";

// P2-UI-031 (SDD-012 sections 17 and 18.4 item 1): axe over the editor in every
// state, in both themes. A `serious` or `critical` violation fails the test; a
// `moderate` or `minor` one fails too unless a11y-allow.json lists it with a
// reason.
interface Allowed {
	rule: string;
	state?: string;
	reason: string;
}

const allow = JSON.parse(
	readFileSync(
		join(dirname(fileURLToPath(import.meta.url)), "../a11y-allow.json"),
		"utf8",
	),
) as Allowed[];

const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];

interface Finding {
	state: string;
	rule: string;
	impact: string | null | undefined;
	nodes: string[];
}

// Findings of every state are collected and asserted once at the end, so a run
// reports all of them.
async function scan(
	page: Page,
	state: string,
	found: Finding[],
): Promise<void> {
	// A dialog or a popover that is still fading in has a text color halfway to
	// its final one, which axe reads as low contrast; the slower Firefox and
	// WebKit runners catch it mid-transition. Endless animations (a spinner)
	// are left out: they never finish.
	await page.evaluate(() =>
		Promise.race([
			Promise.all(
				document
					.getAnimations()
					.filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
					.map((a) => a.finished.catch(() => undefined)),
			),
			new Promise((resolve) => setTimeout(resolve, 3000)),
		]),
	);
	const { violations } = await new AxeBuilder({ page })
		.withTags(TAGS)
		.analyze();
	for (const v of violations) {
		const listed =
			v.impact !== "serious" &&
			v.impact !== "critical" &&
			allow.some(
				(a) => a.rule === v.id && (a.state === undefined || a.state === state),
			);
		if (listed) continue;
		found.push({
			state,
			rule: v.id,
			impact: v.impact,
			nodes: v.nodes.map((n) => `${n.target.join(" ")} ${n.failureSummary}`),
		});
	}
}

// The names the test looks up, in each language (resources/lang).
const NAMES = {
	en: {
		preview: "Preview",
		exportOpen: "Export",
		history: "History",
		start: "Start export",
	},
	"pt-BR": {
		preview: "Pré-visualização",
		exportOpen: "Exportar",
		history: "Histórico",
		start: "Iniciar exportação",
	},
} as const;

for (const lang of ["en", "pt-BR"] as const) {
	for (const theme of ["dark", "light"] as const) {
		const names = NAMES[lang];
		test.describe(`a11y-axe-both-themes ${theme} ${lang}`, () => {
			// The dark theme is checked with the system in light mode: the default
			// ignores prefers-color-scheme (SDD-012 section 17 item 1).
			test.use({ colorScheme: theme === "dark" ? "light" : "dark" });
			// From 1440 px the palette button shows its text, whose name the
			// label must start with (VV-064); below it the button is an icon.
			test.use({ viewport: { width: 1600, height: 900 } });

			test(`the editor has no serious or critical violation in ${theme} in ${lang}`, async ({
				page,
				request,
			}) => {
				// Eleven states and a whole export: headless Firefox takes over 3 minutes on a loaded machine.
				test.setTimeout(300_000);
				await page.addInitScript((value) => {
					if (value === "light") localStorage.setItem("theme", "light");
				}, theme);
				await page.addInitScript((value) => {
					localStorage.setItem("vv.lang", value);
				}, lang);

				const empty = await seedProject(request, { tracks: 0, perTrack: 0 });
				const seeded = await seedProject(request, {
					files: [testClip(), audioClip()],
					tracks: 1,
					audioTracks: 1,
					perTrack: 0,
				});
				const call = await caller(request);
				const head = (await call("GET", `/projects/${seeded.id}`)) as {
					revision: number;
				};
				const visual = seeded.trackIds[0];
				const audio = seeded.audioTrackIds[0];
				const out = (await call("POST", `/projects/${seeded.id}/edit`, {
					base_revision: head.revision,
					operations: [
						{
							op: "clip.add",
							args: {
								track_id: visual,
								asset_id: seeded.assetIds[0],
								start: "0s",
								source_in: "0s",
								duration: "2s",
							},
						},
						{
							op: "text.add",
							args: {
								track_id: visual,
								start: "2s",
								duration: "2s",
								text: "Hi",
							},
						},
						{
							op: "shape.add",
							args: {
								track_id: visual,
								start: "4s",
								duration: "2s",
								shape: "rounded_rect",
								width: 400,
								height: 240,
							},
						},
						{
							op: "caption.add",
							args: {
								track_id: visual,
								start: "6s",
								duration: "2s",
								text: "Caption words",
							},
						},
						{
							op: "audio.add",
							args: {
								track_id: audio,
								asset_id: seeded.assetIds[1],
								start: "0s",
							},
						},
					],
				})) as { effective: { created?: string[] }[] };
				// `created` at the top level is sorted by id, not by operation.
				const [clip, text, shape, caption, tone] = out.effective.map(
					(effect) => effect.created?.[0],
				);

				const found: Finding[] = [];
				const html = page.locator("html");
				const check = async (): Promise<void> => {
					if (theme === "dark") await expect(html).toHaveClass(/\bdark\b/);
					else await expect(html).not.toHaveClass(/\bdark\b/);
				};

				await page.goto(`/editor/${empty.id}`);
				await expect(
					page.getByRole("main", { name: names.preview }),
				).toBeVisible();
				await check();
				await scan(page, "empty project", found);

				// A failed card in the library.
				// media* also takes the timeline_id the editor puts in the query (SDD-003 section 1.7).
				await page.route("**/api/v1/projects/*/media*", async (route) => {
					if (route.request().method() !== "GET") {
						await route.fallback();
						return;
					}
					const response = await route.fetch();
					const body = (await response.json()) as {
						assets: Record<string, unknown>[];
					};
					const [first, ...rest] = body.assets;
					if (first !== undefined) {
						first.state = "failed";
						first.error = {
							code: "E_MEDIA_UNSUPPORTED",
							message: "The file could not be read.",
							hint: "Convert it to MP4.",
						};
					}
					await route.fulfill({
						response,
						json: { ...body, assets: first ? [first, ...rest] : rest },
					});
				});
				await page.goto(`/editor/${seeded.id}`);
				await expect(page.locator("[data-item-id]")).toHaveCount(5);
				await expect(
					page.getByText("The file could not be read."),
				).toBeVisible();
				await scan(page, "library with an error card", found);
				await page.unroute("**/api/v1/projects/*/media*");
				await page.reload();
				await expect(page.locator("[data-item-id]")).toHaveCount(5);
				await scan(page, "project with items", found);

				const selected: [string, string | undefined][] = [
					["clip selected", clip],
					["text selected", text],
					["shape selected", shape],
					["caption selected", caption],
					["audio selected", tone],
				];
				for (const [state, id] of selected) {
					await page.locator(`[data-item-id="${id}"]`).click();
					await scan(page, state, found);
				}

				await page.keyboard.press("Escape");
				await page.keyboard.press("Alt+3");
				await expect(
					page.getByRole("region", { name: names.history }),
				).toBeVisible();
				await scan(page, "history", found);
				await page.keyboard.press("Alt+3");

				await page.keyboard.press("Control+k");
				await expect(page.getByRole("dialog")).toBeVisible();
				await scan(page, "palette", found);
				await page.keyboard.press("Escape");
				await expect(page.getByRole("dialog")).toHaveCount(0);

				await page.getByRole("button", { name: names.exportOpen }).click();
				await expect(page.getByRole("dialog")).toBeVisible();
				// The start button is disabled while the checks run and fades to its
				// enabled colours when they end: scan after, not during the fade.
				await expect(
					page.getByRole("button", { name: names.start }),
				).toBeEnabled();
				await scan(page, "export dialog, idle", found);
				await page.getByRole("button", { name: names.start }).click();
				const dialog = page.getByRole("dialog");
				await expect(dialog.locator("[data-phase]")).toBeVisible();
				await scan(page, "export dialog, started", found);
				await expect(dialog.locator("[data-phase='succeeded']")).toBeVisible({
					timeout: 120_000,
				});
				await scan(page, "export dialog, succeeded", found);
				expect(found, "axe violations").toEqual([]);
			});
		});
	}
}

// Epic 41 task 2 (NFR-065): axe on every route of the local server and of the
// network server, in both themes and both languages. The editors are scanned
// by the specs above and by a11y-image-editor; their entry pages are here.
const ROUTES = [
	"/",
	"/projects",
	"/projects/:project",
	"/edit-video",
	"/edit-image",
	"/edit-image/:image",
	"/write",
	"/visual-assets",
	"/video-assets",
	"/sound-assets",
	"/publish",
	"/approvals",
	"/skills",
	"/prompts",
	"/memory",
	"/connections",
	"/settings",
	"/settings/brand-kit",
] as const;

for (const lang of ["en", "pt-BR"] as const) {
	for (const theme of ["dark", "light"] as const) {
		test.describe(`a11y-axe-every-route ${theme} ${lang}`, () => {
			test.use({ colorScheme: theme === "dark" ? "light" : "dark" });
			test.describe.configure({ timeout: 240_000 });

			test(`no route has a violation in ${theme} in ${lang}`, async ({
				page,
				request,
			}) => {
				await page.addInitScript(
					([th, l]) => {
						localStorage.setItem("theme", th);
						localStorage.setItem("vv.lang", l);
					},
					[theme, lang],
				);
				const project = await seedProject(request, { tracks: 1, perTrack: 0 });
				const image = await newImage(request, { name: "A11y routes" });
				const found: Finding[] = [];
				for (const route of ROUTES) {
					const path = route
						.replace(":project", project.id)
						.replace(":image", image.id);
					await page.goto(path);
					await expect(
						page.getByRole("heading", { level: 1 }).first(),
					).toBeVisible();
					await scan(page, path, found);
				}
				expect(found, "axe violations").toEqual([]);
			});
		});
	}
}

test.describe("a11y-axe-network-pages", () => {
	test.describe.configure({ timeout: 240_000 });
	for (const theme of ["dark", "light"] as const) {
		test(`the sign-in and key pages have no violation in ${theme}`, async ({
			page,
		}) => {
			await page.addInitScript(
				(th) => localStorage.setItem("theme", th),
				theme,
			);
			const found: Finding[] = [];
			await page.goto(`${NETWORK_URL}/login`);
			await expect(page.getByLabel("Email")).toBeVisible();
			await scan(page, "/login", found);
			// One sign-in per test: the server allows five a minute for one email.
			await page.getByLabel("Email").fill(NETWORK_ADMIN.email);
			await page.getByLabel("Password").fill(NETWORK_ADMIN.password);
			await page.getByRole("button", { name: /^Log in$/ }).click();
			for (const path of ["/settings/keys", "/settings/google"]) {
				await page.goto(`${NETWORK_URL}${path}`);
				await expect(
					page.getByRole("heading", { level: 1 }).first(),
				).toBeVisible();
				await scan(page, path, found);
			}
			expect(found, "axe violations").toEqual([]);
		});
	}
});

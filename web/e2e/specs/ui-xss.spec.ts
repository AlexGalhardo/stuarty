import { copyFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { cspViolations } from "../support/browsers";
import { caller, expect, seedProject, test, testClip } from "./helpers";

// P2-SEC-005 (SDD-007 section 8.5, threat T-22): text written by an agent or
// read from a file is shown as text. Every view that shows it is opened with
// a payload that would run script if it reached the DOM as HTML.
const PAYLOAD = "<img src=x onerror=window.__pwned=1>";
const WORD = "<img/src=x/onerror=window.__pwned=1>";
// Windows refuses < and > in a file name: there the asset name carries the
// payload as entities, which must also be shown literally.
const ASSET_NAME =
	process.platform === "win32"
		? "&lt;img src=x onerror=window.__pwned=1&gt;"
		: PAYLOAD;

declare global {
	interface Window {
		__pwned?: number;
		__csp?: string[];
	}
}

function hostileAsset(): string {
	const dir = join(tmpdir(), "vukem-video-e2e-hostile");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, `${ASSET_NAME}.mp4`);
	copyFileSync(testClip(3), out);
	return out;
}

test.describe("ui-xss", () => {
	test("hostile names and texts render as text in every view", async ({
		page,
		request,
		browserName,
	}) => {
		await page.addInitScript(() => {
			window.__csp = [];
			document.addEventListener("securitypolicyviolation", (event) => {
				window.__csp?.push(event.violatedDirective);
			});
		});

		const seeded = await seedProject(request, {
			name: PAYLOAD,
			files: [hostileAsset()],
			tracks: 1,
			perTrack: 0,
		});
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		const track = seeded.trackIds[0] as string;
		const out = (await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{ op: "track.add", args: { kind: "visual", name: PAYLOAD } },
				{
					op: "text.add",
					args: { track_id: track, start: "0s", duration: "3s", text: PAYLOAD },
				},
				{
					op: "caption.add",
					args: {
						track_id: track,
						start: "4s",
						duration: "3s",
						text: `${WORD} ${PAYLOAD}`,
					},
				},
				{
					op: "clip.add",
					args: {
						track_id: track,
						asset_id: seeded.assetId,
						start: "8s",
						source_in: "0s",
						duration: "1s",
					},
				},
				{ op: "template.set_info", args: { description: PAYLOAD } },
			],
		})) as { created: string[] };
		const clip = out.created.at(-1) as string;
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision + 1,
			operations: [
				{ op: "item.rename", args: { item_id: clip, name: PAYLOAD } },
			],
		}).catch(() => undefined);

		const shown = async (): Promise<string> =>
			(await page.locator("body").textContent()) ?? "";
		const clean = async (view: string): Promise<void> => {
			expect(await page.evaluate(() => window.__pwned), view).toBeUndefined();
			expect(
				cspViolations(
					browserName,
					await page.evaluate(() => window.__csp ?? []),
				),
				view,
			).toEqual([]);
			expect(await page.locator("body img[src='x']").count(), view).toBe(0);
		};

		await page.goto("/projects");
		await expect(page.getByText(PAYLOAD).first()).toBeVisible();
		await clean("projects page");

		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-track-id]").first()).toBeVisible();
		await expect(page.locator("[data-item-id]").first()).toBeVisible();
		let text = await shown();
		expect(text).toContain(PAYLOAD);
		expect(text).toContain(ASSET_NAME);
		await clean("editor: top bar, library, headers, timeline");

		await page.locator("[data-item-id]").first().click();
		await clean("editor: inspector, text item");
		await page.keyboard.press("Alt+3");
		await page.waitForTimeout(500);
		text = await shown();
		expect(text).toContain("onerror=window.__pwned=1");
		await clean("editor: history");

		// A caption alone on its row is wide enough to show one segment per word.
		const captions = await seedProject(request, { tracks: 1, perTrack: 0 });
		const captionHead = (await call("GET", `/projects/${captions.id}`)) as {
			revision: number;
		};
		await call("POST", `/projects/${captions.id}/edit`, {
			base_revision: captionHead.revision,
			operations: [
				{
					op: "caption.add",
					args: {
						track_id: captions.trackIds[0],
						start: "0s",
						duration: "10s",
						text: `${WORD} ${PAYLOAD}`,
					},
				},
			],
		});
		await page.goto(`/editor/${captions.id}`);
		await expect(page.locator("[data-word-index]").first()).toBeVisible();
		await expect(page.locator("[data-word-index]").first()).toHaveText(WORD);
		await clean("editor: caption words");
	});
});

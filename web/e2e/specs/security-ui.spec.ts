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

function hostileFont(): string {
	const dir = join(tmpdir(), "vukem-video-e2e-hostile");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, `${ASSET_NAME}.ttf`);
	copyFileSync(
		join(process.cwd(), "..", "renderer", "fonts", "Anton-Regular.ttf"),
		out,
	);
	return out;
}

function hostileAsset(): string {
	const dir = join(tmpdir(), "vukem-video-e2e-hostile");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, `${ASSET_NAME}.mp4`);
	copyFileSync(testClip(3), out);
	return out;
}

test.describe("security-ui", () => {
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

		// The text of the rendered elements, not the text of the body: the body also
		// holds the Inertia props as JSON in a script element, which a browser may
		// keep or drop once the page has started, so it says nothing about what is
		// shown.
		const shown = async (): Promise<string> =>
			page.evaluate(() =>
				[...document.body.querySelectorAll("*")]
					.filter(
						(el) =>
							el.tagName !== "SCRIPT" &&
							el.tagName !== "STYLE" &&
							el.children.length === 0,
					)
					.map((el) => el.textContent ?? "")
					.join(" | "),
			);
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
		// The library lists the assets after the timeline: wait for the name.
		await expect(page.getByText(ASSET_NAME).first()).toBeVisible();
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

	test("hostile text renders as text in the palette, the forms, the slots, the brand kit, the settings and the project lists", async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(120_000);
		await page.addInitScript((payload) => {
			window.__csp = [];
			document.addEventListener("securitypolicyviolation", (event) => {
				window.__csp?.push(event.violatedDirective);
			});
			// Preferences are read back from storage: hostile values must not run.
			for (const key of ["vv.timeUnit", "vv.keymap", "vv.layout"]) {
				if (localStorage.getItem(key) === null)
					localStorage.setItem(key, payload);
			}
		}, PAYLOAD);
		const blobs: string[] = [];
		page.on("request", (r) => {
			if (r.url().startsWith("blob:")) blobs.push(r.url());
		});

		const call = await caller(request);
		await call("PUT", "/brand-kit", { colors: {}, fonts: {} });
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		const added = (await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "text.add",
					args: {
						track_id: seeded.trackIds[0],
						start: "0s",
						duration: "3s",
						text: PAYLOAD,
					},
				},
			],
		})) as { created: string[] };
		const itemId = added.created[0] as string;
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision + 1,
			operations: [
				{
					op: "slot.mark",
					args: { item_id: itemId, name: "title", description: PAYLOAD },
				},
			],
		});

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
			expect(await page.locator("body img[src=x]").count(), view).toBe(0);
			expect(blobs, view).toEqual([]);
		};

		await page.goto("/settings");
		await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
		await clean("settings page");

		await page.goto("/settings/brand-kit");
		await expect(
			page.getByRole("heading", { name: "Brand kit", level: 1 }),
		).toBeVisible();
		// The server restricts colour names, so the hostile text reaches this page
		// through the file name of an imported font and through what is typed.
		await page
			.getByLabel("Font file path on the machine of the service")
			.fill(hostileFont());
		await page.getByLabel("Family name").fill("hostile");
		await page.getByRole("button", { name: "Import font" }).click();
		await expect(page.locator('[data-font-family="hostile"]')).toBeVisible();
		expect(await shown()).toContain(ASSET_NAME);
		await page.getByRole("button", { name: "Add colour" }).click();
		await page.getByLabel("Name of colour 1").fill(PAYLOAD);
		await clean("brand kit page");

		const trashed = (await call("POST", "/projects", { name: PAYLOAD })) as {
			project_id: string;
		};
		await call("POST", `/projects/${trashed.project_id}/trash`, {});
		await page.goto("/projects");
		await expect(page.getByText(PAYLOAD).first()).toBeVisible();
		await clean("projects page: list, trash and templates");

		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]").first()).toBeVisible();
		await clean("editor with hostile preferences");

		// With nothing selected the project panel shows the slots panel.
		await expect(page.locator("[data-slot-row]").first()).toBeVisible();
		const row = page.locator("[data-slot-row]").first();
		await row.getByLabel("Text", { exact: true }).fill(PAYLOAD);
		await row.getByRole("button", { name: "Fill" }).click();
		await page.waitForTimeout(500);
		await clean("editor: template slots");

		await page.keyboard.press("Control+k");
		const search = page.getByRole("combobox", { name: "Search commands" });
		await search.fill(PAYLOAD);
		await clean("palette: hostile query");
		await search.fill("text.add");
		await page.locator(`[data-command="text.add"]`).click();
		const form = page.locator(`[data-form="text.add"]`);
		await expect(form).toBeVisible();
		await form.locator(`[data-path="args.text"]`).fill(PAYLOAD);
		await clean("palette: schema form with a hostile value");
		await page.getByRole("button", { name: "Preview", exact: true }).click();
		await page.waitForTimeout(500);
		await clean("palette: dry run result");
		await page.getByRole("button", { name: "Add to batch" }).click();
		await expect(page.getByText(PAYLOAD).first()).toBeVisible();
		await clean("palette: batch tray");
	});
});

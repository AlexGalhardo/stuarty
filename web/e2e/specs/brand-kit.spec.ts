import { copyFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";
import { openInspectorTab, openSection } from "./sections";

// The tests share one brand kit per workspace, so they run one after the other.
test.describe.configure({ timeout: 120_000, mode: "serial" });

// The font of the test: a bundled file copied under a name of its own, so the
// workspace imports it by path like a font the person brought.
function brandFont(): string {
	const dir = join(tmpdir(), "stuarty-e2e-fonts");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, "Brand Anton.ttf");
	copyFileSync(
		join(process.cwd(), "..", "renderer", "fonts", "Anton-Regular.ttf"),
		out,
	);
	return out;
}

async function bitmapKeys(page: Page): Promise<string[]> {
	return page.evaluate(() =>
		performance
			.getEntriesByType("resource")
			.map((entry) => entry.name)
			.filter((name) => name.includes("/bitmaps/")),
	);
}

// P3-UI-005 (SDD-012 section 12.1, SDD-005 section 5.11): the brand kit page
// imports a font by path, keeps colours and fonts, and the editor offers them.
test.describe("brand-kit", () => {
	test("import a font, save the kit, use it in a text item and in a colour field", async ({
		page,
		request,
	}) => {
		// The kit belongs to the workspace: start empty, so that a second run on the
		// same workspace (another browser project) finds no earlier colour row.
		await (await caller(request))("PUT", "/brand-kit", {
			colors: {},
			fonts: {},
		});
		await page.goto("/settings/brand-kit");
		await expect(
			page.getByRole("heading", { name: "Brand kit", level: 1 }),
		).toBeVisible();

		await page
			.getByLabel("Font file path on the machine of the service")
			.fill(brandFont());
		await page.getByLabel("Family name").fill("Brand Anton");
		await page.getByRole("button", { name: "Import font" }).click();

		const family = page.locator("[data-font-family='Brand Anton']");
		await expect(family).toBeVisible();
		// The file name is text from the person's machine: isolated with dir=auto.
		const fileName = family.locator("[dir=auto]", {
			hasText: "Brand Anton.ttf",
		});
		await expect(fileName).toBeVisible();

		await page.getByRole("button", { name: "Add colour" }).click();
		await page.getByLabel("Name of colour 1").fill("accent");
		await page.getByLabel("Value of colour 1").fill("#ff3366");
		await page.getByRole("button", { name: "Save brand kit" }).click();
		await expect(page.getByText("Brand kit saved.")).toBeVisible();

		await page.reload();
		await expect(
			page.locator("[data-font-family='Brand Anton']"),
		).toBeVisible();
		await expect(page.getByLabel("Value of colour 1")).toHaveValue("#FF3366");

		const call = await caller(request);
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
						text: "Brand",
					},
				},
			],
		})) as { created: string[] };

		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.getByRole("main", { name: "Preview" })).toBeVisible();

		// VV-183: with the text selected, the font select says where the brand
		// kit families come from while the project does not hold them.
		await page.locator("[data-item-id]").click();
		await openInspectorTab(page, "Text");
		const hint = page.getByText(
			"The brand kit has fonts that are not in this project yet.",
		);
		await expect(hint).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(hint).toHaveCount(0);

		// The family is not in the project yet: the Fonts section offers it.
		await openSection(page, "Fonts");
		await page
			.getByRole("button", {
				name: "Use in project: brand kit font family Brand Anton",
			})
			.click();
		await expect
			.poll(() =>
				page.evaluate(
					() =>
						Object.keys(
							(window.__vv?.document as { fonts?: object } | undefined)
								?.fonts ?? {},
						).length,
				),
			)
			.toBe(1);

		await page.locator("[data-item-id]").click();
		await expect(hint).toHaveCount(0);
		await expect
			.poll(async () => (await bitmapKeys(page)).length)
			.toBeGreaterThan(0);
		const before = new Set(await bitmapKeys(page));
		await page
			.getByLabel("Font", { exact: true })
			.selectOption({ label: "Brand Anton" });
		await expect
			.poll(
				async () =>
					(await bitmapKeys(page)).filter((name) => !before.has(name)).length,
			)
			.toBeGreaterThan(0);
		await expect
			.poll(() =>
				page.evaluate((id) => {
					const doc = window.__vv?.document as {
						items: Record<string, { style?: { font_family?: string } }>;
					};
					return doc.items[id]?.style?.font_family;
				}, added.created[0]),
			)
			.toBe("Brand Anton");

		// A colour field offers the brand colours; one click is one commit.
		await expect(
			page.locator("[data-brand-color='accent']").first(),
		).toBeVisible();
	});

	// VV-117: the logo is chosen from the images of the workspace, never typed as a hash.
	test("choose the logo from the images of the workspace", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		await call("PUT", "/brand-kit", { colors: {}, fonts: {} });
		const logo = join(
			process.cwd(),
			"..",
			".config",
			"installers",
			"assets",
			"stuarty.png",
		);
		await seedProject(request, { tracks: 0, files: [logo] });

		await page.goto("/settings/brand-kit");
		const pick = page.getByLabel("Logo image from this workspace");
		await expect(pick.locator("option", { hasText: "stuarty" })).toHaveCount(1);
		await pick.selectOption({ index: 1 });
		await expect(page.getByLabel("Logo image hash")).toHaveValue(/^sha256:/);
		await page.getByRole("button", { name: "Save brand kit" }).click();
		await expect(page.getByText("Brand kit saved.")).toBeVisible();

		const kit = (await call("GET", "/brand-kit")) as { logo?: string };
		expect(kit.logo).toMatch(/^sha256:[0-9a-f]{64}$/);
		await call("PUT", "/brand-kit", { colors: {}, fonts: {} });
	});

	// VV-094: a font in the font store is chosen in the editor, never pasted as a hash.
	test("add an imported font to the project from the Fonts section", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const stored = (await call("POST", "/fonts", { path: brandFont() })) as {
			hash: string;
		};
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });

		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.getByRole("main", { name: "Preview" })).toBeVisible();
		await openSection(page, "Fonts");
		const pick = page.getByLabel("Imported font file");
		await expect(pick.locator(`option[value='${stored.hash}']`)).toHaveCount(1);
		await pick.selectOption(stored.hash);
		await page.getByLabel("Family name").fill("Stored Anton");
		await page.getByRole("button", { name: "Add font" }).click();
		await expect
			.poll(() =>
				page.evaluate(() =>
					Object.keys(
						(window.__vv?.document as { fonts?: object } | undefined)?.fonts ??
							{},
					),
				),
			)
			.toContain("Stored Anton");
	});
});

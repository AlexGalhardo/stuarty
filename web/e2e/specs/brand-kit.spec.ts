import { copyFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";

test.describe.configure({ timeout: 120_000 });

// The font of the test: a bundled file copied under a name of its own, so the
// workspace imports it by path like a font the person brought.
function brandFont(): string {
	const dir = join(tmpdir(), "vukem-video-e2e-fonts");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, "Brand Anton.ttf");
	copyFileSync(
		join(process.cwd(), "..", "engine", "raster", "fonts", "Anton-Regular.ttf"),
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
		await expect(page.getByRole("toolbar", { name: "Editing" })).toBeVisible();

		// The family is not in the project yet: the Fonts section offers it.
		await page
			.getByRole("button", {
				name: "Add brand kit font family Brand Anton to the project",
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
});

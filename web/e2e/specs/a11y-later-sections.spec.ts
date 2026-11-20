import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { unnamedControls } from "../support/keyboard";
import { caller, seedProject } from "./helpers";
import { editImage, expect, newImage, test } from "./image-support";

// Epic 41 (SDD-012 section 18.10): the sections that merged after the epic started are held to the same checklist:
// the blog (epic 23: the post panel of Write and the blog destinations of Connections), the recipes (epic 39,
// which add templates and briefs to pages already covered) and the revert, the named versions and the diff of
// the image editor (epic 50). Axe in both themes, names in the tree, and live regions that stay short.
const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];

async function axeFindings(page: Page): Promise<string[]> {
	// A dialog that is still fading in has a colour halfway to its final one, which axe reads as low contrast.
	await page.evaluate(() =>
		Promise.all(
			document
				.getAnimations()
				.filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
				.map((a) => a.finished.catch(() => undefined)),
		),
	);
	const { violations } = await new AxeBuilder({ page })
		.withTags(TAGS)
		.analyze();
	return violations.map(
		(v) =>
			`${v.id}: ${v.nodes.map((n) => `${n.target.join(" ")} ${n.html} ${n.failureSummary}`).join(" | ")}`,
	);
}

// The unit self-check of the tree helper: an unnamed control is found at any depth, with a state or a value.
test("the tree check finds a control without a name at any depth", () => {
	const tree = [
		'- region "Timeline":',
		'  - button "Save"',
		"  - button",
		"    - button [disabled]",
		"  - textbox: typed",
		'  - textbox "Name": typed',
	].join("\n");
	expect(unnamedControls(tree)).toEqual([
		"- button",
		"- button [disabled]",
		"- textbox: typed",
	]);
});

for (const theme of ["dark", "light"] as const) {
	test.describe(`a11y-later-sections ${theme}`, () => {
		test.describe.configure({ timeout: 180_000 });
		test.beforeEach(async ({ page }) => {
			await page.addInitScript((value) => {
				localStorage.setItem("theme", value);
			}, theme);
		});

		test("the post panel of Write has no violation and says only its summary in the polite region", async ({
			page,
			request,
		}) => {
			const api = await caller(request);
			const made = await api("POST", "/projects", {
				name: `A11y post ${theme} ${Date.now()}`,
			});
			await page.goto("/write");
			await page.getByRole("button", { name: "New text" }).click();
			const form = page.getByRole("form", { name: "New text" });
			await form.getByLabel("Project").selectOption(made.project_id as string);
			await form.getByLabel("Title").fill(`Post ${theme}`);
			await form.getByLabel("Kind").selectOption("post");
			await form.getByRole("button", { name: "Create" }).click();
			const panel = page.getByTestId("post-panel");
			await expect(page.getByTestId("post-issues")).toContainText("title");

			// The check runs again at every save: the region that speaks holds the summary, not the issues,
			// the images and the preview of the body.
			// One main landmark, the layout's, which the skip link targets.
			await expect(page.getByRole("main")).toHaveCount(1);
			const summary = page.getByTestId("post-summary");
			await expect(summary).toHaveAttribute("role", "status");
			await expect(panel.getByRole("status")).toHaveCount(1);
			await expect(summary.getByTestId("post-issues")).toHaveCount(0);
			await expect(summary.getByTestId("write-preview")).toHaveCount(0);

			expect(await axeFindings(page), "axe on the post panel").toEqual([]);
			expect(
				unnamedControls(await page.locator("main").ariaSnapshot()),
			).toEqual([]);
		});

		test("the blog destinations of Connections have no violation for any kind", async ({
			page,
		}) => {
			await page.goto("/connections");
			const blog = page.getByTestId("blog-section");
			await expect(blog).toBeVisible();
			const kind = blog.getByLabel("Destination", { exact: true });
			const found: string[] = [];
			for (const value of await kind
				.locator("option")
				.evaluateAll((options) =>
					options.map((o) => (o as HTMLOptionElement).value),
				)) {
				await kind.selectOption(value);
				for (const finding of await axeFindings(page))
					found.push(`${value}: ${finding}`);
			}
			expect(found, "axe on the blog form").toEqual([]);
			// The hint of a field is its description, read when the field takes the focus.
			await kind.selectOption("blog_files");
			await expect(
				blog.getByLabel("Folder", { exact: true }),
			).toHaveAccessibleDescription(/absolute path/);
		});

		test("the named versions, the diff and the revert dialog of the image editor have no violation", async ({
			page,
			request,
		}) => {
			const { id, call } = await newImage(request, {
				name: `A11y history ${theme}`,
			});
			await editImage(call, id, [
				{ op: "layer.add_text", args: { text: "HELLO", name: "Title" } },
			]);
			await page.goto(`/edit-image/${id}`);
			await expect(page.getByRole("treeitem")).toHaveCount(1);
			await page.getByTestId("image-checkpoint-name").fill("Kept");
			await page.getByTestId("image-checkpoint-add").click();
			const named = page.locator('[data-checkpoint="Kept"]');
			await expect(named).toBeVisible();
			await editImage(call, id, [
				{
					op: "layer.add_shape",
					args: { shape: "ellipse", w: 120, h: 120, name: "Circle" },
				},
			]);
			await expect(page.getByRole("treeitem")).toHaveCount(2);
			const found: string[] = [];
			const scan = async (state: string): Promise<void> => {
				for (const finding of await axeFindings(page))
					found.push(`${state}: ${finding}`);
				for (const line of unnamedControls(
					await page.locator("body").ariaSnapshot(),
				))
					found.push(`${state}: unnamed ${line}`);
			};
			await scan("history with a named version");

			await page.getByTestId("image-history-compare").first().click();
			await expect(page.getByTestId("image-diff")).toBeVisible();
			await expect(
				page.getByTestId("image-diff").getByRole("status"),
			).toHaveCount(0);
			await scan("diff");

			await named.getByTestId("image-checkpoint-revert").click();
			await expect(page.getByTestId("image-revert-preview")).toBeVisible();
			await scan("revert dialog");
			await page.keyboard.press("Escape");
			await expect(page.getByRole("alertdialog")).toHaveCount(0);
			expect(found, "violations in the image history").toEqual([]);
		});
	});
}

// The palette of the video editor (SDD-012 section 18.4): its combobox names a listbox only while one is shown.
test("the palette combobox points at its list only while the list is shown", async ({
	page,
	request,
}) => {
	const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
	await page.goto(`/editor/${seeded.id}`);
	await expect(
		page.getByRole("region", { name: "Timeline", exact: true }),
	).toBeVisible();
	await page.keyboard.press("Control+k");
	const combobox = page.getByRole("combobox");
	await expect(page.getByRole("listbox")).toBeVisible();
	const listId = await page.getByRole("listbox").getAttribute("id");
	await expect(combobox).toHaveAttribute("aria-expanded", "true");
	await expect(combobox).toHaveAttribute("aria-controls", listId ?? "");

	await combobox.fill("zzzz no such command");
	await expect(page.getByRole("listbox")).toHaveCount(0);
	await expect(combobox).toHaveAttribute("aria-expanded", "false");
	await expect(combobox).not.toHaveAttribute("aria-controls", /.*/);
	expect(await axeFindings(page), "axe on the palette with no match").toEqual(
		[],
	);
});

// The timeline while the project loads (SDD-012 section 7.1): the grid says it is busy, and its loading skeleton is
// no child of the grid that is not a row (axe aria-required-children), a state the editor scan met only by chance.
test("the timeline grid is busy while the project loads and holds only rows", async ({
	page,
	request,
}) => {
	const seeded = await seedProject(request, { tracks: 0, perTrack: 0 });
	let release: () => void = () => undefined;
	const held = new Promise<void>((resolve) => {
		release = resolve;
	});
	await page.route(
		new RegExp(String.raw`/api/v1/projects/${seeded.id}(\?.*)?$`),
		async (route) => {
			if (route.request().method() === "GET") await held;
			await route.fallback();
		},
	);
	await page.goto(`/editor/${seeded.id}`);
	const grid = page.locator("#timeline");
	await expect(grid).toHaveAttribute("aria-busy", "true");
	const { violations } = await new AxeBuilder({ page })
		.include("#timeline")
		.withRules(["aria-required-children"])
		.analyze();
	expect(violations.map((v) => v.id)).toEqual([]);
	release();
	await expect(grid).not.toHaveAttribute("aria-busy", /.*/);
});

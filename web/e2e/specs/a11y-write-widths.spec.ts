import { randomBytes } from "node:crypto";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { caller, expect, projectBody, test } from "./helpers";

// Epic 80 task 6 (SDD-012 section 18.10): the writing screens with a text open, its comparison and its end, at the
// four widths of the redesign in both themes: no sideways scroll, no text under 12 px, and no violation of axe.
// When STUARTY_SHOTS_DIR is set the page is also photographed there, for the review.
const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];
const WIDTHS = [1440, 1280, 390, 320] as const;

async function axeFindings(page: Page): Promise<string[]> {
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

// The smallest font size, in CSS pixels, of any text drawn on the page and visible to a person.
async function smallestText(page: Page): Promise<string[]> {
	return page.evaluate(() => {
		const small: string[] = [];
		const walker = document.createTreeWalker(
			document.body,
			NodeFilter.SHOW_TEXT,
		);
		for (
			let node = walker.nextNode();
			node !== null;
			node = walker.nextNode()
		) {
			const text = node.textContent?.trim() ?? "";
			const el = node.parentElement;
			if (text === "" || el === null) continue;
			if (["SCRIPT", "STYLE", "NOSCRIPT", "OPTION"].includes(el.tagName))
				continue;
			const box = el.getBoundingClientRect();
			const style = getComputedStyle(el);
			if (box.width === 0 || box.height === 0 || style.visibility === "hidden")
				continue;
			// A visually hidden text (sr-only) is clipped to one pixel and read by screen readers only.
			if (box.width <= 1 && box.height <= 1) continue;
			const size = Number.parseFloat(style.fontSize);
			if (size < 12) small.push(`${size}px: ${text.slice(0, 40)}`);
		}
		return small;
	});
}

for (const theme of ["dark", "light"] as const) {
	test.describe(`a11y-write-widths ${theme}`, () => {
		test.describe.configure({ timeout: 180_000 });
		test.use({ colorScheme: theme === "dark" ? "light" : "dark" });
		test.beforeEach(async ({ page }) => {
			await page.addInitScript((value) => {
				// Dark is the default; only light is stored (SDD-012 section 17 item 1).
				if (value === "light") localStorage.setItem("theme", "light");
			}, theme);
		});

		test("a copy with its review and its end holds at every width", async ({
			page,
			request,
			browserName,
		}) => {
			const api = await caller(request);
			const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
			const made = await api("POST", "/projects", {
				...projectBody(`Widths ${theme} ${tag}`),
				kind: "text",
			});
			const projectId = made.project_id as string;
			const created = (await api("POST", `/projects/${projectId}/texts`, {
				title: `A long title for a copy that has to wrap on a phone ${tag}`,
				text_kind: "copy",
				text: "First paragraph, kept.\n\nSecond paragraph, taken out.",
			})) as { item: { id: string } };
			const itemId = created.item.id;
			await api("PUT", `/projects/${projectId}/texts/${itemId}`, {
				text: "First paragraph, kept.\n\nSecond paragraph, put in its place with a longer sentence than before.",
				base_revision: 1,
			});

			const shots = process.env.STUARTY_SHOTS_DIR;
			if (shots) mkdirSync(shots, { recursive: true });

			for (const width of WIDTHS) {
				await page.setViewportSize({ width, height: 900 });
				await page.goto("/write");
				await page.locator(`[data-text-id="${itemId}"]`).click();
				const editor = page.getByTestId("text-editor");
				await expect(editor).toBeVisible();
				if (theme === "dark") {
					await expect(page.locator("html")).toHaveClass(/\bdark\b/);
				} else {
					await expect(page.locator("html")).not.toHaveClass(/\bdark\b/);
				}
				await expect(page.getByTestId("flow-path")).toBeVisible();
				await expect(page.getByTestId("agent-text-only")).toBeVisible();
				await page
					.getByTestId("revisions-panel")
					.getByRole("button", { name: "Compare", exact: true })
					.click();
				await expect(page.getByTestId("revision-diff")).toBeVisible();
				await editor
					.getByRole("button", { name: "Review", exact: true })
					.click();
				await expect(page.getByTestId("end-panel")).toBeVisible();

				const wide = await page.evaluate(
					() =>
						document.documentElement.scrollWidth -
						document.documentElement.clientWidth,
				);
				expect(
					wide,
					`sideways scroll at ${width}px in ${theme}`,
				).toBeLessThanOrEqual(0);
				expect(
					await smallestText(page),
					`text under 12 px at ${width}px in ${theme}`,
				).toEqual([]);
				expect(
					await axeFindings(page),
					`axe at ${width}px in ${theme}`,
				).toEqual([]);

				if (shots) {
					await page.screenshot({
						path: join(shots, `write-${theme}-${width}.png`),
						fullPage: true,
					});
				}
			}
		});

		test("the empty page and a text with no revision to compare hold at 320 px", async ({
			page,
		}) => {
			await page.setViewportSize({ width: 320, height: 640 });
			await page.goto("/write");
			await expect(
				page.getByRole("heading", { name: "Write", level: 1 }),
			).toBeVisible();
			const wide = await page.evaluate(
				() =>
					document.documentElement.scrollWidth -
					document.documentElement.clientWidth,
			);
			expect(wide).toBeLessThanOrEqual(0);
			expect(await smallestText(page)).toEqual([]);
			expect(await axeFindings(page)).toEqual([]);
		});
	});
}

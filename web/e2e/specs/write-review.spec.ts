import { randomBytes } from "node:crypto";
import type { Page } from "@playwright/test";
import { caller, chooseOption, expect, projectBody, test } from "./helpers";

// Epic 80 (SDD-019 section 11): a creator reviews a text before keeping it. Two stored revisions are compared side
// by side at desktop width and stacked at phone width, a revision is restored as a new revision, and a post is
// checked by the existing PostCheck. The paragraphs, the hunks and the checks are the core's; the page draws them.
test.describe.configure({ mode: "serial" });

const FIRST = "The first paragraph stays.\n\nThis one will be taken out.";
const SECOND =
	"The first paragraph stays.\n\nThis one was put in, in its place.";
const THIRD = `${SECOND}\n\nAnd a closing line.`;

async function openText(page: Page, itemId: string): Promise<void> {
	await page.goto("/write");
	await page.locator(`[data-text-id="${itemId}"]`).click();
	await expect(page.getByTestId("text-editor")).toBeVisible();
}

test.describe("write review", () => {
	test("two revisions side by side, then one restored as a new revision", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const made = await api("POST", "/projects", {
			...projectBody(`Review ${tag}`),
			kind: "text",
		});
		const projectId = made.project_id as string;
		const created = (await api("POST", `/projects/${projectId}/texts`, {
			title: `Copy ${tag}`,
			text_kind: "copy",
			text: FIRST,
		})) as { item: { id: string } };
		const itemId = created.item.id;
		await api("PUT", `/projects/${projectId}/texts/${itemId}`, {
			text: SECOND,
			base_revision: 1,
		});
		await api("PUT", `/projects/${projectId}/texts/${itemId}`, {
			text: THIRD,
			base_revision: 2,
		});

		await openText(page, itemId);

		// The head says the words the service counted for the saved text.
		const stored = (await api(
			"GET",
			`/projects/${projectId}/texts/${itemId}`,
		)) as { item: { words: number } };
		await expect(page.getByTestId("text-meta")).toContainText(
			`${stored.item.words} words`,
		);
		await expect(page.getByTestId("text-meta")).toContainText("revision 3");

		// Review is the primary action of a saved, unreviewed text; Save is not.
		await expect(
			page.getByRole("button", { name: "Send for review", exact: true }),
		).toHaveAttribute("data-primary", "true");
		await expect(
			page.getByRole("button", { name: "Save", exact: true }),
		).not.toHaveAttribute("data-primary", "true");

		const panel = page.getByTestId("revisions-panel");
		await chooseOption(
			page,
			panel.getByLabel("Compare revision"),
			/^Revision 1, /,
		);
		await chooseOption(
			page,
			panel.getByLabel("with revision"),
			/^Revision 3, /,
		);
		await panel.getByRole("button", { name: "Compare", exact: true }).click();
		const diff = page.getByTestId("revision-diff");
		await expect(diff).toContainText("Changes from revision 1 to 3");

		const from = diff.locator('[data-column="from"]');
		const to = diff.locator('[data-column="to"]');
		await expect(from.locator('[data-op="removed"]')).toContainText(
			"This one will be taken out.",
		);
		await expect(from.locator('[data-op="added"]')).toHaveCount(0);
		await expect(to.locator('[data-op="added"]').first()).toContainText(
			"This one was put in",
		);
		await expect(to.locator('[data-op="removed"]')).toHaveCount(0);
		// Both sides keep the paragraph that did not change.
		await expect(from.locator('[data-op="equal"]')).toContainText(
			"The first paragraph stays.",
		);
		await expect(to.locator('[data-op="equal"]')).toContainText(
			"The first paragraph stays.",
		);

		// Desktop width: side by side. Phone width: stacked.
		const wide = page.viewportSize()?.width ?? 0;
		const a = await from.boundingBox();
		const b = await to.boundingBox();
		if (a === null || b === null) throw new Error("no boxes");
		if (wide >= 768) {
			expect(Math.abs(a.y - b.y)).toBeLessThan(2);
			expect(b.x).toBeGreaterThan(a.x + a.width - 2);
		}
		await page.setViewportSize({ width: 390, height: 844 });
		const narrowA = await from.boundingBox();
		const narrowB = await to.boundingBox();
		if (narrowA === null || narrowB === null) throw new Error("no boxes");
		expect(narrowB.y).toBeGreaterThan(narrowA.y + narrowA.height - 2);
		expect(
			await page.evaluate(
				() =>
					document.documentElement.scrollWidth <=
					document.documentElement.clientWidth,
			),
		).toBe(true);
		await page.setViewportSize({ width: 1280, height: 900 });

		// Restore the first revision from the comparison: it is stored as revision 4, and 1 to 3 stay.
		await from.getByRole("button", { name: "Restore revision 1" }).click();
		await expect(page.getByTestId("save-status")).toHaveText(
			"Went back to revision 1: it is revision 4 now.",
		);
		await expect(
			page.getByTestId("text-editor").getByLabel("Text (Markdown)"),
		).toHaveValue(FIRST);
		const restored = (await api(
			"GET",
			`/projects/${projectId}/texts/${itemId}`,
		)) as {
			body: string;
			revision: { revision: number; source: string; reverted_to: number };
		};
		expect(restored.body).toBe(FIRST);
		expect(restored.revision).toMatchObject({
			revision: 4,
			source: "revert",
			reverted_to: 1,
		});
		const all = (await api(
			"GET",
			`/projects/${projectId}/texts/${itemId}/revisions`,
		)) as { revisions: { revision: number }[] };
		expect(all.revisions.map((r) => r.revision).sort()).toEqual([1, 2, 3, 4]);
		await expect(panel.locator("[data-revision]")).toHaveCount(4);
	});

	test("a text with one revision says a second is needed, and Save is primary while it has changes", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const made = await api("POST", "/projects", {
			...projectBody(`One ${tag}`),
			kind: "text",
		});
		const projectId = made.project_id as string;
		const created = (await api("POST", `/projects/${projectId}/texts`, {
			title: `One ${tag}`,
			text_kind: "copy",
			text: "Only one.",
		})) as { item: { id: string } };
		await openText(page, created.item.id);
		await expect(page.getByTestId("compare-need")).toContainText(
			"A second revision is needed",
		);
		await expect(
			page.getByRole("button", { name: "Compare", exact: true }),
		).toHaveCount(0);

		await page.getByLabel("Text (Markdown)").fill("Only one, changed.");
		await expect(
			page.getByRole("button", { name: "Save", exact: true }),
		).toHaveAttribute("data-primary", "true");
		await expect(
			page.getByRole("button", { name: "Send for review", exact: true }),
		).toBeDisabled();
	});

	test("a post is reviewed with the post check, and the check follows the saved text", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const made = await api("POST", "/projects", {
			...projectBody(`Post ${tag}`),
			kind: "text",
		});
		const projectId = made.project_id as string;
		const slug = `review-${tag}`;
		const created = (await api("POST", `/projects/${projectId}/texts`, {
			title: `Post ${tag}`,
			text_kind: "post",
			text: `---\ntitle: Review ${tag}\nslug: ${slug}\nsummary: A line.\nlanguage: en\n---\n# Body\n\nWords.\n`,
		})) as { item: { id: string } };
		await openText(page, created.item.id);
		await expect(page.getByTestId("post-panel")).toContainText(
			"Ready to publish",
		);
		await page
			.getByRole("button", { name: "Send for review", exact: true })
			.click();
		await expect(page.locator("#post-heading")).toBeFocused();
		await expect(page.getByTestId("post-slug")).toContainText(slug);
	});
});

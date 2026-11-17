import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";

// VV-173: every field of the inspector has an accessible name, the one the
// person sees, so an agent that fills fields by name edits the right one.
const INSPECTOR = "aside[aria-label='Inspector']";

async function openSections(page: Page): Promise<void> {
	const closed = page.locator(`${INSPECTOR} button[aria-expanded='false']`);
	while ((await closed.count()) > 0) {
		await closed.first().click();
	}
}

// The controls under `selector` the person can reach, with the name the browser
// computes from `aria-label`, `aria-labelledby` or an associated `label`.
async function controlNames(
	page: Page,
	selector: string,
): Promise<{ named: string[]; unnamed: string[] }> {
	return page.evaluate((root) => {
		const named: string[] = [];
		const unnamed: string[] = [];
		for (const el of document.querySelectorAll<HTMLInputElement>(
			`${root} input:not([type='hidden']), ${root} select, ${root} textarea`,
		)) {
			if (el.closest("[hidden]") !== null) continue;
			const byIds = (el.getAttribute("aria-labelledby") ?? "")
				.split(/\s+/)
				.map((id) => document.getElementById(id)?.textContent ?? "")
				.join(" ");
			const name = (
				el.getAttribute("aria-label") ??
				(byIds.trim() !== ""
					? byIds
					: Array.from(el.labels ?? [])
							.map((l) => l.textContent ?? "")
							.join(" "))
			).trim();
			if (name === "") unnamed.push(el.outerHTML.slice(0, 120));
			else named.push(name);
		}
		return { named, unnamed };
	}, selector);
}

async function check(
	page: Page,
	itemId: string,
	heading: RegExp,
): Promise<void> {
	await page.locator(`[data-item-id="${itemId}"]`).click();
	await expect(page.getByRole("heading", { name: heading })).toBeVisible();
	await openSections(page);
	const { named, unnamed } = await controlNames(page, INSPECTOR);
	expect(named.length).toBeGreaterThan(10);
	expect(unnamed).toEqual([]);
	const { violations } = await new AxeBuilder({ page })
		.include(INSPECTOR)
		.withRules([
			"label",
			"select-name",
			"aria-input-field-name",
			"label-title-only",
		])
		.analyze();
	expect(
		violations.map((v) => ({
			rule: v.id,
			nodes: v.nodes.map((n) => n.html.slice(0, 120)),
		})),
	).toEqual([]);
}

test.describe("a11y-inspector-labels", () => {
	test("the inspector fields of a clip, a text and a shape have an accessible label (VV-173)", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "text.add",
					args: { start: "0f", duration: "3s", text: "Your text" },
				},
				{
					op: "shape.add",
					args: {
						start: "0f",
						duration: "3s",
						shape: "rounded_rect",
						width: 400,
						height: 240,
					},
				},
			],
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(3);
		const ids = await items.evaluateAll((els) =>
			els.map((e) => e.getAttribute("data-item-id") ?? ""),
		);
		await expect(page.getByLabel("Path of a file to import")).toBeVisible();
		await check(page, seeded.itemIds[0] ?? "", /^Clip/);
		const others = ids.filter((id) => id !== seeded.itemIds[0]);
		const headings = [/^Text/, /^Shape/];
		for (const id of others) {
			await page.locator(`[data-item-id="${id}"]`).click();
			const isText = await page
				.getByRole("heading", { name: /^Text/ })
				.isVisible();
			await check(page, id, isText ? headings[0] : headings[1]);
		}
	});
});

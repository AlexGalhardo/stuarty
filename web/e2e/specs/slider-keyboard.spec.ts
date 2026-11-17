import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";
import { openInspectorTab } from "./sections";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const edits: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			edits.push(r.postDataJSON() as Posted);
		}
	});
	return edits;
}

// VV-182: arrow keys on a slider are a burst like a drag: one command, one
// revision, with every step in the final value.
test.describe("slider keyboard", () => {
	test("twenty arrow presses 80 ms apart are one command with all the steps", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await page.locator("[data-item-id]").click();
		await openInspectorTab(page, "Adjust");
		await page.getByRole("checkbox", { name: "Chroma key" }).check();
		const spill = page.getByRole("slider", { name: "Spill" });
		await expect(spill).toBeVisible();
		const start = Number(await spill.inputValue());
		const edits = recordEdits(page);
		await spill.focus();
		for (let i = 0; i < 20; i++) {
			await spill.press("ArrowRight");
			await page.waitForTimeout(80);
		}
		await expect.poll(() => edits.length).toBeGreaterThan(0);
		await page.waitForTimeout(1000);
		expect(edits).toHaveLength(1);
		const args = edits[0]?.operations[0]?.args as { spill?: number };
		expect(args.spill).toBeCloseTo(start + 0.2, 5);
		await expect(spill).toHaveValue(String(args.spill));
	});
});

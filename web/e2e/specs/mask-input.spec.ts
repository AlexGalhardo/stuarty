import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

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

// VV-181: the mask box is stored as fractions of the item, and the fields also
// take what a person types as a percentage; out-of-range input says the range.
test.describe("mask input", () => {
	test("50 and 50% become the fraction 0.5, a value out of range shows the range", async ({
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
		await page.getByLabel("Mask shape").selectOption("ellipse");
		const width = page.getByLabel("Mask width", { exact: true });
		await expect(width).toHaveValue("0.8");
		const edits = recordEdits(page);

		await width.fill("50");
		await width.press("Enter");
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations[0]?.args).toMatchObject({
			mask: { shape: "ellipse", width: 0.5 },
		});
		await expect(width).toHaveValue("0.5");

		await width.fill("60%");
		await width.press("Enter");
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations[0]?.args).toMatchObject({
			mask: { width: 0.6 },
		});

		await width.fill("1000");
		await width.press("Enter");
		await expect(page.getByRole("alert")).toContainText("1");
		await expect(width).toHaveAttribute("aria-invalid", "true");
		expect(edits).toHaveLength(2);
	});
});

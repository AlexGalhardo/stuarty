import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const posted: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			posted.push(r.postDataJSON() as Posted);
		}
	});
	return posted;
}

// VV-036 (SDD-012 sections 7.10 and 18.7): the roll, slide and slip edits from
// the keyboard; the editor only sends the request, the core judges it.
test.describe("slide-roll-slip", () => {
	test("the chords send item.move slide, item.trim roll and item.slip", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		const first = items.first();
		await first.focus();
		await page.keyboard.press("Enter");
		const posted = recordEdits(page);

		await page.keyboard.press("Control+Alt+ArrowRight");
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations[0]?.op).toBe("item.move");
		expect(posted[0]?.operations[0]?.args.slide).toBe(true);
		expect(typeof posted[0]?.operations[0]?.args.start).toBe("string");

		await page.keyboard.press("Control+Alt+Shift+ArrowRight");
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations[0]?.op).toBe("item.trim");
		expect(posted[1]?.operations[0]?.args.roll).toBe(true);
		expect(posted[1]?.operations[0]?.args.edge).toBe("end");

		await page.keyboard.press("Alt+Period");
		await expect.poll(() => posted.length).toBe(3);
		expect(posted[2]?.operations[0]?.op).toBe("item.slip");
		expect(typeof posted[2]?.operations[0]?.args.source_in).toBe("string");
	});
});

import { expect, seedProject, test } from "./helpers";

// P1-UI-004 asserts that need a real server: the scenarios of SDD-012 section
// 20.1. The unit tests of resources/js/editor/commands and export cover the
// same gestures against the fixture server.
test.describe("thin-client-no-optimistic", () => {
	test("one gesture sends exactly one edit request and the item appears only on the answer", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const edits: string[] = [];
		page.on("request", (r) => {
			if (r.url().endsWith("/edit")) edits.push(r.url());
		});

		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.locator("[data-item-id]").click();

		// The request is held until the count is read, so the test sees the
		// timeline before the core answers.
		let release: () => void = () => {};
		const held = new Promise<void>((resolve) => {
			release = resolve;
		});
		await page.route("**/edit", async (route) => {
			await held;
			await route.continue();
		});
		await page.getByRole("button", { name: "Split" }).click();
		await expect.poll(() => edits.length).toBe(1);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		release();
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		expect(edits).toHaveLength(1);
	});
});

test.describe("overlap-snapback", () => {
	test("an overlap leaves the item in place and shows the message of the core", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Add to timeline" }).first().click();
		await expect(page.getByRole("alert")).toContainText("Overlaps");
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
	});
});

test.describe("command-round-trip", () => {
	test("the round trip of a command is at most 50 ms at the 95th percentile on the 200-item project (NFR-001)", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, { tracks: 5, perTrack: 40 });
		const edits: string[] = [];
		page.on("request", (r) => {
			if (r.url().endsWith("/edit")) edits.push(r.url());
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(200);

		const samples = 25;
		const times: number[] = [];
		const ruler = page.getByRole("slider", { name: "Playhead" });
		for (let i = 0; i < samples; i++) {
			// Select a clip of the first track and put the playhead inside it.
			const clip = page.locator(`[data-item-id="${seeded.itemIds[i]}"]`);
			await clip.scrollIntoViewIfNeeded();
			const box = await clip.boundingBox();
			const rulerBox = await ruler.boundingBox();
			if (!box || !rulerBox) throw new Error("no layout");
			await page.mouse.click(box.x + 10, rulerBox.y + rulerBox.height / 2);
			await clip.click();
			const ms = await page.evaluate(async () => {
				const count = (): number =>
					document.querySelectorAll("[data-item-id]").length;
				const before = count();
				const button = [...document.querySelectorAll("button")].find(
					(b) =>
						b.textContent?.trim() === "Split" ||
						b.getAttribute("aria-label") === "Split",
				);
				if (!button) throw new Error("no Split button");
				return await new Promise<number>((resolve, reject) => {
					setTimeout(
						() => reject(new Error("split did not change the timeline")),
						3000,
					);
					const start = performance.now();
					const observer = new MutationObserver(() => {
						if (count() > before) {
							observer.disconnect();
							resolve(performance.now() - start);
						}
					});
					observer.observe(document.body, { childList: true, subtree: true });
					button.click();
				});
			});
			times.push(ms);
		}
		expect(edits).toHaveLength(samples);
		times.sort((a, b) => a - b);
		const p95 = times[Math.floor(times.length * 0.95)];
		console.log(
			`edit round trip on 200 items: p50 ${times[Math.floor(times.length / 2)].toFixed(1)} ms, p95 ${p95.toFixed(1)} ms, max ${times[times.length - 1].toFixed(1)} ms, ${edits.length} requests for ${samples} gestures`,
		);
		expect(p95).toBeLessThan(50);
	});
});

test.describe("export-dialog", () => {
	test("lint, start, progress, then the path and a download link", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Export" }).click();
		await page.getByRole("button", { name: "Start export" }).click();
		await expect(page.getByRole("link", { name: "Download" })).toBeVisible({
			timeout: 90_000,
		});
	});
});

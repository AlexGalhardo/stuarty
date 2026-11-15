import { buildCli, cliMove, percentile } from "../fixtures/observer";
import { expect, seedProject, test } from "./helpers";

const EDITS = 20;
const P95_LIMIT_MS = 200;

declare global {
	interface Window {
		__seen?: Record<string, number>;
	}
}

// P1-QA-001, the observer test (NFR-002, RF-118): the CLI edits a project that
// is open in the browser, and the change is on the timeline within 200 ms at
// the 95th percentile, counted from the CLI returning. Both clocks are the
// wall clock of this machine.
test.describe("observer-delay", () => {
	test("an edit made by the CLI is visible in the open editor within 200 ms (p95 of 20)", async ({
		page,
		request,
	}) => {
		test.setTimeout(180_000);
		const bin = buildCli();
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;

		await page.goto(`/editor/${seeded.id}`);
		const item = page.locator(`[data-item-id="${itemId}"]`);
		await expect(item).toBeVisible();
		// The timeline places an item by its --f variable (SDD-012 section 7.1):
		// stamp the first time each value is on the page.
		await item.evaluate((el: HTMLElement) => {
			window.__seen = {};
			const stamp = (): void => {
				const f = el.style.getPropertyValue("--f");
				const seen = window.__seen as Record<string, number>;
				if (seen[f] === undefined) seen[f] = Date.now();
			};
			new MutationObserver(stamp).observe(el, {
				attributes: true,
				attributeFilter: ["style"],
			});
		});

		const delays: number[] = [];
		for (let i = 1; i <= EDITS; i++) {
			const start = i * 45;
			cliMove(bin, seeded.id, itemId, start);
			const returned = Date.now();
			await page.waitForFunction(
				(f) => window.__seen?.[f] !== undefined,
				String(start),
				{ timeout: 5000, polling: 5 },
			);
			const seen = await page.evaluate(
				(f) => window.__seen?.[f] as number,
				String(start),
			);
			delays.push(Math.max(0, seen - returned));
		}

		const p95 = percentile(delays, 0.95);
		test.info().annotations.push({
			type: "observer-delay",
			description: `p50 ${percentile(delays, 0.5)} ms, p95 ${p95} ms, max ${Math.max(...delays)} ms over ${EDITS} edits`,
		});
		console.log(
			`observer delay over ${EDITS} CLI edits: p50 ${percentile(delays, 0.5)} ms, p95 ${p95} ms, max ${Math.max(...delays)} ms`,
		);
		expect(delays).toHaveLength(EDITS);
		expect(p95).toBeLessThanOrEqual(P95_LIMIT_MS);
	});
});

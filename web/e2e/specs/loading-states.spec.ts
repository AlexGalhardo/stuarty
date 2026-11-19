import { caller, expect, test } from "./helpers";

const SKELETON = '[data-slot="skeleton"]';

// Epic 37, polish (docs/design-system.md section 4.1, the state model): a region
// that is still waiting for the project is loading, not empty. The library and
// the timeline must not say "No media yet" or "The timeline is empty" before the
// project has answered, and they show the empty sentence once it has.
test.describe("loading states of the editor", () => {
	test("the library and the timeline show a skeleton, not an empty sentence, while the project loads", async ({
		page,
		request,
	}) => {
		// A project with nothing in it, so the sentences are the true ones once loaded.
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			name: `Loading ${Date.now()}`,
		});
		const id = created.project_id as string;
		let release: () => void = () => undefined;
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		await page.route(/\/api\/v1\/projects\/[^/?]+(\?.*)?$/, async (route) => {
			if (route.request().method() === "GET") await gate;
			await route.continue();
		});
		await page.goto(`/editor/${id}`);

		// Still loading: skeletons in both regions, no claim that they are empty.
		await expect(page.getByRole("status").getByText("Loading")).toBeVisible();
		await expect(
			page.getByRole("region", { name: "Media" }).locator(SKELETON).first(),
		).toBeVisible();
		await expect(
			page.getByRole("grid", { name: "Tracks" }).locator(SKELETON).first(),
		).toBeVisible();
		await expect(page.getByText("No media yet")).toHaveCount(0);
		await expect(page.getByText("The timeline is empty")).toHaveCount(0);

		release();
		// Loaded: the project is empty, and now the regions say so.
		await expect(page.getByText("No media yet")).toBeVisible();
		await expect(page.getByText("The timeline is empty")).toBeVisible();
		await expect(page.locator(SKELETON)).toHaveCount(0);
	});
});

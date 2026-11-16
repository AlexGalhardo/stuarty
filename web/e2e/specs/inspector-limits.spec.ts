import { expect, seedProject, test } from "./helpers";

// VV-065: a number the core would refuse is refused by the field itself, with
// the limits of the registry, and no request goes out to be answered with an
// error.
test.describe("inspector limits", () => {
	test("Crop = 1 and Opacity = 2 are refused in the field, with no request and no refused answer", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const errors: string[] = [];
		// The console line of a refused request is the browser's own and differs by
		// engine (Firefox and WebKit add font and CSP lines of their own), so the
		// check is on the answers: no request of the page is refused.
		page.on("response", (r) => {
			if (r.status() >= 400) errors.push(`${r.status()} ${r.url()}`);
		});
		const edits: string[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit"))
				edits.push(r.url());
		});
		await page.goto(`/editor/${seeded.id}`);
		await page.locator("[data-item-id]").click();

		const crop = page.getByLabel("Crop left", { exact: false }).first();
		await crop.fill("1");
		await crop.press("Enter");
		await expect(page.getByRole("alert").first()).toBeVisible();

		const opacity = page.getByLabel("Opacity", { exact: true });
		await opacity.fill("2");
		await opacity.press("Enter");
		await expect(page.getByRole("alert")).toHaveCount(2);

		expect(edits).toHaveLength(0);
		expect(errors).toEqual([]);

		// A value inside the limits still goes through.
		await crop.fill("0.25");
		await crop.press("Enter");
		await expect.poll(() => edits.length).toBe(1);
	});
});

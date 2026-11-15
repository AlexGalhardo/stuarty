import { expect, test } from "@playwright/test";

// P3-DSK-001 (CI side): the feature-detection page of desktop/detect in
// Playwright's WebKit (WebKitGTK on Linux, the WebKit nearest to WKWebView on
// macOS). The verdict is data for the author, so a `fail` verdict does not
// fail the run; only a run that cannot finish does: the page never leaves
// `running`, or the results cannot be stored.
test("the detection page finishes and stores its results", async ({ page }) => {
	test.setTimeout(120_000);
	await page.goto("/");
	await expect(page.locator("#verdict")).not.toHaveAttribute(
		"data-verdict",
		"running",
		{ timeout: 60_000 },
	);

	const box = await page.locator("#box").boundingBox();
	if (!box) throw new Error("no box");
	await page.mouse.move(box.x + 10, box.y + 10);
	await page.mouse.down();
	for (let i = 1; i <= 6; i++) {
		await page.mouse.move(box.x + 10 + i * 8, box.y + 10 + i * 3);
	}
	await page.mouse.up();
	await page.getByRole("button", { name: "Run again" }).click();
	await expect(page.locator("#verdict")).not.toHaveAttribute(
		"data-verdict",
		"running",
		{ timeout: 60_000 },
	);

	const verdict = await page.locator("#verdict").getAttribute("data-verdict");
	const failed = await page
		.locator('td[data-pass="false"]')
		.evaluateAll((cells) =>
			cells
				.map((c) => c.parentElement?.getAttribute("data-test-id"))
				.filter(Boolean),
		);
	console.log(
		`desktop-detect ${test.info().project.name}: verdict ${verdict}; not passed: ${failed.join(", ") || "none"}`,
	);
	expect(["pass", "fail"]).toContain(verdict);

	await page.getByRole("button", { name: "Send results" }).click();
	await expect(page.locator("#sent")).toContainText("Saved");
});

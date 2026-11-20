import { expect, test } from "@playwright/test";

// P2-DSK-001: the feature-detection page of desktop/detect in headless
// Chromium. The verdict is pass once the person's two steps are done: a click
// (the audio graph needs a gesture) and a drag of the box (the pointer test).
test.describe("desktop-detect", () => {
	test("the verdict is pass, with no CSP violation, no external request and no blob: URL", async ({
		page,
	}, testInfo) => {
		test.setTimeout(120_000);
		// The detection server's port is the one of the project (7979, or the
		// one derived from STUARTY_E2E_PORT), never a literal here.
		const origin = new URL(testInfo.project.use.baseURL ?? "").origin;
		const requests: string[] = [];
		page.on("request", (r) => requests.push(r.url()));
		const violations: string[] = [];
		await page.exposeFunction("seen", (v: string) => violations.push(v));
		await page.addInitScript(() => {
			document.addEventListener("securitypolicyviolation", (e) =>
				(window as unknown as { seen(v: string): void }).seen(
					e.violatedDirective,
				),
			);
		});

		await page.goto("/");
		await expect(page.locator("#verdict")).not.toHaveAttribute(
			"data-verdict",
			"running",
			{ timeout: 60_000 },
		);

		// The manual steps: drag the box, then run again (a click).
		const box = await page.locator("#box").boundingBox();
		if (!box) throw new Error("no box");
		await page.mouse.move(box.x + 10, box.y + 10);
		await page.mouse.down();
		for (let i = 1; i <= 6; i++) {
			await page.mouse.move(box.x + 10 + i * 8, box.y + 10 + i * 3);
		}
		await page.mouse.up();
		await page.getByRole("button", { name: "Run again" }).click();
		await expect(page.locator("#verdict")).toHaveAttribute(
			"data-verdict",
			"pass",
			{ timeout: 60_000 },
		);

		const failed = await page
			.locator('td[data-pass="false"]')
			.evaluateAll((cells) =>
				cells
					.map((c) => c.parentElement?.getAttribute("data-test-id"))
					.filter(Boolean),
			);
		console.log(
			`desktop-detect: not passed (reported only): ${failed.join(", ")}`,
		);

		expect(violations).toEqual([]);
		expect(requests.filter((u) => u.startsWith("blob:"))).toEqual([]);
		expect(requests.filter((u) => !u.startsWith(`${origin}/`))).toEqual([]);

		// Send the results: the server writes the file.
		await page.getByRole("button", { name: "Send results" }).click();
		await expect(page.locator("#sent")).toContainText("Saved");
	});
});

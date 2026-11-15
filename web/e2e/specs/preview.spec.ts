import { expect, seedProject, test } from "./helpers";

// P1-UI-003 asserts that need the render pipeline behind the plan and frame
// endpoints. The unit tests of resources/js/editor/preview cover the plan
// windows, the sync rules, the audio clock and truth on pause.

test.describe("csp-clean", () => {
	test("no securitypolicyviolation event and no blob: URL during playback", async ({
		page,
		request,
	}) => {
		const violations: string[] = [];
		const blobs: string[] = [];
		await page.exposeFunction("vvViolation", (v: string) => violations.push(v));
		await page.addInitScript(() => {
			document.addEventListener("securitypolicyviolation", (e) =>
				(window as unknown as { vvViolation(v: string): void }).vvViolation(
					e.violatedDirective,
				),
			);
		});
		page.on("request", (r) => {
			if (r.url().startsWith("blob:")) blobs.push(r.url());
		});
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "5s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("button", { name: "Play" }).click();
		await page.waitForTimeout(2000);
		await page.getByRole("button", { name: "Pause" }).click();
		expect(violations).toEqual([]);
		expect(blobs).toEqual([]);
	});
});

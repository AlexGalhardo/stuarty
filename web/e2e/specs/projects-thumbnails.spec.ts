import { expect, seedProject, test } from "./helpers";

// VV-238: the frame endpoint refuses a credential with more than two frames in flight (429,
// SDD-007 section 4.4). The projects page draws one picture per card, so it asks for them
// through a queue of two and only for the cards near the viewport, and the console stays clean.
const PROJECTS = 6;
const FRAME = /\/api\/v1\/projects\/prj_\w+\/frame\?/;

test("the projects page draws every picture with at most two frame requests in flight and no console error", async ({
	page,
	request,
}) => {
	for (let i = 0; i < PROJECTS; i++) {
		await seedProject(request, { name: `Thumbnails ${Date.now()}-${i}` });
	}

	const problems: string[] = [];
	page.on("console", (message) => {
		if (message.type() === "error") {
			problems.push(`${message.text()} ${message.location().url}`);
		}
	});

	let inFlight = 0;
	let peak = 0;
	let drawn = 0;
	page.on("request", (r) => {
		if (!FRAME.test(r.url())) return;
		inFlight++;
		peak = Math.max(peak, inFlight);
	});
	page.on("requestfinished", (r) => {
		if (!FRAME.test(r.url())) return;
		inFlight--;
		drawn++;
	});
	page.on("requestfailed", (r) => {
		if (FRAME.test(r.url())) inFlight--;
	});

	await page.goto("/projects");
	await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
	// The form of a project (epic 56) is taller than the viewport, and a card asks for its picture only near it.
	await page
		.locator("#project-list")
		.evaluate((el) => el.scrollIntoView({ block: "start" }));
	await expect
		.poll(() => drawn, { timeout: 30_000 })
		.toBeGreaterThanOrEqual(PROJECTS);

	expect(peak).toBeLessThanOrEqual(2);
	expect(problems).toEqual([]);
});

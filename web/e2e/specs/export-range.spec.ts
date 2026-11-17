import { exportCapabilities } from "../../resources/js/editor/export/exportCapabilities";
import { expect, seedProject, test } from "./helpers";

// TASK-0254, SDD-012 section 14: with an in or an out point on the timeline,
// the export dialog offers the range and sends it as args.start and args.end.
// The answers of the core are stubbed so the spec checks the request, not an
// encode; the schema is the real one of the server, and the spec skips when it
// does not list start and end.
test.describe("export-range", () => {
	test("the range checkbox sends the in and out points as args.start and args.end", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const schema = await page.request.get("/openapi.json");
		test.skip(
			!schema.ok() || !exportCapabilities(await schema.json()).range,
			"the server does not list start and end in job_start yet",
		);
		const bodies: { args?: Record<string, unknown>; dry_run?: boolean }[] = [];
		await page.route("**/api/v1/projects/*/jobs", async (route) => {
			if (route.request().method() !== "POST") return route.fallback();
			const body = route.request().postDataJSON();
			bodies.push(body);
			if (body.dry_run) return route.fulfill({ status: 200, json: {} });
			return route.fulfill({
				status: 202,
				json: {
					job_id: "job_stubrange",
					kind: "export",
					project_id: seeded.id,
					state: "succeeded",
					progress: 1,
					actor: "local:ui",
					created_at: "2026-11-16T12:00:00Z",
					text: "done",
					result: {
						export: {
							revision: 1_000_000,
							download_url: "/api/v1/jobs/job_stubrange/file",
							bytes: 1000,
							duration: { frame: 30, seconds: 1 },
							width: 1080,
							height: 1920,
						},
					},
				},
			});
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const ruler = page.getByRole("slider", { name: "Playhead" });
		await ruler.focus();
		await page.keyboard.press("PageUp");
		await page.keyboard.press("i");
		const inFrame = await ruler.getAttribute("aria-valuenow");
		await page.keyboard.press("PageUp");
		await page.keyboard.press("PageUp");
		await page.keyboard.press("o");
		const outFrame = await ruler.getAttribute("aria-valuenow");
		await expect(page.locator('[data-testid="ruler-range"]')).toBeVisible();

		await page.getByRole("button", { name: "Export", exact: true }).click();
		const dialog = page.getByRole("dialog");
		const range = dialog.getByLabel(
			"Export only the range between the in and out points",
		);
		await expect(range).not.toBeChecked();
		await dialog.getByRole("button", { name: "Start export" }).click();
		await expect(dialog.locator("[data-phase='succeeded']")).toBeVisible();
		const whole = bodies.filter((b) => b.dry_run !== true).at(-1);
		expect(whole?.args?.start).toBeUndefined();
		expect(whole?.args?.end).toBeUndefined();

		await page.keyboard.press("Escape");
		await page.reload();
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("slider", { name: "Playhead" }).focus();
		await page.keyboard.press("Home");
		await page.keyboard.press("PageUp");
		await page.keyboard.press("i");
		await page.keyboard.press("PageUp");
		await page.keyboard.press("PageUp");
		await page.keyboard.press("o");
		await page.getByRole("button", { name: "Export", exact: true }).click();
		const again = page.getByRole("dialog");
		await again
			.getByLabel("Export only the range between the in and out points")
			.check();
		await again.getByRole("button", { name: "Start export" }).click();
		await expect(again.locator("[data-phase='succeeded']")).toBeVisible();
		const sent = bodies.filter((b) => b.dry_run !== true).at(-1);
		expect(sent).toEqual({
			kind: "export",
			args: { start: `${inFrame}f`, end: `${outFrame}f` },
		});
	});

	test("PNG exports the frame at the playhead and the job reaches succeeded", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const schema = await page.request.get("/openapi.json");
		test.skip(
			!schema.ok() ||
				!exportCapabilities(await schema.json()).formats.includes("png"),
			"the server does not list png in job_start yet",
		);
		const bodies: { args?: Record<string, unknown>; dry_run?: boolean }[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/jobs")) {
				bodies.push(r.postDataJSON());
			}
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const ruler = page.getByRole("slider", { name: "Playhead" });
		await ruler.focus();
		await page.keyboard.press("PageUp");
		const frame = await ruler.getAttribute("aria-valuenow");

		await page.getByRole("button", { name: "Export", exact: true }).click();
		const dialog = page.getByRole("dialog");
		await dialog
			.getByLabel("Format")
			.selectOption({ label: "PNG image of the frame at the playhead" });
		await dialog.getByRole("button", { name: "Start export" }).click();
		await expect(dialog.locator("[data-phase='succeeded']")).toBeVisible({
			timeout: 90_000,
		});
		expect(bodies.filter((b) => b.dry_run !== true).at(-1)).toEqual({
			kind: "export",
			args: { format: "png", at: `${frame}f` },
		});
		await expect(
			dialog.getByRole("link", { name: "Download" }),
		).toHaveAttribute("href", /\/api\/v1\/jobs\/job_.+\/file$/);
	});
});

import { mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { exportCapabilities } from "../../resources/js/editor/export/exportCapabilities";
import { expect, seedProject, test } from "./helpers";

test.describe.configure({ timeout: 120_000 });

// What the server lists as export arguments in its OpenAPI document. The
// dialog offers only these, so a spec about a format runs only when the server
// has wired it (P3-SRV-005).
async function exposes(page: Page): Promise<boolean> {
	const response = await page.request.get("/openapi.json");
	if (!response.ok()) return false;
	return exportCapabilities(await response.json()).formats.length > 0;
}

// P3-UI-006 (SDD-012 section 14): bundles from the projects page, and the
// format option of the export dialog (frame rate and size are project settings,
// SDD-004 section 8.1).
test.describe("export-formats", () => {
	test("a bundle exported from a project imports back with the same name", async ({
		page,
		request,
	}) => {
		const name = `Bundle ${Date.now()}`;
		const seeded = await seedProject(request, { name, tracks: 1, perTrack: 1 });
		await page.goto("/projects");

		await page
			.getByRole("button", { name: `Export bundle ${seeded.id}` })
			.click();
		const result = page.locator(`[data-bundle-result='${seeded.id}']`);
		await expect(result.getByText("Bundle ready:")).toBeVisible({
			timeout: 60_000,
		});
		const link = result.getByRole("link", { name: "Download" });
		await expect(link).toHaveAttribute(
			"href",
			/\/api\/v1\/jobs\/job_.+\/file$/,
		);
		const path =
			(await result.locator(".font-mono").textContent())?.trim() ?? "";
		expect(path).toMatch(/\.vvbundle$/);

		// The service refuses an import from inside its own workspace (SDD-007
		// section 7.6): the download link is how the file leaves it.
		const file = await page.request.get(
			(await link.getAttribute("href")) as string,
		);
		expect(file.ok()).toBe(true);
		const dir = join(tmpdir(), "vukem-video-e2e-bundles");
		mkdirSync(dir, { recursive: true });
		const outside = join(dir, `${seeded.id}.vvbundle`);
		writeFileSync(outside, await file.body());

		await page
			.getByLabel("Bundle file path on the machine of the service")
			.fill(outside);
		await page.getByRole("button", { name: "Import bundle" }).click();
		await expect(page.getByRole("status")).toContainText("Imported:");
		await expect(page.getByRole("status")).toContainText(name);
		await expect(
			page.locator("li p.font-medium", { hasText: name }),
		).toHaveCount(2);
	});

	// The options are read from the schema of the server: these two tests give the
	// dialog a schema that lists the arguments and stub the answers of the core,
	// so they pass whether or not the server has wired the formats yet.
	async function withSchema(page: Page): Promise<void> {
		await page.route("**/openapi.json", async (route) => {
			const response = await route.fetch();
			const doc = (await response.json()) as {
				paths: Record<string, { post?: { requestBody?: unknown } }>;
			};
			const key = Object.keys(doc.paths).find((p) =>
				p.endsWith("/projects/{id}/jobs"),
			) as string;
			const post = doc.paths[key]?.post;
			if (post !== undefined) {
				post.requestBody = {
					content: {
						"application/json": {
							schema: {
								type: "object",
								properties: {
									args: {
										type: "object",
										properties: {
											format: { enum: ["mp4", "gif", "wav", "aac"] },
										},
									},
								},
							},
						},
					},
				};
			}
			await route.fulfill({ response, json: doc });
		});
	}

	test("the dialog sends the chosen options and shows the file link", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await withSchema(page);
		const bodies: { args?: unknown; dry_run?: boolean }[] = [];
		await page.route("**/api/v1/projects/*/jobs", async (route) => {
			if (route.request().method() !== "POST") return route.fallback();
			const body = route.request().postDataJSON();
			bodies.push(body);
			if (body.dry_run) return route.fulfill({ status: 200, json: {} });
			return route.fulfill({
				status: 202,
				json: {
					job_id: "job_stubgif",
					kind: "export",
					project_id: seeded.id,
					state: "succeeded",
					progress: 1,
					actor: "local:ui",
					created_at: "2026-11-15T12:00:00Z",
					text: "done",
					result: {
						export: {
							// At or above the project revision, or the dialog drops the result as
							// that of an older revision (VV-009).
							revision: 1_000_000,
							download_url: "/api/v1/jobs/job_stubgif/file",
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
		await page.getByRole("button", { name: "Export", exact: true }).click();
		const dialog = page.getByRole("dialog");
		await dialog.getByLabel("Format").selectOption({ label: "GIF animation" });
		await expect.poll(() => bodies.length).toBe(1);
		expect(bodies.at(-1)).toMatchObject({
			args: { format: "gif" },
			dry_run: true,
		});
		await dialog.getByRole("button", { name: "Start export" }).click();
		await expect(dialog.locator("[data-phase='succeeded']")).toBeVisible();
		expect(bodies.at(-1)).toEqual({
			kind: "export",
			args: { format: "gif" },
		});
		await expect(
			dialog.getByRole("link", { name: "Download" }),
		).toHaveAttribute("href", "/api/v1/jobs/job_stubgif/file");
	});

	test("the dialog shows the E_NO_ENCODER hint of the core for a format and blocks Start", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await withSchema(page);
		await page.route("**/api/v1/projects/*/jobs", async (route) => {
			if (route.request().method() !== "POST") return route.fallback();
			return route.fulfill({
				status: 422,
				contentType: "application/json",
				json: {
					code: "E_NO_ENCODER",
					message: "The encoder on this machine cannot encode a GIF.",
					hint: "Choose another format or install a build with another encoder.",
					retryable: false,
				},
			});
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Export", exact: true }).click();
		const dialog = page.getByRole("dialog");
		await dialog.getByLabel("Format").selectOption({ label: "GIF animation" });
		await expect(dialog.getByRole("alert")).toContainText("E_NO_ENCODER");
		await expect(dialog.getByRole("alert")).toContainText(
			"Choose another format",
		);
		await expect(
			dialog.getByRole("button", { name: "Start export" }),
		).toBeDisabled();
		await dialog.getByLabel("Format").selectOption({ label: "MP4 video" });
		await expect(dialog.getByRole("alert")).toHaveCount(0);
	});

	test("an export of GIF and an audio-only export reach succeeded with the file link", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		test.skip(
			!(await exposes(page)),
			"the server does not list the format argument of job_start yet",
		);

		for (const format of ["GIF animation", "WAV audio only"]) {
			await page.getByRole("button", { name: "Export", exact: true }).click();
			const dialog = page.getByRole("dialog");
			await dialog.getByLabel("Format").selectOption({ label: format });
			await dialog.getByRole("button", { name: "Start export" }).click();
			await expect(dialog.locator("[data-phase='succeeded']")).toBeVisible({
				timeout: 90_000,
			});
			await expect(
				dialog.getByRole("link", { name: "Download" }),
			).toHaveAttribute("href", /\/api\/v1\/jobs\/job_.+\/file$/);
			await page.keyboard.press("Escape");
			await expect(dialog).toBeHidden();
			// The next format starts from a fresh dialog.
			await page.reload();
			await expect(page.locator("[data-item-id]")).toHaveCount(1);
		}
	});
});

import type { Page, Route } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";
import { openInspectorTab } from "./sections";

const offer = (installed: boolean) => ({
	name: "face",
	files: [
		{
			name: "stuarty-aihelper.exe",
			bytes: 300_000,
			host: "github.com",
			license: "MIT",
		},
		{
			name: "onnxruntime.dll",
			bytes: 12_458_649,
			host: "api.nuget.org",
			license: "MIT",
		},
		{
			name: "face_detection_yunet_2023mar.onnx",
			bytes: 232_589,
			host: "github.com",
			license: "MIT",
		},
	],
	download_bytes: 12_991_238,
	dataset: "WIDER Face (a test sentence about the dataset)",
	published: true,
	installed,
});

const status = (installed: boolean) => ({
	installed: [],
	offers: [offer(installed)],
	state: "idle",
	done_bytes: 0,
	total_bytes: 0,
	text: "",
});

const json = (route: Route, body: unknown, statusCode = 200): Promise<void> =>
	route.fulfill({
		status: statusCode,
		contentType: "application/json",
		json: body,
	});

async function openClip(page: Page, projectId: string): Promise<void> {
	await page.goto(`/editor/${projectId}?debug=1`);
	await page.locator("[data-item-id]").first().click();
	await openInspectorTab(page, "Text");
}

// SDD-024: the face set is offered with its size, licences and the dataset of the model, and is downloaded only
// when the person presses Install; a proposal creates nothing until it is accepted.
test.describe("vertical cuts", () => {
	test("the install prompt states what it downloads and sends nothing until Install", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		let installs = 0;
		await page.route("**/api/v1/system/tools", (route) =>
			json(route, status(false)),
		);
		await page.route("**/api/v1/system/tools/install", (route) => {
			installs += 1;
			return json(
				route,
				{ code: "E_FORBIDDEN", message: "only a person can install." },
				403,
			);
		});
		await openClip(page, seeded.id);

		await page.getByRole("button", { name: "Find vertical cuts" }).click();
		const dialog = page.getByRole("dialog", { name: "Install face tracking" });
		await expect(dialog).toBeVisible();
		await expect(dialog.getByTestId("face-install-files")).toContainText(
			"face_detection_yunet_2023mar.onnx",
		);
		await expect(dialog.getByTestId("face-install-files")).toContainText(
			"licence MIT",
		);
		await expect(dialog.getByTestId("face-install-size")).toContainText(
			"13.0 MB",
		);
		await expect(dialog).toContainText("WIDER Face");
		expect(installs).toBe(0);

		await dialog.getByRole("button", { name: "Cancel" }).click();
		await expect(dialog).toBeHidden();
		expect(installs).toBe(0);

		await page.getByRole("button", { name: "Find vertical cuts" }).click();
		await page
			.getByRole("dialog")
			.getByRole("button", { name: "Install", exact: true })
			.click();
		await expect(
			page.getByLabel("Or run this command in a terminal"),
		).toHaveValue("stuarty tools:install face");
		expect(installs).toBe(1);
	});

	test("a proposal creates nothing until it is accepted, and the batch is sent as the service built it", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const batch = [
			{
				op: "track.add",
				args: { kind: "visual", id: "trk_cq00000001", name: "Cut 1" },
			},
		];
		const created: string[] = [];
		const edits: { url: string; body: { operations: unknown[] } }[] = [];

		await page.route("**/api/v1/system/tools", (route) =>
			json(route, status(true)),
		);
		await page.route("**/api/v1/projects/*/jobs**", (route) => {
			if (route.request().method() !== "POST") return route.fallback();
			return json(route, {
				job_id: "job_e2e0000001",
				kind: "face_track",
				project_id: seeded.id,
				state: "succeeded",
				progress: 1,
				actor: "local:ui",
				created_at: new Date().toISOString(),
				text: "face_track succeeded",
				result: {
					face_track: {
						asset_id: "ast_x",
						samples: 10,
						faces: 10,
						provider: "cpu",
						fps: 5,
					},
				},
			});
		});
		await page.route("**/api/v1/projects/*/cuts**", (route) =>
			json(route, {
				project_id: seeded.id,
				revision: 1,
				asset_id: "ast_x",
				text: "1 vertical cuts",
				proposals: [
					{
						n: 1,
						start_ms: 12_000,
						end_ms: 47_000,
						words: 90,
						face_share: 950,
						score: 1,
						title: "hello and welcome",
						batch,
					},
				],
			}),
		);
		await page.route("**/api/v1/projects/*/items", (route) => {
			if (route.request().method() !== "POST") return route.fallback();
			created.push(route.request().url());
			return json(route, {
				project_id: seeded.id,
				item: {
					id: "itm_e2e",
					kind: "timeline",
					title: "Cut 1",
					position: 1,
					state: "active",
					timeline_id: "prj_e2etimeline",
					updated_at: new Date().toISOString(),
				},
				text: "added",
			});
		});
		await page.route("**/api/v1/projects/*/edit?timeline_id=*", (route) => {
			edits.push({
				url: route.request().url(),
				body: route.request().postDataJSON(),
			});
			return json(route, { project_id: seeded.id, revision: 1, applied: true });
		});
		await openClip(page, seeded.id);

		await page.getByRole("button", { name: "Find vertical cuts" }).click();
		const list = page.getByRole("list", { name: "Proposed cuts" });
		await expect(list).toContainText("0:12 to 0:47");
		await expect(list).toContainText("90 words, face on screen 95%");
		await expect(list).toContainText("hello and welcome");
		expect(created).toHaveLength(0);
		expect(edits).toHaveLength(0);

		await page.getByRole("button", { name: "Create 1 timelines" }).click();
		await expect(page.getByText("1 timelines created")).toBeVisible();
		expect(created).toHaveLength(1);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.url).toContain("timeline_id=prj_e2etimeline");
		expect(edits[0]?.body.operations).toEqual(batch);
	});
});

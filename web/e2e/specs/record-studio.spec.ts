import { expect, seedProject, test } from "./helpers";

test("the recording studio captures, pauses, and opens a take in the editor", async ({
	page,
	request,
}, info) => {
	const project = await seedProject(request);
	const recordingId = "rec_0000000001";
	const assetId = "ast_0000000001";
	const base = {
		recording_id: recordingId,
		project_id: project.id,
		source: { kind: "browser" },
		fps: 30,
		duration_ms: 1400,
		bytes: 800,
		frames: 42,
		dropped: 0,
	};
	let take: Record<string, unknown> | null = null;
	let placed = false;

	await page.addInitScript(() => {
		Object.defineProperty(navigator, "mediaDevices", {
			value: { getDisplayMedia: async () => new MediaStream() },
		});
		class FakeRecorder extends EventTarget {
			static isTypeSupported() {
				return true;
			}
			state = "inactive";
			start() {
				this.state = "recording";
			}
			pause() {
				this.state = "paused";
			}
			resume() {
				this.state = "recording";
			}
			stop() {
				this.state = "inactive";
				this.dispatchEvent(new Event("stop"));
			}
		}
		Object.defineProperty(window, "MediaRecorder", { value: FakeRecorder });
	});
	await page.route("**/api/v1/recording/sources", (route) =>
		route.fulfill({
			status: 409,
			json: {
				code: "E_CAPTURE_UNAVAILABLE",
				message: "No desktop helper",
				details: { reason: "web" },
			},
		}),
	);
	await page.route("**/api/v1/recordings", (route) =>
		route.fulfill({ json: { recordings: take ? [take] : [] } }),
	);
	await page.route("**/api/v1/recordings/upload", async (route) => {
		expect(route.request().postDataJSON()).not.toHaveProperty("project_id");
		take = { ...base, state: "recording" };
		await route.fulfill({ json: take });
	});
	await page.route(
		`**/api/v1/recordings/${recordingId}/finish`,
		async (route) => {
			take = { ...base, state: "ready", review: "pending" };
			await route.fulfill({ json: take });
		},
	);
	await page.route(
		`**/api/v1/recordings/${recordingId}/confirm`,
		async (route) => {
			take = { ...base, state: "ready", review: "added", asset_id: assetId };
			await route.fulfill({ json: take });
		},
	);
	await page.route(
		`**/api/v1/recordings/${recordingId}/place`,
		async (route) => {
			placed = true;
			await route.fulfill({ json: { revision: 1 } });
		},
	);
	const documentRequest = new RegExp(
		`/api/v1/projects/${project.id}\\?detail=document`,
	);
	await page.route(documentRequest, (route) =>
		route.fulfill({
			json: {
				project_id: project.id,
				name: "Screen recording",
				revision: placed ? 1 : 0,
				document: {
					items: placed
						? { clip: { id: "clip", type: "clip", asset: assetId } }
						: {},
				},
			},
		}),
	);
	await page.route(
		`**/api/v1/projects/${project.id}/media/${assetId}/proxy`,
		(route) => route.fulfill({ status: 204 }),
	);

	await page.goto("/");
	await expect(page.getByRole("link", { name: "Record screen" })).toBeVisible();
	await page
		.getByRole("navigation", { name: "Main navigation" })
		.getByRole("link", { name: "Record", exact: true })
		.click();
	await expect(page).toHaveURL(/\/record$/);
	await expect(
		page.getByRole("heading", { name: "Record screen", level: 1 }),
	).toBeVisible();
	await expect(page.locator("[data-record-step]")).toHaveAttribute(
		"data-record-step",
		"browser",
	);
	await info.attach("record-setup", {
		body: await page.screenshot(),
		contentType: "image/png",
	});
	await page.locator("[data-record-start]").click();
	await expect(page.locator("[data-record-browser-live]")).toHaveAttribute(
		"data-record-browser-live",
		"recording",
	);
	await page.getByRole("button", { name: "Pause", exact: true }).click();
	await expect(page.locator("[data-record-browser-live]")).toHaveAttribute(
		"data-record-browser-live",
		"paused",
	);
	await page.getByRole("button", { name: "Resume", exact: true }).click();
	await page.locator("[data-record-stop]").click();
	await expect(page.locator("[data-record-step]")).toHaveAttribute(
		"data-record-step",
		"review",
	);
	await page.locator("[data-record-add]").click();
	await page.getByRole("button", { name: /place.*timeline/i }).click();
	await expect(page.getByRole("button", { name: "Export MP4" })).toBeVisible();
	await expect(
		page.getByRole("button", { name: "Apply auto-zoom" }),
	).toBeDisabled();
	await info.attach("record-polish", {
		body: await page.screenshot(),
		contentType: "image/png",
	});
	await page.unroute(documentRequest);
	await page.getByRole("link", { name: "Open in Video Editor" }).click();
	await expect(page).toHaveURL(new RegExp(`/editor/${project.id}$`));
});

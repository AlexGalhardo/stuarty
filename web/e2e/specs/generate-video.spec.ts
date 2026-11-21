import { randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import {
	caller,
	expect,
	home,
	makeProject,
	test,
	testClip,
	validName,
} from "./helpers";

// RF-361 (epic 11, SDD-016 sections 2.3, 6.4 and 10): a creator asks for a clip in Video Assets, sees the
// estimate, learns that a video always waits for a person, approves, and finds the clip in the library with its
// origin, ready for a timeline. The provider is a server of this spec on loopback that speaks the Interactions
// API of the Gemini API from its documentation and records what it receives; no real provider is called, no key
// is used and nothing is paid.
test.describe.configure({ mode: "serial", timeout: 240_000 });

const KEY = "AIza-e2e-CANARY-6c20e1";
// The price table is one per workspace, shared by the browsers of a run and by the next run, so each run lists a
// model of its own in the family the adapter knows (gemini-omni*), as the other generation specs do.
const OMNI = "gemini-omni-1.2-flash";

interface FakeGemini {
	url: string;
	keys: string[];
	submissions: Record<string, unknown>[];
	polls: number;
	close(): Promise<void>;
}

async function fakeGemini(clip: Buffer, model: string): Promise<FakeGemini> {
	const fake: FakeGemini = {
		url: "",
		keys: [],
		submissions: [],
		polls: 0,
		close: () => Promise.resolve(),
	};
	const id = `v1_e2e_${randomBytes(4).toString("hex")}`;
	const server: Server = createServer((request, response) => {
		fake.keys.push(String(request.headers["x-goog-api-key"]));
		const chunks: Buffer[] = [];
		request.on("data", (c: Buffer) => chunks.push(c));
		request.on("end", () => {
			response.setHeader("content-type", "application/json");
			if (
				request.method === "GET" &&
				request.url?.startsWith("/v1beta/models")
			) {
				response.end(
					JSON.stringify({
						models: [
							{
								name: `models/${model}`,
								displayName: "Gemini Omni 1.2 Flash",
								inputTokenLimit: 131072,
								outputTokenLimit: 65536,
							},
						],
					}),
				);
				return;
			}
			if (request.method === "POST" && request.url === "/v1beta/interactions") {
				fake.submissions.push(
					JSON.parse(Buffer.concat(chunks).toString("utf8")) as Record<
						string,
						unknown
					>,
				);
				response.end(
					JSON.stringify({ id, object: "interaction", status: "in_progress" }),
				);
				return;
			}
			if (
				request.method === "GET" &&
				request.url === `/v1beta/interactions/${id}`
			) {
				fake.polls += 1;
				// The first read says the clip is not ready, as a real generation takes time.
				if (fake.polls < 2) {
					response.end(
						JSON.stringify({
							id,
							object: "interaction",
							status: "in_progress",
						}),
					);
					return;
				}
				response.end(
					JSON.stringify({
						id,
						object: "interaction",
						status: "completed",
						steps: [
							{
								type: "model_output",
								content: [
									{
										type: "video",
										mime_type: "video/mp4",
										data: clip.toString("base64"),
									},
								],
							},
						],
					}),
				);
				return;
			}
			response.statusCode = 404;
			response.end("{}");
		});
	});
	await new Promise<void>((resolve) =>
		server.listen(0, "127.0.0.1", () => resolve()),
	);
	const { port } = server.address() as AddressInfo;
	fake.url = `http://127.0.0.1:${port}/v1beta`;
	fake.close = () => new Promise((resolve) => server.close(() => resolve()));
	return fake;
}

// A person decides in the desktop app, whose launch code carries the launcher secret (SDD-007 section 3.4).
async function signInAsPerson(page: Page): Promise<void> {
	const launch = await page.request.post("/api/v1/system/launch", {
		headers: {
			Authorization: `Bearer ${(await readFile(join(home, "token"), "utf8")).trim()}`,
			"Stuarty-Launcher": LAUNCHER_SECRET,
		},
	});
	expect(launch.ok()).toBe(true);
	const { url } = (await launch.json()) as { url: string };
	await page.goto(url);
}

async function removeConnection(page: Page, label: string): Promise<void> {
	await page.goto("/connections");
	const row = page.locator("[data-connection-id]").filter({ hasText: label });
	await row.getByRole("button", { name: "Remove" }).click();
	await expect(row).toHaveCount(0);
}

async function approve(page: Page, approvalId: string): Promise<void> {
	const other = await page.context().newPage();
	try {
		await other.goto("/approvals");
		const row = other.locator(`[data-approval-id="${approvalId}"]`);
		await row.getByRole("button", { name: "Approve" }).click();
		await expect(row).toHaveCount(0);
	} finally {
		await other.close();
	}
}

test.describe("Generate a video clip", () => {
	test("the estimate, the wait for a person, one submission, and a clip in the library that goes on a timeline", async ({
		page,
		request,
		browserName,
	}) => {
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const modelId = `${OMNI}-${tag}`;
		const fake = await fakeGemini(await readFile(testClip(0)), modelId);
		const label = `Google video ${tag}`;
		const projectName = validName(`Clips ${tag}`);
		try {
			const api = await caller(request);
			const connectionId = String(
				(
					await api("POST", "/connections", {
						kind: "google",
						label,
						endpoint: fake.url,
						accepts: ["text"],
						secret: KEY,
					})
				).id,
			);
			const projectId = String(
				(await makeProject(api, projectName)).project_id,
			);
			await signInAsPerson(page);

			// The price of a video model is per second; the product ships none.
			await page.goto("/connections");
			const connection = page
				.locator("[data-provider-connection]")
				.filter({ hasText: label });
			await connection.getByRole("button", { name: "Show the models" }).click();
			const model = connection.locator(`[data-model-id="${modelId}"]`);
			await expect(model).toContainText("No price yet");
			await model.getByRole("button", { name: "Set the price" }).click();
			await model.getByLabel("Price of one second of video").fill("0.07");
			await model.getByRole("button", { name: "Save the price" }).click();
			await expect(model).toContainText("Per second of video: $0.07");
			await expect(page.locator("body")).not.toContainText(KEY);

			await page.goto("/video-assets");
			await page.getByRole("button", { name: "Generate a video clip" }).click();
			const dialog = page.getByRole("dialog");
			await dialog.getByLabel("Project").selectOption({ label: projectName });
			await dialog.getByLabel("Connection").selectOption(connectionId);
			await expect(dialog.getByLabel("Model")).toHaveValue(modelId);
			await dialog
				.getByLabel("What the clip shows")
				.fill("A paper boat crossing a puddle at dawn");
			await dialog.getByLabel("Seconds").fill("5");
			await dialog.getByLabel("Aspect ratio").selectOption("9:16");
			await expect(dialog.getByTestId("generate-always-asks")).toContainText(
				"always waits for your approval",
			);

			await dialog.getByRole("button", { name: "Show the estimate" }).click();
			await expect(dialog.getByTestId("generate-estimate")).toContainText(
				"Estimated cost: $0.35",
			);
			// The estimate and the wait make no outbound call to the provider.
			expect(fake.submissions).toHaveLength(0);

			await dialog.getByRole("button", { name: "Generate" }).click();
			await expect(dialog.getByTestId("generate-job")).toContainText(
				"Waiting for your approval.",
			);
			expect(fake.submissions).toHaveLength(0);

			const approvals = (await api("GET", "/approvals?state=pending")) as {
				requests: { id: string; kind: string; connection_id: string }[];
			};
			const pending = approvals.requests.find(
				(a) => a.kind === "video.generate" && a.connection_id === connectionId,
			);
			expect(pending).toBeDefined();
			await approve(page, String(pending?.id));

			await expect(dialog.getByTestId("generate-job")).toContainText(
				"1 of 1 made, it cost $0.35.",
				{ timeout: 120_000 },
			);
			// Exactly one submission, with the key in the header, the model and the options asked for.
			expect(fake.submissions).toHaveLength(1);
			expect(fake.submissions[0]).toMatchObject({
				model: modelId,
				background: true,
				response_format: {
					type: "video",
					aspect_ratio: "9:16",
					duration: "5s",
				},
			});
			expect(new Set(fake.keys)).toEqual(new Set([KEY]));
			await page.keyboard.press("Escape");
			await expect(dialog).toBeHidden();

			// The library has the clip, with the origin Generated, and it goes on a timeline.
			const media = (await api("GET", `/projects/${projectId}/media`)) as {
				assets: { asset_id: string; kind: string; state: string }[];
			};
			const asset = media.assets.find((a) => a.kind === "video");
			expect(asset?.state).toBe("ready");

			await page.goto("/video-assets");
			await page.getByRole("radio", { name: "Generated" }).check();
			await page.getByRole("button", { name: "Apply" }).click();
			await expect(
				page.getByTestId("asset-grid").locator("li").first(),
			).toContainText("Generated");

			const head = (await api(
				"GET",
				`/projects/${projectId}?detail=document`,
			)) as { revision: number };
			const edited = (await api("POST", `/projects/${projectId}/edit`, {
				base_revision: head.revision,
				operations: [
					{ op: "track.add", args: { kind: "visual", name: "Video 1" } },
				],
			})) as { revision: number; effective: { created?: string[] }[] };
			const trackId = edited.effective[0]?.created?.[0];
			expect(trackId).toBeDefined();
			const placed = await api("POST", `/projects/${projectId}/edit`, {
				base_revision: edited.revision,
				operations: [
					{
						op: "clip.add",
						args: {
							track_id: trackId,
							asset_id: asset?.asset_id,
							start: "0s",
							source_in: "0s",
							duration: "2s",
						},
					},
				],
			});
			expect(placed.revision).toBeGreaterThan(edited.revision);
		} finally {
			await removeConnection(page, label);
			await fake.close();
		}
	});
});

import { randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { join } from "node:path";
import { crc32, deflateSync } from "node:zlib";
import type { Page } from "@playwright/test";

import { LAUNCHER_SECRET } from "../support/launcher";
import { caller, home, makeProject } from "./helpers";
import {
	editImage,
	expect,
	imageState,
	layerByName,
	newImage,
	seedPhoto,
	test,
} from "./image-support";

// Epic 30 (SDD-016 section 6.5, SDD-012 section 24.14): in the image editor a creator removes the background of a
// layer, and generates a new layer from a prompt, with the connected provider: the plain words that say the picture
// is sent, the estimate first, approval, and a result that is an ordinary layer one Undo takes back. The provider
// is a server of this spec on loopback that records what it receives; no real provider is called and nothing is
// paid.
test.describe.configure({ mode: "serial", timeout: 240_000 });

const KEY = "sk-e2e-CANARY-30a1b2";

interface FakeProvider {
	url: string;
	auths: string[];
	edits: { pictureBytes: number; background: string }[];
	generations: number;
	close(): Promise<void>;
}

function chunk(type: string, data: Buffer): Buffer {
	const length = Buffer.alloc(4);
	length.writeUInt32BE(data.length);
	const body = Buffer.concat([Buffer.from(type, "ascii"), data]);
	const sum = Buffer.alloc(4);
	sum.writeUInt32BE(crc32(body));
	return Buffer.concat([length, body, sum]);
}

// A real RGBA PNG of w by h whose left half is transparent: what a cut-out looks like to the ingest.
function cutout(width: number, height: number): Buffer {
	const row = Buffer.alloc(1 + width * 4);
	for (let x = 0; x < width; x += 1) {
		row[1 + x * 4] = 200;
		row[2 + x * 4] = (x * 3) % 256;
		row[3 + x * 4] = 90;
		row[4 + x * 4] = x < width / 2 ? 0 : 255;
	}
	const raw = Buffer.concat(Array.from({ length: height }, () => row));
	const header = Buffer.alloc(13);
	header.writeUInt32BE(width, 0);
	header.writeUInt32BE(height, 4);
	header[8] = 8;
	header[9] = 6;
	return Buffer.concat([
		Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
		chunk("IHDR", header),
		chunk("IDAT", deflateSync(raw)),
		chunk("IEND", Buffer.alloc(0)),
	]);
}

async function fakeProvider(model: string): Promise<FakeProvider> {
	const fake: FakeProvider = {
		url: "",
		auths: [],
		edits: [],
		generations: 0,
		close: () => Promise.resolve(),
	};
	const server: Server = createServer((request, response) => {
		fake.auths.push(String(request.headers.authorization));
		const chunks: Buffer[] = [];
		request.on("data", (c: Buffer) => chunks.push(c));
		request.on("end", () => {
			response.setHeader("content-type", "application/json");
			if (request.url === "/v1/models") {
				response.end(JSON.stringify({ object: "list", data: [{ id: model }] }));
				return;
			}
			if (request.url === "/v1/images/edits") {
				const body = JSON.parse(Buffer.concat(chunks).toString("utf8")) as {
					background: string;
					images: { image_url: string }[];
				};
				const b64 = body.images[0]?.image_url.split("base64,")[1] ?? "";
				const picture = Buffer.from(b64, "base64");
				fake.edits.push({
					pictureBytes: picture.length,
					background: body.background,
				});
				// The cut-out has the size of the picture that was sent (the IHDR is at byte 16).
				const result = cutout(
					picture.readUInt32BE(16),
					picture.readUInt32BE(20),
				);
				response.end(
					JSON.stringify({
						created: 1,
						data: [{ b64_json: result.toString("base64") }],
					}),
				);
				return;
			}
			if (request.url === "/v1/images/generations") {
				fake.generations += 1;
				response.end(
					JSON.stringify({
						created: 1,
						data: [{ b64_json: cutout(96, 64).toString("base64") }],
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
	fake.url = `http://127.0.0.1:${port}/v1`;
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

// Sets the price of an image model on the Connections page, which only a person may do.
async function setImagePrice(
	page: Page,
	label: string,
	modelId: string,
): Promise<void> {
	await page.goto("/connections");
	const connection = page
		.locator("[data-provider-connection]")
		.filter({ hasText: label });
	await connection.getByRole("button", { name: "Show the models" }).click();
	const model = connection.locator(`[data-model-id="${modelId}"]`);
	await expect(model).toContainText("No price yet");
	await model.getByRole("button", { name: "Set the price" }).click();
	await model.getByLabel("Price of one image").fill("0.04");
	await model.getByRole("button", { name: "Save the price" }).click();
	await expect(model).toContainText("Per image: $0.04");
}

// A person approves the request in the queue.
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

async function pendingApproval(
	api: Awaited<ReturnType<typeof caller>>,
	kind: string,
	connectionId: string,
): Promise<string> {
	const list = (await api("GET", "/approvals?state=pending")) as {
		requests: { id: string; kind: string; connection_id: string }[];
	};
	const found = list.requests.find(
		(a) => a.kind === kind && a.connection_id === connectionId,
	);
	expect(found).toBeDefined();
	return String(found?.id);
}

test.describe("image operations with a model", () => {
	test("remove the background of a layer and undo it; generate a new layer from a prompt", async ({
		page,
		request,
		browserName,
	}) => {
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const model = `gpt-image-${tag}`;
		const fake = await fakeProvider(model);
		const label = `OpenAI pictures ${tag}`;
		const project = `Edited ${tag}`;
		try {
			const api = await caller(request);
			const connectionId = String(
				(
					await api("POST", "/connections", {
						kind: "openai",
						label,
						endpoint: fake.url,
						accepts: ["text", "image"],
						secret: KEY,
					})
				).id,
			);
			await makeProject(api, project);
			await signInAsPerson(page);
			await setImagePrice(page, label, model);

			// An image with a photo layer that has a place of its own.
			const photo = await seedPhoto(request, 0);
			const { id, call } = await newImage(request, { name: "Model spec" });
			const before = await editImage(call, id, [
				{
					op: "layer.add_image",
					args: {
						asset: photo.hash,
						name: "Photo",
						x: 100,
						y: 60,
						w: 640,
						h: 360,
					},
				},
			]);
			const original = layerByName(before.layers, "Photo");
			expect(original?.asset).toBe(photo.hash);

			await page.goto(`/edit-image/${id}`);

			// With no layer selected the removal is not offered, and says why.
			await expect(page.getByTestId("image-model-remove")).toBeDisabled();
			await page
				.getByTestId("image-layer-row")
				.filter({ hasText: "Photo" })
				.click();
			await expect(page.getByTestId("image-model-remove")).toBeEnabled();
			await page.getByTestId("image-model-remove").click();

			const dialog = page.getByTestId("model-edit-dialog");
			await expect(dialog).toBeVisible();
			await dialog.getByLabel("Project").selectOption({ label: project });
			await dialog.getByLabel("Connection").selectOption(connectionId);
			await expect(dialog.getByLabel("Model")).toHaveValue(model);

			// The picture leaves the machine: the dialog says so, and to whom.
			await expect(dialog.getByTestId("model-edit-sent")).toContainText(
				"will be sent to OpenAI",
			);
			await expect(dialog.getByTestId("model-edit-sent")).toContainText(
				"Photo",
			);

			await dialog.getByRole("button", { name: "Show the estimate" }).click();
			await expect(dialog.getByTestId("model-edit-estimate")).toContainText(
				"Estimated cost: $0.04",
			);
			expect(fake.edits).toHaveLength(0);

			await dialog.getByRole("button", { name: "Start" }).click();
			await expect(dialog).toBeHidden();

			const strip = page.getByTestId("model-job");
			await expect(strip).toContainText("Waiting for your approval.");
			expect(fake.edits).toHaveLength(0);
			await approve(
				page,
				await pendingApproval(api, "image.edit", connectionId),
			);

			await expect(strip).toContainText("The layer is in the image", {
				timeout: 90_000,
			});
			expect(fake.edits).toHaveLength(1);
			expect(fake.edits[0]?.background).toBe("transparent");
			expect(fake.auths.at(-1)).toBe(`Bearer ${KEY}`);

			// The same layer, in the same place, with another file; the result is selected.
			const after = await imageState(call, id);
			const replaced = layerByName(after.layers, "Photo");
			expect(replaced?.id).toBe(original?.id);
			expect(replaced?.asset).not.toBe(photo.hash);
			expect([replaced?.x, replaced?.y, replaced?.w, replaced?.h]).toEqual([
				100, 60, 640, 360,
			]);
			await expect(
				page.getByTestId("image-layer-row").filter({ hasText: "Photo" }),
			).toHaveAttribute("aria-selected", "true");

			// One undo of the person who asked gives the original file back.
			await page.getByTestId("image-undo").click();
			await expect
				.poll(
					async () =>
						layerByName((await imageState(call, id)).layers, "Photo")?.asset,
				)
				.toBe(photo.hash);
			await page.getByTestId("model-job-dismiss").click();
			await expect(strip).toBeHidden();

			// Generating adds a layer that covers the one that is selected.
			await page.getByTestId("image-model-generate").click();
			await expect(dialog).toBeVisible();
			await dialog.getByLabel("Project").selectOption({ label: project });
			await dialog.getByLabel("Connection").selectOption(connectionId);
			await expect(dialog.getByTestId("model-edit-sent")).toContainText(
				"Only the text you write below will be sent to OpenAI",
			);
			await dialog
				.getByLabel("What to draw")
				.fill("A snowy forest, soft light");
			await dialog.getByRole("checkbox").check();
			await dialog.getByRole("button", { name: "Show the estimate" }).click();
			await expect(dialog.getByTestId("model-edit-estimate")).toContainText(
				"Estimated cost: $0.04",
			);
			await dialog.getByRole("button", { name: "Start" }).click();
			await expect(dialog).toBeHidden();
			await expect(strip).toContainText("Waiting for your approval.");
			expect(fake.generations).toBe(0);
			await approve(
				page,
				await pendingApproval(api, "image.edit", connectionId),
			);
			await expect(strip).toContainText("The layer is in the image", {
				timeout: 90_000,
			});
			expect(fake.generations).toBe(1);

			const grown = await imageState(call, id);
			const made = layerByName(grown.layers, "Generated");
			expect(made?.kind).toBe("image");
			expect(made?.asset).toBeDefined();
			await expect(
				page.getByTestId("image-layer-row").filter({ hasText: "Generated" }),
			).toHaveAttribute("aria-selected", "true");

			// The origin of the new file says where it came from.
			const origin = (await api(
				"GET",
				`/assets/${encodeURIComponent(String(made?.asset))}`,
			)) as { origin: { kind: string; provider: string; prompt: string } };
			expect(origin.origin.kind).toBe("generated");
			expect(origin.origin.provider).toBe("openai");
			expect(origin.origin.prompt).toBe("A snowy forest, soft light");
			await expect(page.locator("body")).not.toContainText(KEY);
		} finally {
			await removeConnection(page, label);
			await fake.close();
		}
	});
});

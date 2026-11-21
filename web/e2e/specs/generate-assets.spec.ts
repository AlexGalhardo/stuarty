import { randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { join } from "node:path";
import { crc32, deflateSync } from "node:zlib";
import type { Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { caller, expect, home, test } from "./helpers";

// RF-361 (epic 10, SDD-016 sections 6.1 and 10): a creator asks for a picture in Visual Assets and for a
// narration in Video Assets, sees the estimate before anything is submitted, approves the job, and finds
// the files in the grid with their origin. The provider is a server of this spec on loopback that records
// what it receives; no real provider is called and nothing is paid.
test.describe.configure({ mode: "serial", timeout: 180_000 });

const KEY = "sk-e2e-CANARY-6c20e1";

interface FakeProvider {
	url: string;
	auths: string[];
	images: number;
	speeches: number;
	close(): Promise<void>;
}

// A real PNG of one colour, so the ingest can probe it; the colour tells two images apart.
function png(seed: number): Buffer {
	const width = 64;
	const height = 64;
	const row = Buffer.alloc(1 + width * 3);
	for (let x = 0; x < width; x += 1) {
		row[1 + x * 3] = (seed * 37) % 256;
		row[2 + x * 3] = (seed * 91 + x) % 256;
		row[3 + x * 3] = 128;
	}
	const raw = Buffer.concat(Array.from({ length: height }, () => row));
	const chunk = (type: string, data: Buffer): Buffer => {
		const length = Buffer.alloc(4);
		length.writeUInt32BE(data.length);
		const body = Buffer.concat([Buffer.from(type, "ascii"), data]);
		const sum = Buffer.alloc(4);
		sum.writeUInt32BE(crc32(body));
		return Buffer.concat([length, body, sum]);
	};
	const header = Buffer.alloc(13);
	header.writeUInt32BE(width, 0);
	header.writeUInt32BE(height, 4);
	header[8] = 8;
	header[9] = 2;
	return Buffer.concat([
		Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
		chunk("IHDR", header),
		chunk("IDAT", deflateSync(raw)),
		chunk("IEND", Buffer.alloc(0)),
	]);
}

// One second of 8 kHz mono PCM silence: a real file the ingest can probe.
function wav(): Buffer {
	const bytes = 16_000;
	const head = Buffer.alloc(44);
	head.write("RIFF", 0);
	head.writeUInt32LE(36 + bytes, 4);
	head.write("WAVEfmt ", 8);
	head.writeUInt32LE(16, 16);
	head.writeUInt16LE(1, 20);
	head.writeUInt16LE(1, 22);
	head.writeUInt32LE(8000, 24);
	head.writeUInt32LE(16_000, 28);
	head.writeUInt16LE(2, 32);
	head.writeUInt16LE(16, 34);
	head.write("data", 36);
	head.writeUInt32LE(bytes, 40);
	return Buffer.concat([head, Buffer.alloc(bytes)]);
}

async function fakeProvider(
	imageModel: string,
	voiceModel: string,
): Promise<FakeProvider> {
	const fake: FakeProvider = {
		url: "",
		auths: [],
		images: 0,
		speeches: 0,
		close: () => Promise.resolve(),
	};
	let seed = Math.floor(Math.random() * 200);
	const server: Server = createServer((request, response) => {
		fake.auths.push(String(request.headers.authorization));
		const chunks: Buffer[] = [];
		request.on("data", (c: Buffer) => chunks.push(c));
		request.on("end", () => {
			if (request.url === "/v1/models") {
				response.setHeader("content-type", "application/json");
				response.end(
					JSON.stringify({
						object: "list",
						data: [{ id: imageModel }, { id: voiceModel }, { id: "gpt-e2e" }],
					}),
				);
				return;
			}
			if (request.url === "/v1/images/generations") {
				fake.images += 1;
				const body = JSON.parse(Buffer.concat(chunks).toString("utf8")) as {
					n: number;
				};
				const data = Array.from({ length: body.n }, () => {
					seed += 1;
					return { b64_json: png(seed).toString("base64") };
				});
				response.setHeader("content-type", "application/json");
				response.end(JSON.stringify({ created: 1, data }));
				return;
			}
			if (request.url === "/v1/audio/speech") {
				fake.speeches += 1;
				response.setHeader("content-type", "audio/wav");
				response.end(wav());
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

// Sets the price of a model on the Connections page, in the unit its kind of model has.
async function setPrice(
	page: Page,
	label: string,
	modelId: string,
	field: string,
	amount: string,
	shown: string,
): Promise<void> {
	await page.goto("/connections");
	const connection = page
		.locator("[data-provider-connection]")
		.filter({ hasText: label });
	await connection.getByRole("button", { name: "Show the models" }).click();
	const model = connection.locator(`[data-model-id="${modelId}"]`);
	await expect(model).toContainText("No price yet");
	await model.getByRole("button", { name: "Set the price" }).click();
	await model.getByLabel(field).fill(amount);
	await model.getByRole("button", { name: "Save the price" }).click();
	await expect(model).toContainText(shown);
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

test.describe("Generate images and voice", () => {
	test("an image in Visual Assets and a narration in Video Assets: the estimate first, then approval, then the files with their origin", async ({
		page,
		request,
		browserName,
	}) => {
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const imageModel = `gpt-image-${tag}`;
		const voiceModel = `tts-${tag}`;
		const fake = await fakeProvider(imageModel, voiceModel);
		const label = `OpenAI media ${tag}`;
		const project = `Generated ${tag}`;
		try {
			const api = await caller(request);
			const connectionId = String(
				(
					await api("POST", "/connections", {
						kind: "openai",
						label,
						endpoint: fake.url,
						accepts: ["text"],
						secret: KEY,
					})
				).id,
			);
			await api("POST", "/projects", { name: project });
			await signInAsPerson(page);

			// The price of an image model is per image, and of a voice model per million characters.
			await setPrice(
				page,
				label,
				imageModel,
				"Price of one image",
				"0.04",
				"Per image: $0.04",
			);
			await setPrice(
				page,
				label,
				voiceModel,
				"Price of a million characters",
				"15",
				"Per million characters: $15.00",
			);
			await expect(page.locator("body")).not.toContainText(KEY);

			// An image.
			await page.goto("/visual-assets");
			await page.getByRole("button", { name: "Generate an image" }).click();
			const dialog = page.getByRole("dialog");
			await dialog.getByLabel("Project").selectOption({ label: project });
			await dialog.getByLabel("Connection").selectOption(connectionId);
			await expect(dialog.getByLabel("Model")).toHaveValue(imageModel);
			await dialog.getByLabel("What to draw").fill("A red fox in the snow");
			await dialog.getByLabel("How many").fill("2");

			await dialog.getByRole("button", { name: "Show the estimate" }).click();
			await expect(dialog.getByTestId("generate-estimate")).toContainText(
				"Estimated cost: $0.08",
			);
			expect(fake.images).toBe(0);

			await dialog.getByRole("button", { name: "Generate" }).click();
			await expect(dialog.getByTestId("generate-job")).toContainText(
				"Waiting for your approval.",
			);
			expect(fake.images).toBe(0);

			const approvals = (await api("GET", "/approvals?state=pending")) as {
				requests: { id: string; kind: string; connection_id: string }[];
			};
			const request1 = approvals.requests.find(
				(a) => a.kind === "image.generate" && a.connection_id === connectionId,
			);
			expect(request1).toBeDefined();
			await approve(page, String(request1?.id));

			await expect(dialog.getByTestId("generate-job")).toContainText(
				"2 of 2 made, it cost $0.08.",
				{ timeout: 90_000 },
			);
			expect(fake.images).toBe(1);
			expect(fake.auths.at(-1)).toBe(`Bearer ${KEY}`);
			await page.keyboard.press("Escape");
			await expect(dialog).toBeHidden();

			// The grid shows the two files with the origin Generated.
			await page.getByRole("radio", { name: "Generated" }).check();
			await page.getByRole("button", { name: "Apply" }).click();
			await expect(
				page.getByTestId("asset-grid").locator("li").first(),
			).toContainText("Generated");

			// A narration.
			await page.goto("/sound-assets");
			await page.getByRole("button", { name: "Generate a voice-over" }).click();
			const voice = page.getByRole("dialog");
			await voice.getByLabel("Project").selectOption({ label: project });
			await voice.getByLabel("Connection").selectOption(connectionId);
			await expect(voice.getByLabel("Model")).toHaveValue(voiceModel);
			await expect(voice.getByLabel("Voice")).toHaveValue("alloy");
			await voice.getByLabel("Format").selectOption("wav");
			await voice
				.getByLabel("What to read out")
				.fill("Welcome to the channel.");
			// The provider asks that a voice be said to be AI-generated.
			await expect(voice).toContainText("AI-generated");

			await voice.getByRole("button", { name: "Show the estimate" }).click();
			await expect(voice.getByTestId("generate-estimate")).toContainText(
				"Estimated cost: $0.000345",
			);
			await voice.getByRole("button", { name: "Generate" }).click();
			await expect(voice.getByTestId("generate-job")).toContainText(
				"Waiting for your approval.",
			);
			expect(fake.speeches).toBe(0);

			const second = (await api("GET", "/approvals?state=pending")) as {
				requests: { id: string; kind: string; connection_id: string }[];
			};
			const request2 = second.requests.find(
				(a) => a.kind === "voice.generate" && a.connection_id === connectionId,
			);
			expect(request2).toBeDefined();
			await approve(page, String(request2?.id));

			await expect(voice.getByTestId("generate-job")).toContainText(
				"1 of 1 made, it cost $0.000345.",
				{ timeout: 90_000 },
			);
			expect(fake.speeches).toBe(1);
			await page.keyboard.press("Escape");
		} finally {
			await removeConnection(page, label);
			await fake.close();
		}
	});
});

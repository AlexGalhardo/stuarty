import { randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { caller, expect, home, test } from "./helpers";

// RF-360 and RF-361 (epic 09, SDD-016): a creator connects an AI provider with an API key, tests it,
// reads the models the provider lists now, sets the price of a model, asks for a text, sees the
// estimate before anything runs, approves it, and sees the text and what it cost. The provider is a
// server of this spec on loopback that records what it receives; no real provider is called.
test.describe.configure({ mode: "serial" });

const KEY = "sk-e2e-CANARY-9d41c7";
const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

interface FakeProvider {
	url: string;
	auths: string[];
	chats: number;
	close(): Promise<void>;
}

async function fakeProvider(model: string): Promise<FakeProvider> {
	const fake: FakeProvider = {
		url: "",
		auths: [],
		chats: 0,
		close: () => Promise.resolve(),
	};
	const server: Server = createServer((request, response) => {
		fake.auths.push(String(request.headers.authorization));
		response.setHeader("content-type", "application/json");
		if (request.url === "/v1/models") {
			response.end(
				JSON.stringify({
					object: "list",
					data: [{ id: model }, { id: `${model}-mini` }],
				}),
			);
			return;
		}
		if (request.url === "/v1/chat/completions") {
			fake.chats += 1;
			response.end(
				JSON.stringify({
					model,
					choices: [
						{
							message: { content: "Sourdough, from starter to slice." },
							finish_reason: "stop",
						},
					],
					usage: { prompt_tokens: 10, completion_tokens: 20 },
				}),
			);
			return;
		}
		response.statusCode = 404;
		response.end("{}");
	});
	await new Promise<void>((resolve) =>
		server.listen(0, "127.0.0.1", () => resolve()),
	);
	const { port } = server.address() as AddressInfo;
	fake.url = `http://127.0.0.1:${port}/v1`;
	fake.close = () => new Promise((resolve) => server.close(() => resolve()));
	return fake;
}

// A person decides in the desktop app, whose launch code carries the launcher secret (SDD-007
// section 3.4 item 7); the server of this suite is started with the secret of LAUNCHER_SECRET.
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

// The workspace of the suite is shared by every spec and every browser, so each test names what it
// creates after its browser, looks only at its own connection, and removes it when it ends.
async function removeConnection(page: Page, label: string): Promise<void> {
	await page.goto("/connections");
	const row = page.locator("[data-connection-id]").filter({ hasText: label });
	await row.getByRole("button", { name: "Remove" }).click();
	await expect(row).toHaveCount(0);
}

test.describe("AI providers", () => {
	test("connect with a key, test, read the models, set a price, generate, approve and see the cost", async ({
		page,
		request,
		browserName,
	}) => {
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const modelId = `gpt-${tag}`;
		const fake = await fakeProvider(modelId);
		const label = `OpenAI ${tag}`;
		try {
			await signInAsPerson(page);
			await page.goto("/connections");
			await expect(
				page.getByRole("heading", { name: "AI providers", level: 2 }),
			).toBeVisible();

			const card = page.locator('[data-provider="openai"]');
			await card.getByLabel("Name for this connection").fill(label);
			await card.getByLabel("Base address of the API").fill(fake.url);
			await card.getByLabel("API key").fill(KEY);
			await card.getByRole("button", { name: "Save the key" }).click();

			const connection = card
				.locator("[data-provider-connection]")
				.filter({ hasText: label });
			await expect(connection).toContainText(label);
			await expect(page.locator("body")).not.toContainText(KEY);
			await expect(card.getByLabel("API key")).toHaveValue("");
			await expect(connection).toContainText("Nothing spent today.");

			await connection.getByRole("button", { name: "Check the key" }).click();
			await expect(connection.locator("[data-provider-status]")).toContainText(
				"The destination answered 200.",
			);
			expect(fake.auths.at(-1)).toBe(`Bearer ${KEY}`);

			await connection.getByRole("button", { name: "Show the models" }).click();
			const model = connection.locator(`[data-model-id="${modelId}"]`);
			await expect(model).toContainText("No price yet");
			await expect(connection.locator("[data-model-id]")).toHaveCount(2);

			await model.getByRole("button", { name: "Set the price" }).click();
			await model.getByLabel("Input, per million tokens").fill("1");
			await model.getByLabel("Output, per million tokens").fill("2");
			await model.getByRole("button", { name: "Save the price" }).click();
			await expect(model).toContainText("your price");
			await expect(model).toContainText("$1.00");
			await expect(model).toContainText("$2.00");

			// The price is in the table the person edits, and no answer of the API carries the key.
			const api = await caller(request);
			const providers = (await api("GET", "/providers")) as {
				prices: { provider: string; model: string }[];
			};
			expect(providers.prices).toContainEqual(
				expect.objectContaining({ provider: "openai", model: modelId }),
			);
			expect(JSON.stringify(providers)).not.toContain(KEY);

			// An agent asks for a text: the job waits, shows its estimate, and calls nobody.
			const made = await api("POST", "/projects", { name: `Text ${tag}` });
			const connectionId = (
				(await api("GET", "/connections")) as {
					connections: { id: string; label: string }[];
				}
			).connections.find((c) => c.label === label)?.id;
			const job = (await api("POST", "/text/generate", {
				project_id: made.project_id,
				connection_id: connectionId,
				model: modelId,
				prompt: "Caption a video about sourdough.",
				max_tokens: 100,
			})) as {
				job_id: string;
				state: string;
				approval_id: string;
				estimate: { micros: number; currency: string };
			};
			expect(job.state).toBe("waiting_approval");
			expect(job.estimate.currency).toBe("USD");
			expect(job.estimate.micros).toBeGreaterThan(0);
			expect(fake.chats).toBe(0);

			// A person approves it in the queue.
			await page.goto("/approvals");
			const row = page.locator(`[data-approval-id="${job.approval_id}"]`);
			await expect(row.getByTestId("approval-summary")).toContainText(modelId);
			await row.getByRole("button", { name: "Approve" }).click();
			await expect(row).toHaveCount(0);

			await expect
				.poll(async () => (await api("GET", `/jobs/${job.job_id}`)).state)
				.toBe("succeeded");
			const done = (await api("GET", `/jobs/${job.job_id}`)) as {
				result: { text: { text: string; cost: { micros: number } } };
			};
			expect(done.result.text.text).toBe("Sourdough, from starter to slice.");
			// 10 tokens in at 1 and 20 out at 2 per million: 50 millionths.
			expect(done.result.text.cost.micros).toBe(50);
			expect(fake.chats).toBe(1);
			expect(fake.auths.at(-1)).toBe(`Bearer ${KEY}`);

			// The card shows what the connection spent today.
			await page.goto("/connections");
			await expect(
				page.locator("[data-provider-connection]").filter({ hasText: label }),
			).toContainText("Spent today: $0.00005");
		} finally {
			await removeConnection(page, label);
			await fake.close();
		}
	});

	test("the local token an agent holds can read the providers and cannot set a price", async ({
		request,
	}) => {
		const agent = await caller(request, "claude-code");
		const list = (await agent("GET", "/providers")) as {
			providers: { id: string }[];
		};
		expect(list.providers.map((p) => p.id)).toEqual([
			"anthropic",
			"openai",
			"openrouter",
		]);

		let status = 0;
		try {
			await agent("PUT", "/providers/prices", {
				provider: "openai",
				model: "gpt-e2e",
				currency: "USD",
				input_micros_per_mtok: 0,
				output_micros_per_mtok: 0,
			});
		} catch (error) {
			status = Number(/: (\d{3}) /.exec(String(error))?.[1]);
		}
		expect(status).toBe(403);
	});

	test("the label of a connection is shown as text, and the page is in Portuguese when the language is", async ({
		page,
		request,
		browserName,
	}) => {
		const fake = await fakeProvider("claude-e2e");
		const label = `${PAYLOAD} ${browserName}-${randomBytes(3).toString("hex")}`;
		try {
			await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
			await page.goto("/connections");
			await expect(
				page.getByRole("heading", { name: "Provedores de IA", level: 2 }),
			).toBeVisible();
			const card = page.locator('[data-provider="anthropic"]');
			await expect(
				card.getByRole("button", { name: "Guardar a chave" }),
			).toBeDisabled();
			await expect(card.getByLabel("Chave de API")).toBeVisible();

			const api = await caller(request);
			await api("POST", "/connections", {
				kind: "anthropic",
				label,
				endpoint: fake.url,
				accepts: ["text"],
				secret: KEY,
			});
			await page.goto("/connections");
			const connection = page
				.locator("[data-provider-connection]")
				.filter({ hasText: label });
			await expect(connection).toContainText(PAYLOAD);
			await expect(connection.locator("img")).toHaveCount(0);
			expect(await page.evaluate(() => "__pwned" in window)).toBe(false);
		} finally {
			await removeConnection(page, label);
			await fake.close();
		}
	});
});

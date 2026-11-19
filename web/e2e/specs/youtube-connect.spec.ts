import { randomBytes } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import { readFile } from "node:fs/promises";
import {
	createServer,
	type IncomingMessage,
	type Server,
	type ServerResponse,
} from "node:http";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import type { Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { caller, expect, home, test } from "./helpers";

// ADR-0035, ADR-0032, epic 18 (SDD-020): a creator connects a channel with their own developer project,
// and a finished video goes out only after a person approves it. The network is a fake of this spec on
// loopback, which the server of the suite is told about with STUARTY_TEST_SOCIAL_URL; it records what it
// receives, so the spec can prove that nothing was sent before the approval.
test.describe.configure({ mode: "serial" });

const SOCIAL_PORT = Number(process.env.STUARTY_E2E_PORT ?? 3101) + 3;
const CLIENT_SECRET = "GOCSPX-e2e-client-secret-CANARY";

interface FakeNetwork {
	received: string[];
	close(): Promise<void>;
}

function readBody(request: IncomingMessage): Promise<string> {
	return new Promise((resolve) => {
		let data = "";
		request.on("data", (chunk) => {
			data += String(chunk);
		});
		request.on("end", () => resolve(data));
	});
}

// The fake network of the suite: the consent screen redirects straight back with a code, the token and
// the account answer, and an upload is recorded.
async function fakeNetwork(): Promise<FakeNetwork> {
	const received: string[] = [];
	let uploads = 0;
	const server: Server = createServer(
		(request: IncomingMessage, response: ServerResponse) => {
			void (async () => {
				const url = new URL(
					request.url ?? "/",
					`http://127.0.0.1:${SOCIAL_PORT}`,
				);
				received.push(`${request.method} ${url.pathname}`);
				const body = await readBody(request);
				response.setHeader("Content-Type", "application/json");
				if (url.pathname === "/authorize") {
					const target = new URL(url.searchParams.get("redirect_uri") ?? "");
					target.searchParams.set("code", "e2e-code");
					target.searchParams.set("state", url.searchParams.get("state") ?? "");
					response.writeHead(302, { Location: target.toString() });
					response.end();
				} else if (url.pathname === "/token") {
					const form = new URLSearchParams(body);
					if (form.get("client_secret") !== CLIENT_SECRET) {
						response.writeHead(400);
						response.end(JSON.stringify({ error: "invalid_client" }));
						return;
					}
					response.end(
						JSON.stringify({
							access_token: "e2e-access",
							refresh_token: "e2e-refresh",
							expires_in: 3600,
						}),
					);
				} else if (url.pathname === "/account") {
					response.end(JSON.stringify({ id: "UC-E2E", title: "E2E channel" }));
				} else if (url.pathname === "/videos" && request.method === "POST") {
					uploads += 1;
					const id = `vid${uploads}`;
					response.end(
						JSON.stringify({
							id,
							url: `https://fake.example/watch/${id}`,
							state: "published",
						}),
					);
				} else {
					response.end("{}");
				}
			})();
		},
	);
	await new Promise<void>((resolve) =>
		server.listen(SOCIAL_PORT, "127.0.0.1", () => resolve()),
	);
	return {
		received,
		close: () => new Promise((resolve) => server.close(() => resolve())),
	};
}

const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

function newId(prefix: string): string {
	let out = "";
	for (const byte of randomBytes(10)) out += ALPHABET[byte % ALPHABET.length];
	return `${prefix}_${out}`;
}

// A succeeded export of the project and the file it made: the product runs the real export in other
// specs; here the video is a few bytes, since the fake network keeps none.
function seedExport(project: string): string {
	const id = newId("job");
	const dir = join(home, "exports", project);
	mkdirSync(dir, { recursive: true });
	const data = Buffer.from("e2e video bytes");
	writeFileSync(join(dir, `${id}.mp4`), data);
	const db = new DatabaseSync(join(home, "db.sqlite"));
	try {
		db.exec("PRAGMA busy_timeout = 10000");
		const now = Date.now();
		db.prepare(
			`INSERT INTO jobs (id, kind, project_id, actor, state, progress, message, input, result, created_ms, updated_ms, started_ms, finished_ms)
 VALUES (?, 'export', ?, 'local:ui', 'succeeded', 1, '', '{}', ?, ?, ?, ?, ?)`,
		).run(
			id,
			project,
			JSON.stringify({ export: { format: "mp4", bytes: data.length } }),
			now,
			now,
			now,
			now,
		);
	} finally {
		db.close();
	}
	return id;
}

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

// @chromium-only: the fake network listens on the one port the server of the suite is told about, so two
// browsers running this spec at once would collide; one engine is enough for a flow of the product.
test.describe("youtube connect and publish @chromium-only", () => {
	test("connect with your own project, publish after approval, and nothing is sent before", async ({
		page,
		request,
	}) => {
		const network = await fakeNetwork();
		try {
			await signInAsPerson(page);
			await page.goto("/connections");
			await expect(
				page.getByRole("heading", { name: "Social networks", level: 2 }),
			).toBeVisible();

			await page.getByRole("button", { name: "Set up Fake network" }).click();
			await expect(
				page.getByRole("heading", {
					name: "Connect Fake network with your own developer project",
				}),
			).toBeVisible();
			const redirect = page.locator("[data-redirect-address]");
			await expect(redirect).toHaveText(/\/social\/callback$/);
			await expect(page.locator("[data-research-note]")).toContainText(
				"2026-11-19",
			);
			expect(await page.getByLabel("Client secret").getAttribute("type")).toBe(
				"password",
			);

			await page.getByLabel("Client id").fill("e2e-client-id");
			await page.getByLabel("Client secret").fill(CLIENT_SECRET);
			await page.getByRole("button", { name: "Save and sign in" }).click();

			// The fake consent screen redirects back; the page finishes the sign-in.
			await expect(page.locator("[data-signed-in]")).toContainText(
				"E2E channel",
			);
			// The state and the code are not left in the address.
			expect(page.url()).not.toContain("code=");
			await page.getByRole("link", { name: "Back to Connections" }).click();
			const row = page.locator("[data-social-account]").first();
			await expect(row).toContainText("Signed in as");
			await expect(row).toContainText("E2E channel");
			// The secret is never shown.
			await expect(page.locator("body")).not.toContainText(CLIENT_SECRET);

			const api = await caller(request);
			const made = (await api("POST", "/projects", {
				name: "Youtube e2e",
				preset: "9:16",
			})) as { project_id: string };
			const exportJob = seedExport(made.project_id);
			const uploadsBefore = network.received.filter((r) =>
				r.includes("/videos"),
			).length;

			await row.getByRole("button", { name: "Publish now" }).click();
			const dialog = page.getByRole("dialog");
			await dialog.getByLabel("Project").selectOption(made.project_id);
			await dialog.getByLabel("Exported video").selectOption({ index: 1 });
			await dialog.getByLabel("Title", { exact: true }).fill("My first short");
			await dialog.getByLabel("No", { exact: true }).check();

			await dialog.getByRole("button", { name: "Check" }).click();
			await expect(dialog.locator("[data-checked]")).toContainText(
				"My first short",
			);

			await dialog.getByRole("button", { name: "Send for approval" }).click();
			await expect(dialog.locator("[data-waiting]")).toContainText(
				"Nothing has been sent yet",
			);
			expect(network.received.filter((r) => r.includes("/videos")).length).toBe(
				uploadsBefore,
			);

			// A person approves in the queue.
			await dialog.getByRole("link", { name: "Open Approvals" }).click();
			const item = page
				.locator("[data-approval-id]")
				.filter({ hasText: "My first short" });
			await expect(item.getByTestId("approval-summary")).toContainText(
				"My first short",
			);
			await item.getByRole("button", { name: "Approve" }).click();
			await expect(item).toHaveCount(0);

			await expect
				.poll(
					() => network.received.filter((r) => r.includes("/videos")).length,
				)
				.toBe(uploadsBefore + 1);

			const jobs = (await api("GET", `/projects/${made.project_id}/jobs`)) as {
				jobs: { job_id: string; kind: string }[];
			};
			const publication = jobs.jobs.find((j) => j.kind === "publish.upload");
			expect(publication).toBeDefined();
			await expect
				.poll(
					async () =>
						(
							(await api("GET", `/publish/${publication?.job_id}`)) as {
								state: string;
							}
						).state,
				)
				.toBe("published");
			expect(exportJob).toMatch(/^job_/);
		} finally {
			await network.close();
		}
	});

	test("the guide and the dialog speak Portuguese", async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await signInAsPerson(page);
		await page.goto("/connections");
		await expect(
			page.getByRole("heading", { name: "Redes sociais", level: 2 }),
		).toBeVisible();
		await page.getByRole("button", { name: "Configurar Fake network" }).click();
		await expect(page.locator("[data-research-note]")).toContainText(
			"As regras da rede foram lidas em",
		);
		await expect(page.getByLabel("Segredo do cliente")).toBeVisible();
	});
});

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
import { listenWhenFree, SOCIAL_PORT } from "../support/social";
import { caller, expect, home, test } from "./helpers";

// ADR-0035, ADR-0032, epic 19 (SDD-020 part 2): a creator connects Instagram and Facebook with their own Meta
// app, chooses the account each connection publishes to, and a Reel goes out only after a person approves it.
// Meta is a fake of this spec on loopback: the server of the suite is told about it with
// STUARTY_TEST_SOCIAL_URL, which makes the Instagram and Facebook connectors speak to it alone. It records
// what it receives, so the spec proves that nothing was sent before the approval.
test.describe.configure({ mode: "serial" });

const APP_SECRET = "meta-e2e-app-secret-CANARY";
const PAGE_TOKEN = "meta-e2e-page-token-CANARY";

interface FakeMeta {
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

// The Graph API as far as the flow needs it: the consent screen redirects straight back with a code, the
// token answers, the Pages and the Instagram account are listed, and a Reel is made in its steps.
async function fakeMeta(): Promise<FakeMeta> {
	const received: string[] = [];
	const self = `http://127.0.0.1:${SOCIAL_PORT}`;
	const server: Server = createServer(
		(request: IncomingMessage, response: ServerResponse) => {
			void (async () => {
				const url = new URL(request.url ?? "/", self);
				received.push(`${request.method} ${url.pathname}`);
				const body = await readBody(request);
				const form = new URLSearchParams(body);
				const path = url.pathname.replace(/^\/v27\.0/, "");
				response.setHeader("Content-Type", "application/json");
				const json = (value: unknown, status = 200): void => {
					response.writeHead(status);
					response.end(JSON.stringify(value));
				};
				if (url.pathname === "/dialog/oauth") {
					const target = new URL(url.searchParams.get("redirect_uri") ?? "");
					target.searchParams.set("code", "e2e-code");
					target.searchParams.set("state", url.searchParams.get("state") ?? "");
					response.writeHead(302, { Location: target.toString() });
					response.end();
				} else if (path === "/oauth/access_token") {
					if (form.get("client_secret") !== APP_SECRET) {
						json({ error: { message: "bad secret", code: 100 } }, 400);
					} else if (form.get("grant_type") === "fb_exchange_token") {
						json({ access_token: "long-user", expires_in: 5183944 });
					} else {
						json({ access_token: "short-user", expires_in: 3600 });
					}
				} else if (path === "/me/accounts") {
					json({
						data: [
							{
								id: "P1",
								name: "First Page",
								access_token: PAGE_TOKEN,
								instagram_business_account: { id: "IG1", username: "first_ig" },
							},
							{ id: "P2", name: "Second Page", access_token: PAGE_TOKEN },
						],
					});
				} else if (path === "/IG1" && request.method === "GET") {
					json({ id: "IG1", username: "first_ig" });
				} else if (path === "/P2" && request.method === "GET") {
					json({ id: "P2", name: "Second Page" });
				} else if (path === "/IG1/content_publishing_limit") {
					json({
						data: [{ quota_usage: 0, config: { quota_total: 100 } }],
					});
				} else if (path === "/IG1/media") {
					json({ id: "C1", uri: `${self}/ig-api-upload/v27.0/C1` });
				} else if (url.pathname.startsWith("/ig-api-upload/")) {
					json({ success: true });
				} else if (path === "/C1") {
					json({ status_code: "FINISHED", id: "C1" });
				} else if (path === "/IG1/media_publish") {
					json({ id: "M1" });
				} else if (path === "/M1") {
					json({ id: "M1", permalink: "https://instagram.example/reel/abc" });
				} else {
					json({ error: { message: "no such thing", code: 100 } }, 404);
				}
			})();
		},
	);
	await listenWhenFree(server);
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

// A succeeded export of a vertical video, with the facts the export measured on the file it wrote: the
// product runs the real export in other specs; here the file is a few bytes, since the fake keeps none.
function seedReel(project: string): string {
	const id = newId("job");
	const dir = join(home, "exports", project);
	mkdirSync(dir, { recursive: true });
	const data = Buffer.from("e2e reel bytes");
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
			JSON.stringify({
				export: {
					format: "mp4",
					bytes: data.length,
					width: 1080,
					height: 1920,
					fps: "30",
					duration: { frame: 600, seconds: 20 },
					video_codec: "h264",
					audio_codec: "aac",
				},
			}),
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

// Sign in to a network of Meta with the app of the person; the fake consent screen sends the browser back.
async function connect(page: Page, name: string): Promise<void> {
	await page.goto("/connections");
	await page.getByRole("button", { name: `Set up ${name}` }).click();
	await expect(
		page.getByRole("heading", {
			name: `Connect ${name} with your own developer project`,
		}),
	).toBeVisible();
	await page.getByLabel("Client id").fill("e2e-meta-app-id");
	await page.getByLabel("Client secret").fill(APP_SECRET);
	await page.getByRole("button", { name: "Save and sign in" }).click();
	// The sign-in reached accounts and chose none: the page says to choose them on Connections.
	await expect(page.locator("[data-needs-account]")).toBeVisible();
	// The state and the code are not left in the address.
	expect(page.url()).not.toContain("code=");
	await page.getByRole("link", { name: "Back to Connections" }).click();
}

// @chromium-only: the fake Meta listens on the one port the server of the suite is told about, so two
// browsers running this spec at once would collide; one engine is enough for a flow of the product.
test.describe("instagram and facebook connect and publish @chromium-only", () => {
	test("connect, choose the account, publish a Reel after approval, and nothing is sent before", async ({
		page,
		request,
	}) => {
		const meta = await fakeMeta();
		try {
			await signInAsPerson(page);
			await page.goto("/connections");
			await page.getByRole("button", { name: "Set up Instagram" }).click();
			await expect(
				page.getByRole("heading", {
					name: "Connect Instagram with your own developer project",
				}),
			).toBeVisible();
			await expect(page.locator("[data-redirect-address]")).toHaveText(
				/\/social\/callback$/,
			);
			const note = page.locator("[data-research-note]");
			await expect(note).toContainText("2026-11-19");
			await expect(note).toContainText("no public address");
			await expect(note).not.toContainText("Google");
			expect(await page.getByLabel("Client secret").getAttribute("type")).toBe(
				"password",
			);

			await connect(page, "Instagram");

			const row = page.locator("[data-social-account]").first();
			await expect(row).toContainText("Choose the account");
			// Nothing is chosen for the person: the account is offered, and publishing waits.
			await expect(
				row.getByRole("button", { name: /Publish now/ }),
			).toBeDisabled();
			await expect(
				row.getByRole("button", { name: "Use this account" }),
			).toBeDisabled();
			await row.getByLabel("@first_ig (First Page)").check();
			await row.getByRole("button", { name: "Use this account" }).click();
			await expect(row).toContainText("Signed in as");
			await expect(row).toContainText("@first_ig");
			// No secret and no token is ever shown.
			await expect(page.locator("body")).not.toContainText(APP_SECRET);
			await expect(page.locator("body")).not.toContainText(PAGE_TOKEN);

			const api = await caller(request);
			const made = (await api("POST", "/projects", {
				name: "Meta e2e",
				preset: "9:16",
			})) as { project_id: string };
			seedReel(made.project_id);
			const publishes = (): number =>
				meta.received.filter((r) => r.includes("/media")).length;
			const before = publishes();

			await row.getByRole("button", { name: /Publish now/ }).click();
			const dialog = page.getByRole("dialog");
			await dialog.getByLabel("Project").selectOption(made.project_id);
			await dialog.getByLabel("Exported video").selectOption({ index: 1 });
			// Instagram asks no kids question and takes no thumbnail, and Reels are public only.
			await expect(dialog.getByText("made for kids")).toHaveCount(0);
			await expect(dialog.getByLabel("Thumbnail")).toHaveCount(0);
			await expect(
				dialog.getByLabel("Who can watch").locator("option"),
			).toHaveText(["Everyone (public)"]);
			await dialog.getByLabel("Title", { exact: true }).fill("My first reel");

			await dialog.getByRole("button", { name: "Check" }).click();
			await expect(dialog.locator("[data-checked]")).toContainText(
				"My first reel",
			);

			await dialog.getByRole("button", { name: "Send for approval" }).click();
			await expect(dialog.locator("[data-waiting]")).toContainText(
				"Nothing has been sent yet",
			);
			expect(publishes()).toBe(before);

			// A person approves in the queue.
			await dialog.getByRole("link", { name: "Open Approvals" }).click();
			const item = page
				.locator("[data-approval-id]")
				.filter({ hasText: "My first reel" });
			await expect(item.getByTestId("approval-summary")).toContainText(
				"@first_ig",
			);
			await expect(item.getByTestId("approval-summary")).toContainText(
				"AI disclosure: not set",
			);
			await item.getByRole("button", { name: "Approve" }).click();
			await expect(item).toHaveCount(0);

			await expect
				.poll(() => meta.received.includes("POST /v27.0/IG1/media_publish"))
				.toBe(true);

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
			expect(meta.received).not.toContain("POST /v27.0/P1/video_reels");
		} finally {
			// Leave the workspace as it was found: the connections spec expects it to hold none.
			const api = await caller(request);
			const list = (await api("GET", "/connections")) as {
				connections: { id: string; kind: string }[];
			};
			for (const connection of list.connections.filter(
				(c) => c.kind === "instagram" || c.kind === "facebook",
			)) {
				await api("DELETE", `/connections/${connection.id}`);
			}
			await meta.close();
		}
	});

	test("a Facebook sign-in offers every Page and publishes to the one chosen", async ({
		page,
		request,
	}) => {
		const meta = await fakeMeta();
		try {
			await signInAsPerson(page);
			await connect(page, "Facebook");
			const row = page.locator("[data-social-account]").first();
			await expect(row).toContainText("Choose the account");
			await expect(row.getByLabel("First Page")).toBeVisible();
			await row.getByLabel("Second Page").check();
			await row.getByRole("button", { name: "Use this account" }).click();
			await expect(row).toContainText("Signed in as");
			await expect(row).toContainText("Second Page");
			await expect(
				row.getByRole("button", { name: /Publish now/ }),
			).toBeEnabled();
			// The connection holds the Page that was chosen and no other.
			const api = await caller(request);
			const accounts = (await api("GET", "/social/accounts")) as {
				accounts: { network: string; account_id?: string; state: string }[];
			};
			const facebook = accounts.accounts.find((a) => a.network === "facebook");
			expect(facebook?.account_id).toBe("P2");
			expect(facebook?.state).toBe("ready");
			expect(JSON.stringify(accounts)).not.toContain(PAGE_TOKEN);
		} finally {
			const api = await caller(request);
			const list = (await api("GET", "/connections")) as {
				connections: { id: string; kind: string }[];
			};
			for (const connection of list.connections.filter(
				(c) => c.kind === "instagram" || c.kind === "facebook",
			)) {
				await api("DELETE", `/connections/${connection.id}`);
			}
			await meta.close();
		}
	});

	test("the guide and the picker speak Portuguese", async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await signInAsPerson(page);
		await page.goto("/connections");
		await page.getByRole("button", { name: "Configurar Instagram" }).click();
		await expect(page.locator("[data-research-note]")).toContainText(
			"O que a Meta disse em",
		);
		await expect(page.getByLabel("Segredo do cliente")).toBeVisible();
	});
});

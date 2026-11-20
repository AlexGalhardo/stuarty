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
import type { APIRequestContext, Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { listenWhenFree, SOCIAL_PORT } from "../support/social";
import { caller, expect, home, test } from "./helpers";

// ADR-0035, ADR-0032, epic 22 (SDD-020 part 4): a creator connects X and TikTok with their own developer apps,
// sees the price of an X post and that a TikTok post is a draft before they approve either, and after the
// approval the X post is published and the TikTok one ends as a draft to finish in the TikTok app. X and TikTok
// are fakes of this spec on loopback: STUARTY_TEST_SOCIAL_URL makes the connectors speak to it alone. It records
// what it receives, so the spec proves that nothing was sent before the approval.
test.describe.configure({ mode: "serial" });

const X_SECRET = "x-e2e-client-secret-CANARY";
const TIKTOK_SECRET = "tiktok-e2e-client-secret-CANARY";
const UPLOAD_TOKEN = "tiktok-e2e-upload-token-CANARY";

interface FakeNetwork {
	received: string[];
	sent(): number;
	uploadAuth: string[];
	bodies: string[];
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

// X and TikTok as far as the flow needs them: the consent screens redirect straight back with a code, the token
// endpoints answer (and change the refresh token at every use), and a post, a media and a draft are made.
async function fakeNetwork(): Promise<FakeNetwork> {
	const received: string[] = [];
	const uploadAuth: string[] = [];
	const bodies: string[] = [];
	const self = `http://127.0.0.1:${SOCIAL_PORT}`;
	let issued = 0;
	let posts = 0;
	const server: Server = createServer(
		(request: IncomingMessage, response: ServerResponse) => {
			void (async () => {
				const url = new URL(request.url ?? "/", self);
				received.push(`${request.method} ${url.pathname}`);
				const body = await readBody(request);
				bodies.push(body);
				if (url.pathname === "/video/") {
					uploadAuth.push(String(request.headers.authorization ?? ""));
				}
				response.setHeader("Content-Type", "application/json");
				const json = (value: unknown, status = 200): void => {
					response.writeHead(status);
					response.end(JSON.stringify(value));
				};
				const redirectBack = (): void => {
					const target = new URL(url.searchParams.get("redirect_uri") ?? "");
					target.searchParams.set("code", "e2e-code");
					target.searchParams.set("state", url.searchParams.get("state") ?? "");
					response.writeHead(302, { Location: target.toString() });
					response.end();
				};
				const p = url.pathname;
				if (p === "/x/authorize" || p === "/tiktok/authorize") {
					redirectBack();
				} else if (p === "/2/oauth2/token" || p === "/v2/oauth/token/") {
					issued += 1;
					json({
						access_token: "e2e-access-CANARY",
						refresh_token: `e2e-refresh-CANARY-${issued}`,
						expires_in: 7200,
						token_type: "Bearer",
						open_id: "OPEN-E2E",
					});
				} else if (p === "/2/users/me") {
					json({ data: { id: "x-e2e", name: "X E2E", username: "xe2e" } });
				} else if (p === "/2/media/upload/initialize") {
					json({ data: { id: "m1", expires_after_secs: 86400 } });
				} else if (p.startsWith("/2/media/upload/m1/")) {
					json({ data: { id: "m1", expires_at: 1795190400 } });
				} else if (p === "/2/tweets") {
					posts += 1;
					response.writeHead(201);
					response.end(
						JSON.stringify({ data: { id: `post${posts}`, text: "x" } }),
					);
				} else if (p === "/v2/user/info/") {
					json({
						data: { user: { open_id: "OPEN-E2E", display_name: "TikTok E2E" } },
						error: { code: "ok", message: "", log_id: "1" },
					});
				} else if (p === "/v2/post/publish/inbox/video/init/") {
					json({
						data: {
							publish_id: "v_inbox_file~v2.e2e",
							upload_url: `${self}/video/?upload_id=1&upload_token=${UPLOAD_TOKEN}`,
						},
						error: { code: "ok", message: "", log_id: "1" },
					});
				} else if (p === "/video/") {
					response.writeHead(201);
					response.end();
				} else if (p === "/v2/post/publish/status/fetch/") {
					json({
						data: { status: "SEND_TO_USER_INBOX", uploaded_bytes: 16 },
						error: { code: "ok", message: "", log_id: "1" },
					});
				} else {
					json({ error: "no such thing" }, 404);
				}
			})();
		},
	);
	await listenWhenFree(server);
	const sends = [
		"POST /2/tweets",
		"POST /2/media/upload/initialize",
		"PUT /video/",
		"POST /v2/post/publish/inbox/video/init/",
	];
	return {
		received,
		uploadAuth,
		bodies,
		sent: () => received.filter((r) => sends.includes(r)).length,
		close: () => new Promise((resolve) => server.close(() => resolve())),
	};
}

const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

function newId(prefix: string): string {
	let out = "";
	for (const byte of randomBytes(10)) out += ALPHABET[byte % ALPHABET.length];
	return `${prefix}_${out}`;
}

// A succeeded export of a vertical clip, with the facts the export measured on the file it wrote.
function seedClip(project: string): string {
	const id = newId("job");
	const dir = join(home, "exports", project);
	mkdirSync(dir, { recursive: true });
	const data = Buffer.from("e2e clip bytes!!");
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
					width: 720,
					height: 1280,
					fps: "30",
					duration: { frame: 900, seconds: 30 },
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

// Sign in to a network with the app of the person; the fake consent screen sends the browser back.
async function connect(
	page: Page,
	name: string,
	secret: string,
	title: string,
): Promise<void> {
	await page.goto("/connections");
	await page.getByRole("button", { name: `Set up ${name}` }).click();
	await expect(
		page.getByRole("heading", {
			name: `Connect ${name} with your own developer project`,
		}),
	).toBeVisible();
	await page.getByLabel("Client id").fill("e2e-client-id");
	await page.getByLabel("Client secret").fill(secret);
	await page.getByRole("button", { name: "Save and sign in" }).click();
	await expect(page.locator("[data-signed-in]")).toContainText(title);
	expect(page.url()).not.toContain("code=");
}

interface Target {
	id: string;
	network: string;
	state: string;
	url?: string;
	remote_state?: string;
}

interface Publication {
	id: string;
	targets: Target[];
}

async function publicationOf(
	api: Awaited<ReturnType<typeof caller>>,
	id: string,
): Promise<Publication> {
	return (await api("GET", `/publications/${id}`)) as Publication;
}

async function cleanUp(
	request: APIRequestContext,
	network: FakeNetwork,
): Promise<void> {
	const api = await caller(request);
	const list = (await api("GET", "/connections")) as {
		connections: { id: string; kind: string }[];
	};
	for (const connection of list.connections.filter(
		(c) => c.kind === "x" || c.kind === "tiktok",
	)) {
		await api("DELETE", `/connections/${connection.id}`);
	}
	await network.close();
}

// @chromium-only: the fake listens on the one port the server of the suite is told about; one engine is
// enough for a flow of the product.
test.describe("x and tiktok @chromium-only", () => {
	test("the price and the draft are shown before the approval, and each ends as its network allows", async ({
		page,
		request,
	}) => {
		test.setTimeout(240_000);
		const network = await fakeNetwork();
		try {
			await signInAsPerson(page);

			// The guide says what each network allows before anything is connected.
			await page.goto("/connections");
			await page.getByRole("button", { name: "Set up TikTok" }).click();
			await expect(page.locator("[data-research-note]")).toContainText(
				"a draft",
			);
			await expect(page.locator("[data-research-note]")).toContainText(
				"no public post",
			);
			await page.getByRole("button", { name: "Set up X" }).click();
			await expect(page.locator("[data-research-note]")).toContainText(
				"$0.200",
			);

			await connect(page, "X", X_SECRET, "@xe2e");
			await connect(page, "TikTok", TIKTOK_SECRET, "TikTok E2E");

			// The price of a post is on the account, with its date, and a person can correct it.
			await page.goto("/connections");
			await expect(page.getByTestId("account-price")).toContainText(
				"price of 2026-11-20",
			);
			await page.getByLabel(/^A post \(USD\)$/).fill("0.02");
			await page.getByRole("button", { name: "Save the prices" }).click();
			await expect(page.getByTestId("account-price")).toContainText("$0.02");
			await page.getByLabel(/^A post \(USD\)$/).fill("0.015");
			await page.getByRole("button", { name: "Save the prices" }).click();
			await expect(page.getByTestId("account-price")).toContainText("$0.015");

			const api = await caller(request);
			const made = (await api("POST", "/projects", {
				name: "X and TikTok e2e",
				preset: "9:16",
			})) as { project_id: string };
			const exportJob = seedClip(made.project_id);
			const accounts = (await api("GET", "/social/accounts")) as {
				accounts: { connection_id: string; network: string }[];
			};
			const xConn = accounts.accounts.find((a) => a.network === "x");
			const tiktokConn = accounts.accounts.find((a) => a.network === "tiktok");
			expect(xConn).toBeDefined();
			expect(tiktokConn).toBeDefined();
			const before = network.received.length;

			const created = (await api("POST", "/publications", {
				project_id: made.project_id,
				title: "Both networks",
				text: "Watch it on https://example.com/clip",
				export: exportJob,
				targets: [
					{
						connection_id: xConn?.connection_id,
						fields: { visibility: "public" },
					},
					{ connection_id: tiktokConn?.connection_id, fields: {} },
				],
			})) as Publication;
			for (const target of created.targets) {
				await api("POST", `/publications/${created.id}/send-now`, {
					target_id: target.id,
				});
			}

			// Both wait for a person: nothing was sent, and the approval says what each costs and is.
			expect(network.sent()).toBe(0);
			await page.goto("/approvals");
			const xItem = page
				.locator("[data-approval-id]")
				.filter({ hasText: "to X," });
			const tiktokItem = page
				.locator("[data-approval-id]")
				.filter({ hasText: "to TikTok," });
			await expect(xItem.getByTestId("approval-summary")).toContainText(
				"cost: 0.20 USD (a post with a link, price of",
			);
			await expect(xItem.getByTestId("approval-cost")).toContainText("$0.20");
			await expect(tiktokItem.getByTestId("approval-summary")).toContainText(
				"a draft in the TikTok app: the title, text and tags are not sent, you finish it there",
			);
			await expect(tiktokItem.getByTestId("approval-summary")).toContainText(
				"visible to: private",
			);
			await expect(tiktokItem.getByTestId("approval-cost")).toContainText(
				"Free",
			);
			expect(network.sent()).toBe(0);

			await xItem.getByRole("button", { name: "Approve" }).click();
			await tiktokItem.getByRole("button", { name: "Approve" }).click();

			await expect
				.poll(
					async () =>
						(await publicationOf(api, created.id)).targets
							.map((t) => `${t.network}:${t.state}`)
							.sort()
							.join(","),
					{ timeout: 120_000 },
				)
				.toBe("tiktok:sent_as_draft,x:published");

			// What went out: one post, one draft, no public address of the file, and the access token was
			// not sent to the upload address.
			expect(
				network.received.filter((r) => r === "POST /2/tweets"),
			).toHaveLength(1);
			expect(
				network.received.filter(
					(r) => r === "POST /v2/post/publish/inbox/video/init/",
				),
			).toHaveLength(1);
			expect(network.uploadAuth.every((a) => a === "")).toBe(true);
			expect(network.bodies.join("")).not.toContain("PULL_FROM_URL");
			expect(network.received.length).toBeGreaterThan(before);

			// The calendar tells a draft from a publication, in words.
			await page.goto("/publish");
			await page.getByTestId("publish-mode-list").click();
			// Only the rows of this publication: the other specs share the workspace and run at the same time.
			const rows = page.locator(
				created.targets
					.map((t) => `[data-testid="publish-row"][data-target-id="${t.id}"]`)
					.join(", "),
			);
			await expect(rows).toHaveCount(2);
			const draft = rows.filter({ hasText: "Sent as a draft" });
			await expect(draft).toHaveCount(1);
			await expect(draft.getByTestId("target-draft")).toContainText(
				"not published",
			);
			const posted = rows.filter({
				has: page.locator('[data-state="published"]'),
			});
			await expect(posted).toHaveCount(1);
			// The address of the loopback fake is not https, so the page shows it as text, not as a link.
			await expect(posted).toContainText("/i/status/post1");
			await expect(posted.getByTestId("target-draft")).toHaveCount(0);

			// The ledger recorded the price after the post went out.
			const cost = (await api("GET", "/cost")) as {
				today: { scope: string; project_id?: string; micros: number }[];
			};
			// The line of this project: other specs spend in the same workspace at the same time.
			expect(
				cost.today
					.filter(
						(l) => l.scope === "project" && l.project_id === made.project_id,
					)
					.reduce((sum, l) => sum + l.micros, 0),
			).toBe(200_000);
		} finally {
			await cleanUp(request, network);
		}
	});

	test("the guides and the draft speak Portuguese", async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await signInAsPerson(page);
		await page.goto("/connections");
		await page.getByRole("button", { name: "Configurar TikTok" }).click();
		await expect(page.locator("[data-research-note]")).toContainText(
			"rascunho",
		);
		await page.getByRole("button", { name: "Configurar X" }).click();
		await expect(page.locator("[data-research-note]")).toContainText(
			"US$ 0,200",
		);
	});
});

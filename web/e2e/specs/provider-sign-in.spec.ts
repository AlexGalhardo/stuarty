import { createHash, randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import {
	createServer,
	type IncomingMessage,
	type Server,
	type ServerResponse,
} from "node:http";
import { join } from "node:path";
import type { APIRequestContext, Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { SIGNIN_PORT } from "../support/network";
import { caller, expect, home, test } from "./helpers";

// ADR-0036, D-059, epic 12 (SDD-016 section 12): a creator connects an AI provider with a sign-in in the
// browser instead of a key. The authorization server is a fake of this spec on loopback, which the server
// of the suite is told about with STUARTY_TEST_SIGNIN_URL: it checks PKCE, plays the consent screen,
// issues the tokens and answers the model list, so no real account is involved.
test.describe.configure({ mode: "serial" });

const FAKE_PORT = SIGNIN_PORT;
const ISSUED_CLIENT = "oaiapp_e2e0123456789";
const ACCESS_TOKEN = "e2e-access-token-CANARY-0123456789";
const REFRESH_TOKEN = "e2e-refresh-token-CANARY-0123456789";
const ROUTER_KEY = "sk-or-v1-e2e-key-CANARY-0123456789";

interface FakeProvider {
	received: string[];
	// holdExchange makes the next key exchange of OpenRouter wait until the returned function is called, so
	// a spec can look at the page while the exchange is in flight.
	holdExchange(): () => void;
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

function challengeOf(verifier: string): string {
	return createHash("sha256").update(verifier).digest("base64url");
}

function jwt(claims: Record<string, unknown>): string {
	const part = (value: unknown) =>
		Buffer.from(JSON.stringify(value)).toString("base64url");
	return `${part({ alg: "none" })}.${part(claims)}.sig`;
}

interface Grant {
	challenge: string;
	redirect: string;
	nonce: string;
	used: boolean;
}

// The fake provider: the consent screen of both providers redirects straight back with a code; the token
// endpoint of the plan sign-in and the key exchange of OpenRouter check the PKCE verifier; the model
// lists answer only to the token or the key they issued.
async function fakeProvider(): Promise<FakeProvider> {
	const received: string[] = [];
	const grants = new Map<string, Grant>();
	let held: Promise<void> = Promise.resolve();
	const server: Server = createServer(
		(request: IncomingMessage, response: ServerResponse) => {
			void (async () => {
				const url = new URL(
					request.url ?? "/",
					`http://127.0.0.1:${FAKE_PORT}`,
				);
				received.push(`${request.method} ${url.pathname}`);
				const body = await readBody(request);
				const json = (status: number, value: unknown) => {
					response.writeHead(status, { "Content-Type": "application/json" });
					response.end(JSON.stringify(value));
				};
				const bearer = (request.headers.authorization ?? "").replace(
					"Bearer ",
					"",
				);
				if (
					url.pathname === "/api/accounts/authorize" ||
					url.pathname === "/auth"
				) {
					const plan = url.pathname === "/api/accounts/authorize";
					const redirect =
						url.searchParams.get(plan ? "redirect_uri" : "callback_url") ?? "";
					const code = randomBytes(8).toString("hex");
					grants.set(code, {
						challenge: url.searchParams.get("code_challenge") ?? "",
						redirect,
						nonce: url.searchParams.get("nonce") ?? "",
						used: false,
					});
					const target = new URL(redirect);
					target.searchParams.set("code", code);
					target.searchParams.set("state", url.searchParams.get("state") ?? "");
					if (plan) target.searchParams.set("client_id", ISSUED_CLIENT);
					response.writeHead(302, { Location: target.toString() });
					response.end();
				} else if (url.pathname === "/api/accounts/oauth/token") {
					const form = new URLSearchParams(body);
					const grant = grants.get(form.get("code") ?? "");
					if (
						form.get("grant_type") !== "authorization_code" ||
						grant === undefined ||
						grant.used ||
						challengeOf(form.get("code_verifier") ?? "") !== grant.challenge ||
						form.get("client_id") !== ISSUED_CLIENT ||
						form.get("redirect_uri") !== grant.redirect
					) {
						json(400, { error: "invalid_grant" });
						return;
					}
					grant.used = true;
					json(200, {
						access_token: ACCESS_TOKEN,
						refresh_token: REFRESH_TOKEN,
						expires_in: 3600,
						token_type: "Bearer",
						scope:
							"openid profile email offline_access resource.invoke chatgpt.tokens.use.direct",
						id_token: jwt({
							iss: `http://127.0.0.1:${FAKE_PORT}`,
							aud: ISSUED_CLIENT,
							exp: Math.floor(Date.now() / 1000) + 3600,
							nonce: grant.nonce,
							email: "creator@example.test",
						}),
					});
				} else if (url.pathname === "/api/v1/auth/keys") {
					await held;
					const input = JSON.parse(body || "{}") as {
						code?: string;
						code_verifier?: string;
					};
					const grant = grants.get(input.code ?? "");
					if (
						grant === undefined ||
						grant.used ||
						challengeOf(input.code_verifier ?? "") !== grant.challenge
					) {
						json(403, { error: "Invalid code or code_verifier" });
						return;
					}
					grant.used = true;
					json(200, { key: ROUTER_KEY });
				} else if (url.pathname === "/v1/models") {
					if (bearer !== ACCESS_TOKEN) {
						json(401, { error: { code: "invalid_token" } });
						return;
					}
					json(200, {
						models: [
							{
								slug: "e2e-plan-model",
								display_name: "E2E plan model",
								visibility: "list",
							},
						],
					});
				} else if (url.pathname === "/api/v1/models") {
					if (bearer !== ROUTER_KEY) {
						json(401, { error: { code: 401 } });
						return;
					}
					json(200, {
						data: [
							{
								id: "e2e/router-model",
								name: "E2E router model",
								context_length: 8000,
								top_provider: { max_completion_tokens: 2000 },
								pricing: { prompt: "0.000001", completion: "0.000002" },
							},
						],
					});
				} else {
					json(404, {});
				}
			})();
		},
	);
	await new Promise<void>((resolve) =>
		server.listen(FAKE_PORT, "127.0.0.1", () => resolve()),
	);
	return {
		received,
		holdExchange: () => {
			let release = () => {};
			held = new Promise<void>((resolve) => {
				release = resolve;
			});
			return release;
		},
		close: () => new Promise((resolve) => server.close(() => resolve())),
	};
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

// Leave the workspace as it was found: the connections spec expects it to hold none. A sign-in is removed by a
// person, so the page (which holds the session) does it; the local token would get E_PRIVILEGED_SESSION. Only
// the connections this spec made are removed, by the name the sign-in gives (ChatGPT, OpenRouter), because
// another spec may have a connection of the same kind at the same time.
async function removeConnections(
	page: Page,
	request: APIRequestContext,
	kind: string,
	label: string,
): Promise<void> {
	const api = await caller(request);
	const list = (await api("GET", "/connections")) as {
		connections: { id: string; kind: string; label: string }[];
	};
	for (const connection of list.connections.filter(
		(c) => c.kind === kind && c.label === label,
	)) {
		const status = await page.evaluate(async (id) => {
			const response = await fetch(`/api/v1/connections/${id}`, {
				method: "DELETE",
				credentials: "same-origin",
				headers: {
					Accept: "application/json",
					"Content-Type": "application/json",
				},
			});
			return `${response.status} ${await response.text()}`;
		}, connection.id);
		expect(status).toMatch(/^200 /);
	}
}

// @chromium-only: the fake provider listens on the one port the server of the suite is told about, so two
// browsers running this spec at once would collide; one engine is enough for a flow of the product.
test.describe("provider sign-in @chromium-only", () => {
	test("sign in with ChatGPT, read the models of the plan, and no token reaches the page", async ({
		page,
		request,
	}) => {
		const fake = await fakeProvider();
		try {
			await signInAsPerson(page);
			await page.goto("/connections");
			const card = page.locator('[data-provider="openai"]');
			await expect(
				card.getByRole("button", { name: "Sign in with ChatGPT" }),
			).toBeVisible();
			await card.getByRole("button", { name: "Sign in with ChatGPT" }).click();

			// The fake consent screen redirects back; the page finishes the sign-in.
			await expect(page.locator("[data-signed-in]")).toContainText(
				"creator@example.test",
			);
			// The state, the code and the client are not left in the address.
			expect(page.url()).not.toContain("code=");
			expect(page.url()).not.toContain("state=");
			await page.getByRole("link", { name: "Back to the connections" }).click();

			const connection = page
				.locator('[data-provider="openai"] [data-provider-connection]')
				.filter({ has: page.locator("[data-signin-state]") })
				.first();
			await expect(connection.locator("[data-signin-state]")).toContainText(
				"Signed in",
			);
			await expect(connection.locator("[data-signin-state]")).toContainText(
				"creator@example.test",
			);

			await connection.getByRole("button", { name: "Show the models" }).click();
			await expect(connection.locator("[data-model-id]")).toContainText(
				"e2e-plan-model",
			);
			// A plan has no price per token: the catalogue says zero.
			await expect(connection.locator("[data-model-id]")).toContainText(
				"published by the provider",
			);

			// No token is shown, and none is in what the API says about the connection.
			await expect(page.locator("body")).not.toContainText(ACCESS_TOKEN);
			await expect(page.locator("body")).not.toContainText(REFRESH_TOKEN);
			const api = await caller(request);
			const list = JSON.stringify(await api("GET", "/connections"));
			const signIns = JSON.stringify(await api("GET", "/providers/sign-ins"));
			for (const secret of [ACCESS_TOKEN, REFRESH_TOKEN, ISSUED_CLIENT]) {
				expect(list).not.toContain(secret);
				expect(signIns).not.toContain(secret);
			}
			expect(signIns).toContain("creator@example.test");

			// The token went to the API host and to nowhere else; a replayed redirect makes nothing.
			expect(fake.received).toContain("POST /api/accounts/oauth/token");
			const before = fake.received.length;
			const stale = await page.evaluate(async () => {
				const response = await fetch("/api/v1/providers/signin/complete", {
					method: "POST",
					credentials: "same-origin",
					headers: {
						Accept: "application/json",
						"Content-Type": "application/json",
					},
					body: JSON.stringify({ state: "not-a-state", code: "x" }),
				});
				return response.status;
			});
			expect(stale).toBe(400);
			expect(fake.received.length).toBe(before);
		} finally {
			await removeConnections(page, request, "openai", "ChatGPT");
			await fake.close();
		}
	});

	test("sign in with OpenRouter makes a key that is only in the vault", async ({
		page,
		request,
	}) => {
		const fake = await fakeProvider();
		try {
			await signInAsPerson(page);
			await page.goto("/connections");
			const card = page.locator('[data-provider="openrouter"]');
			const release = fake.holdExchange();
			await card
				.getByRole("button", { name: "Sign in with OpenRouter" })
				.click();
			// While the code is being exchanged (the fake holds the answer), the address no longer holds the
			// state or the code (SDD-007 T-76).
			await expect
				.poll(() => fake.received.includes("POST /api/v1/auth/keys"))
				.toBe(true);
			await expect(page.getByRole("status")).toContainText("Finishing");
			expect(new URL(page.url()).search).toBe("");
			release();
			await expect(page.locator("[data-signed-in]")).toBeVisible();
			await page.getByRole("link", { name: "Back to the connections" }).click();

			const connection = page
				.locator('[data-provider="openrouter"] [data-provider-connection]')
				.filter({ has: page.locator("[data-signin-state]") })
				.first();
			await expect(connection.locator("[data-signin-state]")).toContainText(
				"Signed in",
			);
			await connection.getByRole("button", { name: "Show the models" }).click();
			await expect(connection.locator("[data-model-id]")).toContainText(
				"e2e/router-model",
			);
			await expect(connection.locator("[data-model-id]")).toContainText(
				"published by the provider",
			);
			await expect(page.locator("body")).not.toContainText(ROUTER_KEY);

			const api = await caller(request);
			expect(JSON.stringify(await api("GET", "/connections"))).not.toContain(
				ROUTER_KEY,
			);
			expect(fake.received).toContain("POST /api/v1/auth/keys");
		} finally {
			await removeConnections(page, request, "openrouter", "OpenRouter");
			await fake.close();
		}
	});

	test("the local token cannot begin or complete a sign-in", async ({
		request,
	}) => {
		const api = await caller(request);
		// The local token is what an agent holds: it reads, and a person alone signs in.
		for (const [path, data] of [
			[
				"/providers/signin",
				{
					provider: "openrouter",
					redirect_uri: "http://127.0.0.1/auth/callback",
				},
			],
			["/providers/signin/complete", { state: "x", code: "y" }],
		] as const) {
			const response = await request.post(`/api/v1${path}`, {
				data,
				headers: {
					Authorization: `Bearer ${(await readFile(join(home, "token"), "utf8")).trim()}`,
				},
			});
			expect(response.status()).toBe(403);
			expect(((await response.json()) as { code: string }).code).toBe(
				"E_PRIVILEGED_SESSION",
			);
		}
		expect(await api("GET", "/providers/sign-ins")).toEqual({ sign_ins: [] });
	});

	test("the sign-in speaks Portuguese", async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await signInAsPerson(page);
		await page.goto("/connections");
		await expect(
			page.getByRole("button", { name: "Entrar com o ChatGPT" }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Entrar com o OpenRouter" }),
		).toBeVisible();
	});
});

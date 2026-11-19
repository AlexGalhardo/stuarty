import {
	createPrivateKey,
	createSign,
	generateKeyPairSync,
	type KeyObject,
} from "node:crypto";
import { createServer, type Server } from "node:http";
import { type APIRequestContext, expect, test } from "@playwright/test";
import { GOOGLE_PORT, NETWORK_ADMIN, NETWORK_URL } from "../support/network";
import { test as local } from "./helpers";

// The optional Google sign-in of network mode (epic 24, SDD-007 section 4.7, SDD-014 section 12), against a
// fake Google on loopback: it publishes keys, sends the browser back with a code and signs the ID token.
// No real Google account is involved. The fake listens on one fixed port that the network server was told
// about (STUARTY_TEST_GOOGLE_URL), so the spec runs in one browser only: @chromium-only, a port that two
// workers cannot share, not an engine limit.
test.describe.configure({ mode: "serial" });

const GOOGLE_URL = `http://127.0.0.1:${GOOGLE_PORT}`;
const CLIENT_ID = "e2e-client.apps.example.test";
const origin = { Origin: NETWORK_URL };

const b64 = (value: Buffer | string): string =>
	Buffer.from(value).toString("base64url");

interface Identity {
	sub: string;
	email: string;
}

// The Google account of the person at the keyboard; a test sets it before it presses the button.
let identity: Identity = { sub: "", email: "" };
let google: Server;

async function startGoogle(): Promise<Server> {
	const { privateKey, publicKey } = generateKeyPairSync("rsa", {
		modulusLength: 2048,
	});
	const jwk = {
		...publicKey.export({ format: "jwk" }),
		kid: "e2e-key",
		alg: "RS256",
		use: "sig",
	};
	const grants = new Map<string, { nonce: string; who: Identity }>();
	const sign = (key: KeyObject, input: string): string =>
		b64(createSign("RSA-SHA256").update(input).sign(key));

	const server = createServer((request, response) => {
		const url = new URL(request.url ?? "/", GOOGLE_URL);
		if (url.pathname === "/certs") {
			response.setHeader("Content-Type", "application/json");
			response.end(JSON.stringify({ keys: [jwk] }));
			return;
		}
		if (url.pathname === "/authorize") {
			const code = `code-${grants.size + 1}-${Date.now()}`;
			grants.set(code, {
				nonce: url.searchParams.get("nonce") ?? "",
				who: identity,
			});
			const back = new URL(url.searchParams.get("redirect_uri") ?? "");
			back.searchParams.set("code", code);
			back.searchParams.set("state", url.searchParams.get("state") ?? "");
			response.writeHead(302, { Location: back.toString() });
			response.end();
			return;
		}
		if (url.pathname === "/token" && request.method === "POST") {
			let body = "";
			request.on("data", (chunk) => {
				body += chunk;
			});
			request.on("end", () => {
				const form = new URLSearchParams(body);
				const grant = grants.get(form.get("code") ?? "");
				response.setHeader("Content-Type", "application/json");
				if (grant === undefined || !form.get("code_verifier")) {
					response.statusCode = 400;
					response.end(JSON.stringify({ error: "invalid_grant" }));
					return;
				}
				grants.delete(form.get("code") ?? "");
				const now = Math.floor(Date.now() / 1000);
				const head = b64(
					JSON.stringify({ alg: "RS256", typ: "JWT", kid: "e2e-key" }),
				);
				const claims = b64(
					JSON.stringify({
						iss: GOOGLE_URL,
						sub: grant.who.sub,
						aud: form.get("client_id"),
						iat: now - 60,
						exp: now + 3600,
						nonce: grant.nonce,
						email: grant.who.email,
						email_verified: true,
					}),
				);
				const input = `${head}.${claims}`;
				response.end(
					JSON.stringify({
						access_token: "fake-access-token",
						token_type: "Bearer",
						id_token: `${input}.${sign(createPrivateKey(privateKey.export({ format: "pem", type: "pkcs8" })), input)}`,
					}),
				);
			});
			return;
		}
		response.statusCode = 404;
		response.end();
	});
	await new Promise<void>((resolve) =>
		server.listen(GOOGLE_PORT, "127.0.0.1", () => resolve()),
	);
	return server;
}

async function adminSession(
	newContext: (o: { baseURL: string }) => Promise<APIRequestContext>,
): Promise<APIRequestContext> {
	const context = await newContext({ baseURL: NETWORK_URL });
	// Five attempts a minute for one email: wait the seconds the answer names.
	for (let attempt = 0; ; attempt++) {
		const login = await context.post("/auth/login", {
			data: NETWORK_ADMIN,
			headers: origin,
		});
		if (login.ok()) return context;
		if (login.status() !== 429 || attempt === 12) {
			throw new Error(`admin login: ${login.status()}`);
		}
		await new Promise((resolve) => setTimeout(resolve, 5_000));
	}
}

const PASSWORD = "e2e-member-password-1";
const email = `google-${Date.now()}@example.test`;
const googleEmail = `person-${Date.now()}@gmail.test`;

test.describe("google-sign-in", () => {
	test.use({ baseURL: NETWORK_URL });

	test.beforeAll(async ({ playwright }) => {
		google = await startGoogle();
		const admin = await adminSession((o) => playwright.request.newContext(o));

		const made = await admin.post("/api/v1/users", {
			data: { email, password: PASSWORD, role: "member" },
			headers: origin,
		});
		expect(made.ok(), await made.text()).toBe(true);

		// The connection of the owner's own Google project: a YouTube connection that reaches the fake.
		const connection = await admin.post("/api/v1/connections", {
			data: {
				kind: "youtube",
				label: "Google project (e2e)",
				endpoint: GOOGLE_URL,
				accepts: ["metadata"],
				secret: JSON.stringify({
					client_id: CLIENT_ID,
					client_secret: "e2e-client-secret",
				}),
			},
			headers: origin,
		});
		expect(connection.ok(), await connection.text()).toBe(true);
		const { id } = (await connection.json()) as { id: string };

		const settings = await admin.put("/auth/google/settings", {
			data: { enabled: true, connection_id: id },
			headers: origin,
		});
		expect(settings.ok(), await settings.text()).toBe(true);
	});

	test.afterAll(async () => {
		await new Promise<void>((resolve) => google.close(() => resolve()));
	});

	test("link, sign out, sign in with Google, then an unlinked identity is refused @chromium-only", async ({
		page,
	}) => {
		// The password login, then the link, with the password asked again.
		await page.goto("/login");
		await page.getByLabel("Email").fill(email);
		await page.getByLabel("Password").fill(PASSWORD);
		await page.getByRole("button", { name: "Log in" }).click();
		await expect(page).toHaveURL(`${NETWORK_URL}/projects`);

		identity = { sub: "sub-e2e-person", email: googleEmail };
		await page.goto("/settings/google");
		await page.getByLabel("Your password").fill(PASSWORD);
		await page.getByRole("button", { name: "Link my Google account" }).click();
		await expect(page).toHaveURL(`${NETWORK_URL}/settings/google`);
		await expect(page.locator("[data-google-linked]")).toContainText(
			googleEmail,
		);

		// Sign out, and come back through Google.
		await page.getByRole("button", { name: "Log out" }).click();
		await expect(page).toHaveURL(`${NETWORK_URL}/login`);
		await page.getByRole("button", { name: "Sign in with Google" }).click();
		await expect(page).toHaveURL(`${NETWORK_URL}/projects`);
		await expect(page.getByTestId("profile-name")).toContainText(email);

		// The same session as with a password: the API answers for the same user.
		const me = await page.request.get("/auth/google");
		expect(await me.json()).toMatchObject({
			enabled: true,
			linked: { email: googleEmail },
		});

		// A Google account nobody linked cannot sign in, even with the e-mail of a local user.
		await page.getByRole("button", { name: "Log out" }).click();
		identity = { sub: "sub-a-stranger", email };
		await page.getByRole("button", { name: "Sign in with Google" }).click();
		await expect(page).toHaveURL(
			new RegExp(`^${NETWORK_URL}/login/google/callback`),
		);
		await expect(page.locator("[data-google-phase='failed']")).toBeVisible();
		await expect(page.getByRole("alert")).toContainText("not linked");
		await page.goto("/projects");
		await expect(page).toHaveURL(new RegExp(`^${NETWORK_URL}/login`));
	});

	test("the password works after unlinking, and Google no longer does @chromium-only", async ({
		page,
	}) => {
		await page.goto("/login");
		await page.getByLabel("Email").fill(email);
		await page.getByLabel("Password").fill(PASSWORD);
		await page.getByRole("button", { name: "Log in" }).click();
		await expect(page).toHaveURL(`${NETWORK_URL}/projects`);

		await page.goto("/settings/google");
		await expect(page.locator("[data-google-linked]")).toContainText(
			googleEmail,
		);
		await page.getByLabel("Your password").fill(PASSWORD);
		await page.getByRole("button", { name: "Unlink" }).click();
		await expect(page.locator("[data-google-linked]")).toHaveCount(0);

		await page.getByRole("button", { name: "Log out" }).click();
		identity = { sub: "sub-e2e-person", email: googleEmail };
		await page.getByRole("button", { name: "Sign in with Google" }).click();
		await expect(page.locator("[data-google-phase='failed']")).toBeVisible();

		await page.goto("/login");
		await page.getByLabel("Email").fill(email);
		await page.getByLabel("Password").fill(PASSWORD);
		await page.getByRole("button", { name: "Log in" }).click();
		await expect(page).toHaveURL(`${NETWORK_URL}/projects`);
	});
});

// The local server of the suite, with the session of its page fixture: no login, and no Google pages.
local.describe("google-sign-in in local mode", () => {
	local(
		"no login is shown and the Google routes do not exist",
		async ({ page }) => {
			for (const path of [
				"/login",
				"/settings/google",
				"/login/google/callback",
			]) {
				const response = await page.goto(path);
				expect(response?.status(), path).toBe(404);
			}
			const status = await page.request.get("/auth/google");
			expect(status.status()).toBe(404);
		},
	);
});

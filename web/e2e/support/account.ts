import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import {
	type APIRequestContext,
	expect,
	type Page,
	test,
} from "@playwright/test";
import { accountTargets } from "./accountTargets";

// The account the suite signs in with (ADR-0052): a throwaway, not a credential. Every local server of the
// run has one made before the first spec (accountSetup.ts), since a workspace with none sends every page to
// the create-account page; the first-run spec has a server of its own that has none.
export const E2E_ACCOUNT = {
	username: "stuart",
	email: "stuart@example.test",
	password: "E2e-Password-1!",
};

// The address of the sign-in is a root route and a JSON request, as the page sends it.
const JSON_HEADERS = { "Content-Type": "application/json" };

// ensureAccount makes the account of the server that `request` points at, with the local token of its
// workspace, when it has none yet. A server kept from an earlier run already has it.
export async function ensureAccount(
	request: APIRequestContext,
	home: string,
): Promise<void> {
	const state = await request.get("/auth/account");
	expect(state.ok()).toBe(true);
	if (((await state.json()) as { state: string }).state !== "none") return;
	const token = (await readFile(join(home, "token"), "utf8")).trim();
	// The token alone cannot create the account (VV-378): the run opens the browser the way `stuarty open` does,
	// with a launch code, and creates it from that session.
	const launch = await request.post("/api/v1/system/launch", {
		headers: { ...JSON_HEADERS, Authorization: `Bearer ${token}` },
	});
	expect(launch.ok(), await launch.text()).toBe(true);
	const { code } = (await launch.json()) as { code: string };
	const opened = await request.get(`/auth/launch?code=${code}`, {
		headers: { "Sec-Fetch-Site": "none" },
		maxRedirects: 0,
	});
	expect(opened.status(), await opened.text()).toBe(303);
	const created = await request.post("/auth/account/create", {
		headers: { ...JSON_HEADERS, "Sec-Fetch-Site": "same-origin" },
		data: {
			username: E2E_ACCOUNT.username,
			email: E2E_ACCOUNT.email,
			password: E2E_ACCOUNT.password,
			confirmation: E2E_ACCOUNT.password,
		},
	});
	expect(created.ok(), await created.text()).toBe(true);
	const { recovery_key } = (await created.json()) as { recovery_key: string };
	await writeFile(recoveryKeyPath(home), recovery_key, { mode: 0o600 });
}

// recoveryKeyPath is where the run keeps the recovery key of the throwaway account, next to the workspace and never
// in it: a spec that seeds a sealed column needs it (sealField.ts).
export function recoveryKeyPath(home: string): string {
	return `${home}.recovery-key`;
}

// signInWithPassword puts the session of a person in the cookie jar of `target` (a page, or the request context of
// the global setup), unlocking the studio when no such session exists yet.
//
// A page reuses the session of the person that the global setup opened on its server (accountSetup.ts) instead
// of signing in again: signing in can invalidate sessions and lock the studio while the unlock hooks run.
export async function signInWithPassword(
	target: APIRequestContext | Page,
): Promise<void> {
	if (isPage(target)) {
		await restorePersonSession(target);
		return;
	}
	await signInOn(target);
}

async function signInOn(target: APIRequestContext): Promise<void> {
	const response = await target.post("/auth/account/login", {
		headers: JSON_HEADERS,
		data: {
			identity: E2E_ACCOUNT.username,
			password: E2E_ACCOUNT.password,
		},
	});
	expect(response.ok(), await response.text()).toBe(true);
}

function isPage(target: APIRequestContext | Page): target is Page {
	return typeof (target as Page).goto === "function";
}

// Restore only this project's server state. Account settings requires a person's session.
async function restorePersonSession(page: Page): Promise<void> {
	const baseURL = test.info().project.use.baseURL;
	const target = accountTargets().find((entry) => entry.baseURL === baseURL);
	if (target) {
		const state = JSON.parse(await readFile(target.statePath, "utf8")) as {
			cookies: Parameters<ReturnType<Page["context"]>["addCookies"]>[0];
		};
		await page.context().addCookies(state.cookies);
		const response = await page.request.get("/auth/account/settings");
		if (response.ok()) return;
		// A spec that starts a server of its own (the first-flow ones) has a workspace of its own: the session of
		// the shared server means nothing there, so the person signs in on this server.
		if (response.status() !== 401 && response.status() !== 403) {
			expect(response.ok(), await response.text()).toBe(true);
		}
	}
	await signInOn(page.request);
}

import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { type APIRequestContext, expect } from "@playwright/test";

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
	const created = await request.post("/auth/account/create", {
		headers: { ...JSON_HEADERS, Authorization: `Bearer ${token}` },
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

// signInWithPassword unlocks the studio and puts the session of a person in the cookie jar of `request`.
export async function signInWithPassword(
	request: APIRequestContext,
): Promise<void> {
	const response = await request.post("/auth/account/login", {
		headers: JSON_HEADERS,
		data: {
			identity: E2E_ACCOUNT.username,
			password: E2E_ACCOUNT.password,
		},
	});
	expect(response.ok(), await response.text()).toBe(true);
}

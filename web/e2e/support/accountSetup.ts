import { writeFile } from "node:fs/promises";
import { request } from "@playwright/test";
import { ensureAccount, signInWithPassword } from "./account";

// Where the session of the person of each local server is kept for the specs (playwright.config.ts names the
// files): a storage state with the cookie that the sign-in set.
export interface AccountTarget {
	baseURL: string;
	home: string;
	statePath: string;
}

// The global setup of the run: for each local server that is up, make the account, sign in and keep the
// session. A server that is not running (its project was not chosen) is skipped.
export async function signInEveryServer(
	targets: AccountTarget[],
): Promise<void> {
	for (const target of targets) {
		const context = await request.newContext({ baseURL: target.baseURL });
		try {
			const up = await context.get("/up").catch(() => null);
			if (up === null || !up.ok()) continue;
			await ensureAccount(context, target.home);
			await signInWithPassword(context);
			await writeFile(
				target.statePath,
				JSON.stringify(await context.storageState()),
			);
		} finally {
			await context.dispose();
		}
	}
}

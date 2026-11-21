import { randomInt } from "node:crypto";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import type { Page } from "@playwright/test";
import { home } from "../specs/helpers";
import { ensureAccount, signInWithPassword } from "./account";

const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

function newId(prefix: string): string {
	let out = "";
	for (let i = 0; i < 10; i++) out += ALPHABET[randomInt(ALPHABET.length)];
	return `${prefix}_${out}`;
}

// The product registers no job kind that needs approval until the epics of
// publishing and generation, so a spec writes the rows of the requests into the
// database of the workspace as that code will, and the real server reads and
// decides them (the same as approval.spec.ts).
export function seedApprovals(summaries: string[]): string[] {
	const db = new DatabaseSync(join(home, "db.sqlite"));
	try {
		db.exec("PRAGMA busy_timeout = 10000");
		const insert = db.prepare(
			`INSERT INTO approval_requests
 (id, actor, kind, summary, connection_id, payload_ref, estimate_micros, currency, state, created_ms, expires_ms)
 VALUES (?, 'local:claude-code', 'publish.post', ?, NULL, 'draft:e2e', 0, '', 'pending', ?, ?)`,
		);
		const now = Date.now();
		return summaries.map((summary, i) => {
			const id = newId("apr");
			insert.run(id, summary, now + i, now + 7 * 24 * 3600 * 1000);
			return id;
		});
	} finally {
		db.close();
	}
}

// A person decides, and a person is a session opened with the password (ADR-0049, SDD-007 section 3.9 item 5): a
// launch code no longer makes one. The approval queue refuses the local token an agent holds.
export async function signInAsPerson(page: Page): Promise<void> {
	await ensureAccount(page.request, home);
	await signInWithPassword(page.request);
	await page.goto("/");
}

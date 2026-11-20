import { randomInt } from "node:crypto";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import type { Page } from "@playwright/test";
import { expect, home } from "../specs/helpers";
import { LAUNCHER_SECRET } from "./launcher";

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

// A person decides in the desktop app, whose launch code carries the launcher
// secret (SDD-007 section 3.4 item 7); the server of the suite is started with
// the secret of LAUNCHER_SECRET.
export async function signInAsPerson(page: Page): Promise<void> {
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

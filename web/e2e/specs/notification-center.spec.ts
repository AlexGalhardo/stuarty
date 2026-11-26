import { execFileSync } from "node:child_process";
import { randomBytes, randomInt } from "node:crypto";
import {
	closeSync,
	mkdirSync,
	mkdtempSync,
	openSync,
	readFileSync,
	rmSync,
	writeSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { ensureAccount, signInWithPassword } from "../support/account";
import { fieldSealer } from "../support/sealField";
import { caller, expect, home, test } from "./helpers";

// SDD-012 section 23.4, epic 75: the bell counts what waits for a person (the open permission of the chat
// session the shell reads and the pending approvals), and the center lists each request with the action that
// leads to its own flow. The agent is the fake agent of the tests; the approval is written into the database
// of the workspace the way the code of publishing will, and the real server reads it.
test.describe.configure({ mode: "serial" });

const root = fileURLToPath(new URL("../../..", import.meta.url));

let fakeAgent = "";

// The agent profiles and the live sessions belong to the whole workspace: this file takes the lock of
// agent-chat.spec.ts and scene-bar.spec.ts, so none of the three runs while another holds the agent.
const lockPath = (): string => join(home, "agent-chat.e2e.lock");

function takeLock(): boolean {
	try {
		const fd = openSync(lockPath(), "wx");
		writeSync(fd, String(process.pid));
		closeSync(fd);
		return true;
	} catch {
		let holder = Number.NaN;
		try {
			holder = Number(readFileSync(lockPath(), "utf8"));
		} catch {
			return false;
		}
		try {
			process.kill(holder, 0);
			return false;
		} catch {
			rmSync(lockPath(), { force: true });
			return false;
		}
	}
}

test.beforeAll(async () => {
	test.setTimeout(20 * 60_000);
	await expect
		.poll(takeLock, {
			message: "another browser holds the workspace of the chat tests",
			timeout: 19 * 60_000,
		})
		.toBe(true);
	const dir = mkdtempSync(join(tmpdir(), "stuarty-e2e-notify-"));
	mkdirSync(dir, { recursive: true });
	fakeAgent = join(
		dir,
		process.platform === "win32" ? "fakeagent.exe" : "fakeagent",
	);
	execFileSync(
		"go",
		["build", "-o", fakeAgent, "./service/agenthost/fakeagent"],
		{ cwd: root },
	);
});

test.afterAll(() => {
	try {
		if (Number(readFileSync(lockPath(), "utf8")) === process.pid) {
			rmSync(lockPath(), { force: true });
		}
	} catch {
		// no lock to release
	}
});

const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

function newId(prefix: string): string {
	let out = "";
	for (let i = 0; i < 10; i++) out += ALPHABET[randomInt(ALPHABET.length)];
	return `${prefix}_${out}`;
}

function seedApproval(summary: string): string {
	const db = new DatabaseSync(join(home, "db.sqlite"));
	try {
		db.exec("PRAGMA busy_timeout = 10000");
		const seal = fieldSealer(db, home);
		const id = newId("apr");
		const now = Date.now();
		db.prepare(
			`INSERT INTO approval_requests
 (id, actor, kind, summary, connection_id, payload_ref, estimate_micros, currency, state, created_ms, expires_ms)
 VALUES (?, ?, 'publish.post', ?, NULL, 'draft:e2e', 0, '', 'pending', ?, ?)`,
		).run(
			id,
			"local:claude-code",
			seal("approval_requests", "summary", id, summary),
			now,
			now + 7 * 24 * 3600 * 1000,
		);
		return id;
	} finally {
		db.close();
	}
}

const JSON_HEADERS = {
	"Content-Type": "application/json",
	"Sec-Fetch-Site": "same-origin",
};

async function signInAsPerson(page: Page): Promise<void> {
	await ensureAccount(page.request, home);
	await signInWithPassword(page);
}

async function send(
	page: Page,
	method: "POST" | "PUT" | "DELETE",
	path: string,
	data?: unknown,
): Promise<unknown> {
	const res = await page.request.fetch(`/api/v1${path}`, {
		method,
		headers: JSON_HEADERS,
		data,
	});
	expect(res.ok(), `${method} ${path}: ${await res.text()}`).toBe(true);
	return res.json();
}

const made: string[] = [];
const seeded: string[] = [];

test.afterEach(async ({ page, request }) => {
	const api = await caller(request);
	const live: string[] = [];
	for (const project of made.splice(0)) {
		const list = (await api("GET", `/projects/${project}/chat/sessions`)) as {
			sessions: { id: string; live: boolean }[];
		};
		for (const s of list.sessions) if (s.live) live.push(s.id);
	}
	if (live.length === 0 && seeded.length === 0) return;
	await signInAsPerson(page);
	for (const id of live) {
		await send(page, "POST", `/chat/sessions/${id}/cancel`, { end: true });
	}
	// a request this file wrote is decided, so the workspace is left as it was found
	for (const id of seeded.splice(0)) {
		await send(page, "POST", `/approvals/${id}/decide`, { decision: "reject" });
	}
});

const unique = (browserName: string): string =>
	`${browserName}-${randomBytes(3).toString("hex")}`;

test.describe("notification center", () => {
	test("a pending approval is counted, listed, and its action leads to Approvals", async ({
		page,
		browserName,
	}) => {
		const summary = `Post the launch video ${unique(browserName)}`;
		const id = seedApproval(summary);
		seeded.push(id);
		await signInAsPerson(page);
		await page.goto("/");
		// the bell and the item of the sidebar say there is something, in a word and a number
		await expect(page.getByTestId("bell-count")).toBeVisible();
		await expect(page.getByTestId("bell")).toHaveAccessibleName(
			/Notifications, \d+ waiting for you/,
		);
		await expect(page.getByTestId("nav-count")).toBeVisible();
		await page.getByTestId("bell").click();
		const center = page.getByTestId("notification-center");
		await expect(center).toBeVisible();
		const row = center.getByTestId("notification").filter({ hasText: summary });
		await expect(row).toHaveAttribute("data-kind", "approval");
		await expect(row).toContainText("Waiting for you");
		await row.getByTestId("notification-review").click();
		await expect(page).toHaveURL(/\/approvals$/);
		// following the action closes the center, and the request is in the queue
		await expect(page.getByTestId("notification-center")).toHaveCount(0);
		await expect(page.locator(`[data-approval-id="${id}"]`)).toBeVisible();
	});

	test("the center closes with Escape and gives the focus back to the bell", async ({
		page,
	}) => {
		await signInAsPerson(page);
		await page.goto("/");
		await page.getByTestId("bell").click();
		await expect(page.getByTestId("notification-center")).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(page.getByTestId("notification-center")).toHaveCount(0);
		await expect(page.getByTestId("bell")).toBeFocused();
	});

	test("an open permission is listed with Allow once and Deny and no always, and is answered in place", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const created = await api("POST", "/projects", {
			name: `Notify ${unique(browserName)}`,
		});
		const project = String(created.project_id);
		made.push(project);
		await signInAsPerson(page);
		const listed = (await (
			await page.request.get("/api/v1/agents/profiles")
		).json()) as { profiles: { name: string }[] };
		for (const p of listed.profiles) {
			await send(
				page,
				"DELETE",
				`/agents/profiles/${encodeURIComponent(p.name)}`,
			);
		}
		await send(page, "PUT", "/agents/profiles/notifier", {
			command: fakeAgent,
			args: ["--mode=permission"],
		});
		const session = (await send(
			page,
			"POST",
			`/projects/${project}/chat/sessions`,
			{
				profile: "notifier",
			},
		)) as { id: string };
		await page.goto(`/projects/${project}`);
		await expect(page.getByTestId("scene-bar")).toHaveAttribute(
			"data-variant",
			"idle",
		);
		await send(page, "POST", `/chat/sessions/${session.id}/messages`, {
			text: "delete the clips",
		});
		await expect(page.getByTestId("scene-bar")).toHaveAttribute(
			"data-variant",
			"waiting",
		);
		await page.getByTestId("bell").click();
		const center = page.getByTestId("notification-center");
		const row = center
			.getByTestId("notification")
			.filter({ hasText: "Delete every clip" });
		await expect(row).toHaveAttribute("data-kind", "permission");
		// the agent offered "Always allow": no such action in the center, and the bell says none either
		await expect(row.getByRole("button", { name: /always/i })).toHaveCount(0);
		await expect(center.getByText(/always/i)).toHaveCount(0);
		await expect(row.getByRole("button")).toHaveText([
			"Deny",
			"Allow once",
			"Open the conversation",
		]);
		// a tool outside Stuarty: the center reports nothing it was not told, and says so; the undo of a chat
		// permission is never verified (D-140)
		for (const field of ["item", "destination"]) {
			await expect(row.getByTestId(`notification-${field}`)).toHaveText(
				"Not reported",
			);
		}
		await expect(row.getByTestId("notification-undo")).toHaveText(
			"Not verified",
		);
		await expect(row.getByTestId("notification-cost")).toHaveText("No cost");
		const answered = page.waitForResponse(
			(res) =>
				res.url().includes(`/chat/sessions/${session.id}/permissions/`) &&
				res.request().method() === "POST",
		);
		await row.getByTestId("notification-allow").click();
		expect((await answered).ok()).toBe(true);
		// it is no longer waiting: the row is history and the bar has moved on
		await expect(row).toHaveAttribute("data-state", "allowed");
		await expect(page.getByTestId("scene-bar")).not.toHaveAttribute(
			"data-variant",
			"waiting",
		);
	});

	// Epic 77: a permission for a call to a tool of Stuarty says what the call does, to what and where, from the
	// allowlisted fields of its input; nothing else the agent put in the input reaches the page.
	test("a call to a tool of Stuarty is described from the allowlisted fields of its input and from nothing else", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const created = await api("POST", "/projects", {
			name: `Notify ${unique(browserName)}`,
		});
		const project = String(created.project_id);
		made.push(project);
		await signInAsPerson(page);
		const listed = (await (
			await page.request.get("/api/v1/agents/profiles")
		).json()) as { profiles: { name: string }[] };
		for (const p of listed.profiles) {
			await send(
				page,
				"DELETE",
				`/agents/profiles/${encodeURIComponent(p.name)}`,
			);
		}
		await send(page, "PUT", "/agents/profiles/describer", {
			command: fakeAgent,
			args: ["--mode=permission-stuarty"],
		});
		const session = (await send(
			page,
			"POST",
			`/projects/${project}/chat/sessions`,
			{ profile: "describer" },
		)) as { id: string };
		await page.goto(`/projects/${project}`);
		await send(page, "POST", `/chat/sessions/${session.id}/messages`, {
			text: "restyle the captions",
		});
		await expect(page.getByTestId("scene-bar")).toHaveAttribute(
			"data-variant",
			"waiting",
		);
		await page.getByTestId("bell").click();
		const center = page.getByTestId("notification-center");
		const row = center
			.getByTestId("notification")
			.filter({ hasText: "mcp__stuarty__edit" });
		await expect(row).toHaveAttribute("data-kind", "permission");
		await expect(row.getByTestId("notification-what")).toContainText(
			"Changes a project or an image.",
		);
		await expect(row.getByTestId("notification-item")).toHaveText(
			"ast_3c5v8n2w6d",
		);
		await expect(row.getByTestId("notification-destination")).toHaveText(
			"prj_a4k9m2x7bq",
		);
		// the agent titles the call an edit of commands of the editor: a claim, so the undo is not verified (D-140)
		await expect(row.getByTestId("notification-undo")).toHaveText(
			"Not verified",
		);
		await expect(row).toContainText("as the agent described the call");
		for (const hostile of [
			"sk-hostile-secret-value",
			"hostile-bearer",
			"sk-hostile-in-args",
		]) {
			await expect(center.getByText(hostile)).toHaveCount(0);
		}
		// the same list is what the command line and the API read
		const inbox = (await (await page.request.get("/api/v1/inbox")).json()) as {
			rows: { source: string; state: string; tool: string; undo: string }[];
		};
		expect(
			inbox.rows.some(
				(r) =>
					r.source === "chat" &&
					r.state === "waiting" &&
					r.tool === "mcp__stuarty__edit" &&
					r.undo === "not_verified",
			),
		).toBe(true);
		expect(JSON.stringify(inbox)).not.toContain("sk-hostile");
	});
});

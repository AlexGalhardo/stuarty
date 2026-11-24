import { execFileSync } from "node:child_process";
import { randomBytes } from "node:crypto";
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
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { ensureAccount, signInWithPassword } from "../support/account";
import { caller, expect, home, test } from "./helpers";

// SDD-012 section 23.4, epic 75: the shell of every dashboard page. The agent is the fake agent of the
// tests (service/agenthost/fakeagent); the server is the real one. A permission is answered on the bar
// through the call the chat panel makes, and the conversation shows the same answer.
test.describe.configure({ mode: "serial" });

const root = fileURLToPath(new URL("../../..", import.meta.url));

let fakeAgent = "";

// The agent profiles and the live sessions belong to the whole workspace, which every browser project of a
// run shares, and agent-chat.spec.ts asserts what the workspace holds. The two files take the same lock, so
// they never run at once: a file that names the worker that holds it, taken over when that worker is gone.
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
	const dir = mkdtempSync(join(tmpdir(), "stuarty-e2e-scenebar-"));
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

const made: string[] = [];

const JSON_HEADERS = {
	"Content-Type": "application/json",
	"Sec-Fetch-Site": "same-origin",
};

// A person drives the chat: the session of the desktop app, whose launch carries the launcher secret.
async function signInAsPerson(page: Page): Promise<void> {
	await ensureAccount(page.request, home);
	await signInWithPassword(page.request);
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

async function clearProfiles(page: Page): Promise<void> {
	const listed = await page.request.get("/api/v1/agents/profiles");
	const { profiles } = (await listed.json()) as {
		profiles: { name: string }[];
	};
	for (const p of profiles) {
		await send(
			page,
			"DELETE",
			`/agents/profiles/${encodeURIComponent(p.name)}`,
		);
	}
}

// Connects the fake agent in a mode and starts a conversation in the project; the bar reads it from the
// project page, whose address names the project.
async function converse(
	page: Page,
	project: string,
	name: string,
	mode: string,
): Promise<string> {
	await send(page, "PUT", `/agents/profiles/${name}`, {
		command: fakeAgent,
		args: [`--mode=${mode}`],
	});
	const session = (await send(
		page,
		"POST",
		`/projects/${project}/chat/sessions`,
		{
			profile: name,
		},
	)) as { id: string };
	return session.id;
}

async function say(page: Page, session: string, text: string): Promise<void> {
	await send(page, "POST", `/chat/sessions/${session}/messages`, { text });
}

const unique = (browserName: string): string =>
	`${browserName}-${randomBytes(3).toString("hex")}`;

async function makeProject(
	api: Awaited<ReturnType<typeof caller>>,
	name: string,
): Promise<string> {
	const created = await api("POST", "/projects", { name });
	const id = String(created.project_id);
	made.push(id);
	return id;
}

test.afterEach(async ({ page, request }) => {
	const api = await caller(request);
	const live: string[] = [];
	for (const project of made.splice(0)) {
		const list = (await api("GET", `/projects/${project}/chat/sessions`)) as {
			sessions: { id: string; live: boolean }[];
		};
		for (const s of list.sessions) if (s.live) live.push(s.id);
	}
	if (live.length === 0) return;
	await signInAsPerson(page);
	for (const id of live) {
		await send(page, "POST", `/chat/sessions/${id}/cancel`, { end: true });
	}
});

test.describe("scene bar", () => {
	test("every dashboard page has the sidebar, the top bar and the scene bar, and none covers the page", async ({
		page,
	}) => {
		await signInAsPerson(page);
		for (const path of [
			"/",
			"/projects",
			"/write",
			"/approvals",
			"/publish",
			"/connections",
			"/settings",
		]) {
			await page.goto(path);
			await expect(page.getByTestId("topbar"), path).toBeVisible();
			await expect(page.getByTestId("scene-bar"), path).toBeVisible();
			await expect(
				page.getByRole("navigation", { name: "Main navigation" }),
				path,
			).toBeAttached();
			// at the end of the page, the main region ends where the bar begins: nothing is under it
			await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
			const main = await page.locator("main").boundingBox();
			const bar = await page.getByTestId("scene-bar").boundingBox();
			expect(main && bar && main.y + main.height <= bar.y + 1, path).toBe(true);
			// the bar is 56 px high on a wide window
			if ((page.viewportSize()?.width ?? 0) >= 768) {
				expect(bar?.height).toBe(56);
			}
		}
	});

	test("with no agent session the bar says so and invents no skill, tool or step", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `Bare ${unique(browserName)}`);
		await signInAsPerson(page);
		await clearProfiles(page);
		await page.goto(`/projects/${project}`);
		const bar = page.getByTestId("scene-bar");
		await expect(bar).toHaveAttribute("data-variant", "none");
		await expect(bar).toContainText("No agent session");
		await expect(page.getByTestId("scene-skill")).toHaveCount(0);
		await expect(page.getByTestId("scene-tool")).toHaveCount(0);
		await expect(page.getByTestId("scene-allow")).toHaveCount(0);
		await expect(bar).toContainText("Open chat");
	});

	test("a running fake agent that calls a tool shows the state and the tool title as plain text", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `Tool ${unique(browserName)}`);
		await signInAsPerson(page);
		await clearProfiles(page);
		const session = await converse(page, project, "tooler", "echo");
		await page.goto(`/projects/${project}`);
		await expect(page.getByTestId("scene-bar")).toHaveAttribute(
			"data-variant",
			"idle",
		);
		await expect(page.getByTestId("scene-agent")).toContainText("tooler");
		await say(page, session, "look at the project");
		const tool = page.getByTestId("scene-tool");
		await expect(tool).toContainText("Reading the project");
		await expect(page.getByTestId("scene-state")).toContainText(
			"Waiting for you",
		);
		// plain text: the skill is not reported and no markup was made of the title
		await expect(page.getByTestId("scene-skill")).toContainText("Not reported");
		await expect(tool.locator("a, img, b, script")).toHaveCount(0);
	});

	test("an open permission is answered on the bar, and the conversation shows the same answer", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `Allow ${unique(browserName)}`);
		await signInAsPerson(page);
		await clearProfiles(page);
		const session = await converse(page, project, "asker", "permission");
		await page.goto(`/projects/${project}`);
		await expect(page.getByTestId("scene-bar")).toHaveAttribute(
			"data-variant",
			"idle",
		);
		await say(page, session, "delete the clips");
		const bar = page.getByTestId("scene-bar");
		await expect(bar).toHaveAttribute("data-variant", "waiting");
		await expect(page.getByTestId("scene-state")).toContainText(
			"Asks your permission",
		);
		// the agent offers "Always allow": the bar has no such action
		await expect(bar.getByRole("button", { name: /always/i })).toHaveCount(0);
		await expect(bar.getByRole("button")).toHaveText(["Deny", "Allow once"]);
		const answered = page.waitForResponse(
			(res) =>
				res.url().includes(`/chat/sessions/${session}/permissions/`) &&
				res.request().method() === "POST",
		);
		await page.getByTestId("scene-allow").click();
		expect((await answered).ok()).toBe(true);
		await expect(bar).not.toHaveAttribute("data-variant", "waiting");
		await expect(page.getByTestId("scene-allow")).toHaveCount(0);
		// the conversation shows the same answer
		await page.getByTestId("chat-toggle").click();
		await expect(page.getByTestId("chat-answer")).toContainText("You allowed:");
		await expect(page.getByTestId("chat-agent").last()).toContainText(
			"permission: selected once",
		);
	});

	test("denying on the bar refuses the request and the bar says it was denied", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `Deny ${unique(browserName)}`);
		await signInAsPerson(page);
		await clearProfiles(page);
		const session = await converse(page, project, "refuser", "permission");
		await page.goto(`/projects/${project}`);
		await expect(page.getByTestId("scene-bar")).toHaveAttribute(
			"data-variant",
			"idle",
		);
		await say(page, session, "delete the clips");
		await expect(page.getByTestId("scene-bar")).toHaveAttribute(
			"data-variant",
			"waiting",
		);
		await page.getByTestId("scene-deny").click();
		await expect(page.getByTestId("scene-bar")).toHaveAttribute(
			"data-variant",
			"denied",
		);
		await expect(page.getByTestId("scene-state")).toContainText(
			"You denied it",
		);
		await page.getByTestId("chat-toggle").click();
		await expect(page.getByTestId("chat-answer")).toContainText(
			"You rejected:",
		);
	});
});

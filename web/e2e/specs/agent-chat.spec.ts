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
import { readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { caller, expect, home, test } from "./helpers";

// SDD-017, epic 14: the agent the user hosts, in the panel of the right dock. The agent here is the
// fake agent of the tests (service/agenthost/fakeagent), a program that speaks the Agent Client
// Protocol, answers on request and misbehaves on request; a real agent is a manual check. The
// server is the real one, so what the agent edits goes through its MCP endpoint with its own token.
test.describe.configure({ mode: "serial" });

const root = fileURLToPath(new URL("../../..", import.meta.url));

let fakeAgent = "";

// The agent profiles and the live sessions (at most 4) belong to the whole workspace, which every
// browser project of a run shares, and this file asserts what the workspace holds ("no agent is
// connected"). So one browser holds the workspace for the whole file: the others wait for the lock,
// a file that names the worker holding it; the lock of a worker that is gone is taken over.
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
			return false; // released between the two calls: the next poll takes it
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

// Every project a test made, so that the sessions it left running end with it.
const made: string[] = [];

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
		const ended = await page.request.post(
			`/api/v1/chat/sessions/${id}/cancel`,
			{
				data: { end: true },
				headers: {
					"Content-Type": "application/json",
					"Sec-Fetch-Site": "same-origin",
				},
			},
		);
		expect(ended.ok()).toBe(true);
	}
});

test.beforeAll(() => {
	const dir = mkdtempSync(join(tmpdir(), "stuarty-e2e-fakeagent-"));
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

// A person writes profiles and drives a chat: the session of the desktop app, whose launch carries
// the launcher secret (SDD-007 section 3.4 item 7).
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

const unique = (browserName: string) =>
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

// A person removes every agent profile, so that a run on a workspace that already has some starts
// from none. A change made with the session cookie says where it comes from and that it is JSON.
async function clearProfiles(page: Page): Promise<void> {
	const headers = {
		"Content-Type": "application/json",
		"Sec-Fetch-Site": "same-origin",
	};
	const listed = await page.request.get("/api/v1/agents/profiles");
	expect(listed.ok()).toBe(true);
	const { profiles } = (await listed.json()) as {
		profiles: { name: string }[];
	};
	for (const p of profiles) {
		const gone = await page.request.delete(
			`/api/v1/agents/profiles/${encodeURIComponent(p.name)}`,
			{ headers },
		);
		expect(gone.ok()).toBe(true);
	}
}

async function openPanel(page: Page): Promise<void> {
	await page.goto("/projects");
	await page.getByTestId("chat-toggle").click();
	await expect(page.getByTestId("chat-panel")).toBeVisible();
}

async function connect(
	page: Page,
	name: string,
	mode: string,
	extra: string[] = [],
): Promise<void> {
	// With no agent connected the panel opens on the setup; with one, the setup is behind a button.
	const manage = page.getByTestId("chat-manage");
	// The panel loads the profiles first: wait for whichever it shows.
	await expect(manage.or(page.getByTestId("chat-setup"))).toBeVisible();
	if (await manage.isVisible()) await manage.click();
	await expect(page.getByTestId("chat-setup")).toBeVisible();
	await page
		.getByTestId("chat-panel")
		.getByLabel("Name", { exact: true })
		.fill(name);
	await page
		.getByTestId("chat-panel")
		.getByLabel(/^Program/)
		.fill(fakeAgent);
	await page
		.getByLabel("Arguments, one per line")
		.fill([`--mode=${mode}`, ...extra].join("\n"));
	// the full command is shown before it is saved
	await expect(page.getByTestId("chat-full-command")).toContainText(
		`--mode=${mode}`,
	);
	await page.getByRole("button", { name: "Connect the agent" }).click();
	// The first agent takes the panel to the conversation by itself; later ones leave it on the setup.
	if (await manage.isVisible()) {
		await expect(
			page.getByTestId("chat-profile").filter({ hasText: name }),
		).toBeVisible();
		if (await page.getByTestId("chat-setup").isVisible()) await manage.click();
	}
	await expect(
		page
			.getByLabel("Agent", { exact: true })
			.locator("option", { hasText: name }),
	).toHaveCount(1);
}

async function startWith(
	page: Page,
	project: string,
	profile: string,
): Promise<void> {
	await page
		.getByTestId("chat-panel")
		.getByLabel("Project")
		.selectOption(project);
	await page
		.getByTestId("chat-panel")
		.getByLabel("Agent", { exact: true })
		.selectOption(profile);
	await page
		.getByRole("button", { name: /Start a conversation|New session/ })
		.click();
	await expect(page.getByTestId("chat-state")).toContainText("Waiting for you");
	await expect(page.getByTestId("chat-state")).toContainText("Live");
}

async function say(page: Page, text: string): Promise<void> {
	await page.getByLabel("Message to the agent").fill(text);
	await page.getByRole("button", { name: "Send" }).click();
}

test.describe("agent chat", () => {
	test("the panel opens from every section and says when no agent is connected", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		await makeProject(api, `Chat ${unique(browserName)}`);
		await signInAsPerson(page);
		await clearProfiles(page);
		for (const path of ["/", "/projects", "/memory", "/connections"]) {
			await page.goto(path);
			await expect(page.getByTestId("chat-toggle")).toBeVisible();
		}
		await openPanel(page);
		await expect(page.getByTestId("chat-setup")).toContainText(
			"never installs an agent",
		);
		await expect(page.getByTestId("chat-suggestion").first()).toBeVisible();
		// the form shows the full command before it is saved, and the panel closes with Escape
		await page
			.getByTestId("chat-panel")
			.getByLabel(/^Program/)
			.fill("agent");
		await page
			.getByTestId("chat-panel")
			.getByLabel("Arguments, one per line")
			.fill("acp\n--x y");
		await expect(page.getByTestId("chat-full-command")).toHaveText(
			'agent acp "--x y"',
		);
		await page.keyboard.press("Escape");
		await expect(page.getByTestId("chat-panel")).toBeHidden();
		await expect(page.getByTestId("chat-toggle")).toBeFocused();
	});

	test("a conversation shows the messages as they arrive, and an edit of the agent is in the log and can be undone", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `Edit ${unique(browserName)}`);
		await signInAsPerson(page);
		await openPanel(page);
		await connect(page, "mcp", "mcp");
		await startWith(page, project, "mcp");

		const edit = `mcp edit {"project_id":"${project}","base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual"}}]}`;
		await say(page, edit);
		await expect(page.getByTestId("chat-user").last()).toContainText(
			"mcp edit",
		);
		await expect(page.getByTestId("chat-agent").last()).toContainText(
			"mcp 200",
		);
		await expect(page.getByTestId("chat-state")).toContainText(
			"Waiting for you",
		);

		// The edit is in the operation log with the agent as the actor (RF-381) ...
		const history = (await api("GET", `/projects/${project}/history`)) as {
			entries: { actor: string; revision: number }[];
		};
		const agentStep = history.entries.find((e) =>
			e.actor.startsWith("local:chat-"),
		);
		expect(agentStep).toBeDefined();

		// ... the project is at revision 1 ...
		const got = await api("GET", `/projects/${project}`);
		expect(got.revision).toBe(1);

		// ... and the edit is undone through the same MCP endpoint: undo is scoped to the actor, so
		// the agent undoes its own step, which is in the log as a step of the agent too.
		await say(page, `mcp history {"project_id":"${project}","action":"undo"}`);
		await expect(page.getByTestId("chat-agent").last()).toContainText(
			"undid r1",
		);
		await expect(page.getByTestId("chat-state")).toContainText(
			"Waiting for you",
		);
		expect((await api("GET", `/projects/${project}`)).revision).toBe(2);
		const after = (await api("GET", `/projects/${project}/history`)) as {
			entries: { actor: string; revision: number }[];
		};
		expect(
			after.entries.some(
				(e) => e.revision === 2 && e.actor === agentStep?.actor,
			),
		).toBe(true);

		// ... and an edit of another project is refused: the token is bound to this one.
		const other = await makeProject(api, `Other ${unique(browserName)}`);
		await say(
			page,
			`mcp edit {"project_id":"${other}","base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual"}}]}`,
		);
		await expect(page.getByTestId("chat-agent").last()).toContainText(
			"E_FORBIDDEN",
		);
		expect((await api("GET", `/projects/${other}`)).revision).toBe(0);
	});

	test("a permission request waits for the person, who allows it once", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `Ask ${unique(browserName)}`);
		await signInAsPerson(page);
		await openPanel(page);
		await connect(page, "ask", "permission");
		await startWith(page, project, "ask");
		await say(page, "delete the clips");

		const ask = page.getByTestId("chat-permission");
		await expect(ask).toBeVisible();
		await expect(page.getByTestId("chat-state")).toContainText(
			"Asking your permission",
		);
		// the title is the agent's text: the markup in it is shown, not run
		await expect(page.getByTestId("chat-permission-title")).toHaveText(
			"Delete every clip <b>now</b>",
		);
		await expect(ask.locator("b")).toHaveCount(0);
		// publications are decided elsewhere, with a way to get there
		await expect(
			ask.getByRole("link", { name: "Open Approvals" }),
		).toHaveAttribute("href", "/approvals");
		// a standing choice is never offered
		await expect(ask.getByRole("button", { name: /always/i })).toHaveCount(0);
		await ask.getByRole("button", { name: "Allow once" }).click();
		await expect(page.getByTestId("chat-agent").last()).toContainText(
			"permission: selected once",
		);
		await expect(page.getByTestId("chat-answer")).toBeVisible();
		await expect(ask.getByRole("button", { name: "Allow once" })).toHaveCount(
			0,
		);
	});

	test("what the agent writes is text: markup is shown, nothing runs", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `Markup ${unique(browserName)}`);
		await signInAsPerson(page);
		await page.addInitScript(() => {
			(window as unknown as { __pwned?: number }).__pwned = 0;
		});
		await openPanel(page);
		await connect(page, "markup", "markup");
		await startWith(page, project, "markup");
		await say(page, "go");
		const message = page.getByTestId("chat-agent").last();
		await expect(message).toContainText("<img src=x onerror=alert(1)>");
		await expect(message).toContainText("[a](javascript:alert(1))");
		await expect(
			page.getByTestId("chat-log").locator("img, a, script"),
		).toHaveCount(0);
		await expect(message.locator("h1, h2, h3")).toHaveCount(0);
	});

	test("a turn can be stopped, and the transcript is exported as Markdown and deleted", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `Stop ${unique(browserName)}`);
		await signInAsPerson(page);
		await openPanel(page);
		await connect(page, "stoppable", "cancellable");
		await startWith(page, project, "stoppable");
		await say(page, "keep going");
		await expect(page.getByTestId("chat-state")).toContainText("Working");
		await page.getByRole("button", { name: "Stop", exact: true }).click();
		await expect(page.getByTestId("chat-notice").last()).toContainText(
			"The turn was cancelled.",
		);
		await expect(page.getByTestId("chat-state")).toContainText(
			"Waiting for you",
		);

		const download = page.waitForEvent("download");
		await page.getByRole("button", { name: "Export as Markdown" }).click();
		const file = await (await download).path();
		const markdown = readFileSync(file, "utf8");
		expect(markdown).toContain("# Chat session cht_");
		expect(markdown).toContain("keep going");

		await page.getByRole("button", { name: "Delete the transcript" }).click();
		await page.getByRole("button", { name: "Delete for good" }).click();
		await expect(page.getByTestId("chat-state")).toContainText(
			"No conversation yet",
		);
		const sessions = (await api(
			"GET",
			`/projects/${project}/chat/sessions`,
		)) as {
			sessions: unknown[];
		};
		expect(sessions.sessions).toHaveLength(0);
	});

	test("a new session replaces the old one, and the agent is gone when it ends", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `End ${unique(browserName)}`);
		await signInAsPerson(page);
		await openPanel(page);
		await connect(page, "echo", "echo");
		await startWith(page, project, "echo");
		await say(page, "hello");
		await expect(page.getByTestId("chat-agent").last()).toContainText("echo:");
		await page.getByRole("button", { name: "End the session" }).click();
		await expect(page.getByTestId("chat-state")).toContainText("Ended");
		await expect(page.getByTestId("chat-state")).toContainText("ended by you");
		await expect(page.getByLabel("Message to the agent")).toBeDisabled();
		const list = (await api("GET", `/projects/${project}/chat/sessions`)) as {
			sessions: { live: boolean; state: string }[];
		};
		expect(list.sessions[0]?.live).toBe(false);
		expect(list.sessions[0]?.state).toBe("ended");
	});

	test("the local token of an agent cannot drive a chat or write a profile", async ({
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const project = await makeProject(api, `Token ${unique(browserName)}`);
		for (const [method, path, data] of [
			["PUT", "/agents/profiles/evil", { command: fakeAgent }],
			["POST", `/projects/${project}/chat/sessions`, { profile: "echo" }],
		] as const) {
			await expect(api(method, path, data)).rejects.toThrow(/403/);
		}
	});

	test("the log keeps its list and passes axe once it holds events", async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(browserName === "webkit", "axe runs on Chromium and Firefox"); // verify:env-skip axe is not run on WebKit
		const api = await caller(request);
		const project = await makeProject(api, `Log ${unique(browserName)}`);
		await signInAsPerson(page);
		await openPanel(page);
		await connect(page, "echo", "echo");
		await startWith(page, project, "echo");
		await say(page, "hello");
		await expect(page.getByTestId("chat-agent").last()).toContainText("echo:");
		const log = page.getByRole("log", { name: /conversation|conversa/i });
		await expect(log).toBeVisible();
		await expect(log.getByRole("list")).toBeVisible();
		await expect(log.getByRole("listitem").first()).toBeVisible();
		const results = await new AxeBuilder({ page })
			.include("#chat-panel")
			.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
			.analyze();
		expect(results.violations).toEqual([]);
	});

	test("the panel passes axe, in English and in Portuguese, and has labels for a screen reader", async ({
		page,
		browserName,
	}) => {
		test.skip(browserName === "webkit", "axe runs on Chromium and Firefox"); // verify:env-skip axe is not run on WebKit
		for (const lang of ["en", "pt-BR"] as const) {
			await page.addInitScript((l) => localStorage.setItem("vv.lang", l), lang);
			await signInAsPerson(page);
			await page.goto("/projects");
			await page
				.getByRole("button", {
					name: /Chat with your agent|Conversar com o seu agente/,
				})
				.click();
			const panel = page.getByRole("complementary");
			await expect(panel).toBeVisible();
			const results = await new AxeBuilder({ page })
				.include("#chat-panel")
				.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
				.analyze();
			expect(results.violations).toEqual([]);
			await page.reload();
		}
	});
});

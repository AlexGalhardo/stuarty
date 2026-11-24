import { execFileSync } from "node:child_process";
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
import { expect, home, seedProject, test } from "./helpers";

// Epic 85 task 5 (SDD-012 section 23.4): the scene bar of the dashboard under the timeline of the editor, so a
// person answers the agent without leaving. The agent is the fake agent of the tests (service/agenthost/fakeagent)
// and the server is the real one: the page of the editor carries the session the server shares with every other
// page, and the catalogue of the bar.
test.describe.configure({ mode: "serial" });
test.use({ viewport: { width: 1440, height: 900 } });

const root = fileURLToPath(new URL("../../..", import.meta.url));

let fakeAgent = "";

// The agent profiles and the live sessions belong to the whole workspace, which every browser project of a run
// shares; scene-bar.spec.ts and agent-chat.spec.ts take the same lock.
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
	const dir = mkdtempSync(join(tmpdir(), "stuarty-e2e-editorscene-"));
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

const JSON_HEADERS = {
	"Content-Type": "application/json",
	"Sec-Fetch-Site": "same-origin",
};

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

// The bootstrap the server wrote into the page of the editor.
async function bootstrapOf(page: Page): Promise<{
	session?: { privileged?: unknown };
	messages: Record<string, unknown>;
}> {
	const text = await page.locator("script#vv-bootstrap").textContent();
	return JSON.parse(text ?? "{}");
}

test.describe("the scene bar in the editor", () => {
	test("shows the agent, its state and its tool under the timeline, and a permission is answered without leaving", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await signInAsPerson(page);
		await clearProfiles(page);
		await send(page, "PUT", "/agents/profiles/editorial", {
			command: fakeAgent,
			args: ["--mode=permission"],
		});
		const session = (await send(
			page,
			"POST",
			`/projects/${seeded.id}/chat/sessions`,
			{ profile: "editorial" },
		)) as { id: string };
		await page.goto(`/editor/${seeded.id}`);
		// the server, not the test, says this session is a person's
		expect((await bootstrapOf(page)).session).toEqual({ privileged: true });

		const bar = page.getByTestId("scene-bar");
		await expect(bar).toHaveAttribute("data-variant", "idle");
		await expect(page.getByTestId("scene-agent")).toContainText("editorial");
		// the bar is under the timeline, in the window, and the timeline keeps its place
		const timeline = page.locator("[data-landmark='4']").first();
		const barBox = await bar.boundingBox();
		expect(barBox?.height).toBe(56);
		expect((barBox?.y ?? 0) + (barBox?.height ?? 0)).toBeLessThanOrEqual(900);
		await expect(
			page.getByRole("tablist", { name: "Material panels" }),
		).toBeVisible();
		await expect(timeline).toBeVisible();

		await send(page, "POST", `/chat/sessions/${session.id}/messages`, {
			text: "delete the clips",
		});
		await expect(bar).toHaveAttribute("data-variant", "waiting");
		await expect(page.getByTestId("scene-state")).toContainText(
			"Asks your permission",
		);
		await expect(bar.getByRole("button")).toHaveText(["Deny", "Allow once"]);
		const answered = page.waitForResponse(
			(res) =>
				res.url().includes(`/chat/sessions/${session.id}/permissions/`) &&
				res.request().method() === "POST",
		);
		await page.getByTestId("scene-allow").click();
		expect((await answered).ok()).toBe(true);
		await expect(bar).not.toHaveAttribute("data-variant", "waiting");
		// still in the editor
		await expect(page).toHaveURL(new RegExp(`/editor/${seeded.id}`));
		await send(page, "POST", `/chat/sessions/${session.id}/cancel`, {
			end: true,
		});
	});

	test("the page carries the catalogue of the bar, which is drawn in the words of the locale", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await signInAsPerson(page);
		await page.goto(`/editor/${seeded.id}`);
		const { messages } = await bootstrapOf(page);
		for (const key of [
			"ui.shell.scene.allow",
			"ui.shell.scene.deny",
			"ui.shell.scene.label",
		]) {
			expect(messages[key], key).toEqual(expect.any(String));
		}
		await expect(
			page.getByRole("tablist", { name: "Material panels" }),
		).toBeVisible();
		const bar = page.getByTestId("scene-bar");
		await expect(bar).toBeVisible();
		// no raw message key is drawn
		await expect(bar).not.toContainText("ui.");
	});
});

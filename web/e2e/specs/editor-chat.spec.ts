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

// VV-427: the agent chat works on the page of the video editor. The panel opens from the scene bar, over the
// editor and on the right edge, without moving the editor; a person types and sends in it, and the shortcuts of
// the editor do not take the keys. The agent is the fake agent of the tests and the server is the real one.
test.describe.configure({ mode: "serial" });
test.use({ viewport: { width: 1440, height: 900 } });

const root = fileURLToPath(new URL("../../..", import.meta.url));

let fakeAgent = "";

// The agent profiles and the live sessions belong to the whole workspace, which every browser project of a run
// shares; the other chat specs take the same lock.
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
	const dir = mkdtempSync(join(tmpdir(), "stuarty-e2e-editorchat-"));
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

test.describe("the agent chat in the editor", () => {
	test("opens over the editor, takes the typing, sends, and closes back to the bar", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await ensureAccount(page.request, home);
		await signInWithPassword(page.request);
		await clearProfiles(page);
		await send(page, "PUT", "/agents/profiles/editorial", {
			command: fakeAgent,
			args: ["--mode=echo"],
		});
		const session = (await send(
			page,
			"POST",
			`/projects/${seeded.id}/chat/sessions`,
			{ profile: "editorial" },
		)) as { id: string };

		await page.goto(`/editor/${seeded.id}`);
		const regions = page.getByRole("tablist", { name: "Material panels" });
		await expect(regions).toBeVisible();
		const before = await regions.boundingBox();
		const timeline = page.locator("[data-landmark='4']").first();
		const timelineBefore = await timeline.boundingBox();

		// a clip is selected, so the keys of the editor (S, Delete) would have something to act on
		await page.locator("[data-item-id]").first().click();
		const start = await page.evaluate(() => window.__vv?.revision ?? null);
		expect(start).not.toBeNull();

		await page.getByTestId("scene-open-chat").click();
		const panel = page.getByTestId("chat-panel");
		await expect(panel).toBeVisible();
		// still the editor: no navigation to the project page
		await expect(page).toHaveURL(new RegExp(`/editor/${seeded.id}`));

		// the panel is on top of the editor: the point at its center is the panel or a descendant of it
		const top = await panel.evaluate((el) => {
			const r = el.getBoundingClientRect();
			const hit = document.elementFromPoint(
				r.left + r.width / 2,
				r.top + r.height / 2,
			);
			return hit !== null && el.contains(hit);
		});
		expect(top).toBe(true);
		// ... and so are the controls of it that sit over the editor
		const composer = panel.getByLabel("Message to the agent");
		for (const target of [page.getByTestId("chat-close"), composer]) {
			const hit = await target.evaluate((el) => {
				const r = el.getBoundingClientRect();
				const at = document.elementFromPoint(
					r.left + r.width / 2,
					r.top + r.height / 2,
				);
				return at !== null && el.contains(at);
			});
			expect(hit).toBe(true);
		}
		// the panel sits on the right edge of the window
		const box = await panel.boundingBox();
		expect((box?.x ?? 0) + (box?.width ?? 0)).toBeGreaterThanOrEqual(1439);

		// the editor did not move
		expect(await regions.boundingBox()).toEqual(before);
		expect(await timeline.boundingBox()).toEqual(timelineBefore);

		// typing in the panel: the keys of the editor (J K L, S, M, ?, Delete) stay out of the editor
		await composer.click();
		await page.keyboard.type("jkl s m ? ", { delay: 5 });
		await page.keyboard.press("Delete");
		await page.keyboard.press("Backspace");
		await expect(composer).toHaveValue("jkl s m ?");
		expect(await page.evaluate(() => window.__vv?.revision ?? null)).toBe(
			start,
		);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await expect(page.getByRole("dialog")).toHaveCount(0);

		await composer.fill("hello from the editor");
		await page.keyboard.press("Enter");
		// the fake agent answers a turn by echoing what it was sent
		await expect(panel.getByTestId("chat-log")).toContainText(
			/hello from the editor[\s\S]*echo: [\s\S]*hello from the editor/,
		);

		// the composer is disabled while the turn runs, which drops its focus: wait for the turn to end, then
		// Escape from inside the panel closes it and the focus goes back to the bar
		await expect(composer).toBeEnabled();
		await composer.focus();
		await page.keyboard.press("Escape");
		await expect(panel).toHaveCount(0);
		await expect(page.getByTestId("scene-open-chat")).toBeFocused();

		await send(page, "POST", `/chat/sessions/${session.id}/cancel`, {
			end: true,
		});
	});
});

test.describe("the project of the chat panel", () => {
	test("a project that is gone is dropped: the panel picks one that exists and shows no Not found", async ({
		page,
		request,
	}) => {
		const gone = await seedProject(request, { tracks: 1, perTrack: 1 });
		await seedProject(request, { tracks: 1, perTrack: 1 });
		await ensureAccount(page.request, home);
		await signInWithPassword(page.request);
		await clearProfiles(page);
		await send(page, "PUT", "/agents/profiles/editorial", {
			command: fakeAgent,
			args: ["--mode=echo"],
		});
		// a trashed project is still listed and still answers; an emptied one is gone
		await send(page, "POST", `/projects/${gone.id}/trash`, {});
		await send(page, "POST", "/trash/empty", { project_id: gone.id });
		// the list of projects answers after the read of the session of the missing one, which is the order
		// that left the error on the screen
		const sessionsAnswered = page.waitForResponse((res) =>
			res.url().includes(`/projects/${gone.id}/chat/sessions`),
		);
		await page.route("**/api/v1/projects", async (route) => {
			await sessionsAnswered;
			await route.continue();
		});
		// the address names the project that is gone
		await page.goto(`/projects?from=${gone.id}`);
		await page.getByTestId("chat-toggle").click();
		const panel = page.getByTestId("chat-panel");
		const select = panel.getByLabel("Project", { exact: true });
		await expect(select).toBeVisible();
		await expect(select.locator("option")).not.toHaveCount(0);
		await expect(select).not.toHaveValue(gone.id);
		await expect(select).not.toHaveValue("");
		await expect(panel).not.toContainText("there is no project");
		await expect(panel).not.toContainText("Not found");
	});
});

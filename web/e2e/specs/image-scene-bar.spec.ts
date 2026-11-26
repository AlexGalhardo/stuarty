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
import { editImage, newImage } from "./image-support";

// Epic 86 task 5 (SDD-012 sections 23.4 and 24): the scene bar of the dashboard at the foot of the image editor,
// so a person answers the agent without leaving the image. The agent is the fake agent of the tests
// (internal/tools/fakeagent) and the server is the real one.
test.describe.configure({ mode: "serial" });
test.use({ viewport: { width: 1440, height: 900 } });

const root = fileURLToPath(new URL("../../..", import.meta.url));

let fakeAgent = "";

// The agent profiles and the live sessions belong to the whole workspace, which every browser project of a run
// shares; scene-bar.spec.ts, editor-scene-bar.spec.ts and agent-chat.spec.ts take the same lock.
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
	const dir = mkdtempSync(join(tmpdir(), "stuarty-e2e-imagescene-"));
	mkdirSync(dir, { recursive: true });
	fakeAgent = join(
		dir,
		process.platform === "win32" ? "fakeagent.exe" : "fakeagent",
	);
	execFileSync("go", ["build", "-o", fakeAgent, "./internal/tools/fakeagent"], {
		cwd: root,
	});
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

test.describe("the scene bar in the image editor", () => {
	test("shows the agent and its permission request at the foot of the editor, which is answered without leaving", async ({
		page,
		request,
	}) => {
		const project = await seedProject(request, { tracks: 1, perTrack: 1 });
		const { id: imageId, call } = await newImage(request, {
			name: "Scene bar spec",
		});
		await editImage(call, imageId, [
			{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		]);
		await signInAsPerson(page);
		await clearProfiles(page);
		await send(page, "PUT", "/agents/profiles/editorial", {
			command: fakeAgent,
			args: ["--mode=permission"],
		});
		const session = (await send(
			page,
			"POST",
			`/projects/${project.id}/chat/sessions`,
			{ profile: "editorial" },
		)) as { id: string };
		// the page names no project, so the chat talks about the one it remembers
		await page.addInitScript((value) => {
			localStorage.setItem("stuarty.chat.project", value);
		}, project.id);
		await page.goto(`/edit-image/${imageId}`);
		await expect(page.getByTestId("image-frame")).toBeVisible({
			timeout: 30_000,
		});

		const bar = page.getByTestId("scene-bar");
		await expect(bar).toHaveAttribute("data-variant", "idle");
		await expect(page.getByTestId("scene-agent")).toContainText("editorial");
		// the bar is the last thing in the window, and the editor ends where it begins: the page does not scroll
		const editor = await page.getByTestId("image-editor").boundingBox();
		const barBox = await bar.boundingBox();
		expect(barBox?.height).toBe(56);
		expect(barBox?.y ?? 0).toBeGreaterThanOrEqual(
			(editor?.y ?? 0) + (editor?.height ?? 0),
		);
		expect((barBox?.y ?? 0) + (barBox?.height ?? 0)).toBeLessThanOrEqual(900);
		await expect(
			page.evaluate(
				() =>
					document.documentElement.scrollHeight <=
					document.documentElement.clientHeight,
			),
		).resolves.toBe(true);

		await send(page, "POST", `/chat/sessions/${session.id}/messages`, {
			text: "delete the layers",
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
		// still in the image
		await expect(page).toHaveURL(new RegExp(`/edit-image/${imageId}`));
		await expect(page.getByTestId("image-frame")).toBeVisible();
		await send(page, "POST", `/chat/sessions/${session.id}/cancel`, {
			end: true,
		});
	});
});

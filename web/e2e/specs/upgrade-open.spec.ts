import { type ChildProcess, execFileSync, spawn } from "node:child_process";
import {
	cpSync,
	existsSync,
	mkdirSync,
	readdirSync,
	readFileSync,
	rmSync,
} from "node:fs";
import { readFile } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { DatabaseSync } from "node:sqlite";
import { fileURLToPath } from "node:url";
import {
	type APIRequestContext,
	test as base,
	expect,
	type Page,
	request as playwrightRequest,
} from "@playwright/test";

// P4-UI-002 (SDD-012 sections 2.2 and 5.6, RF-001, NFR-012): the workspace that
// each released version wrote, opened in the editor of this build.
//
// The workspaces are made by the upgrade job (scripts/upgrade-test.sh, P4-REL-002):
// one folder per released version under VUKEM_UPGRADE_WORKSPACES, each with the
// scenario.txt that lists the steps that release could do. Without the variable the
// spec skips. Each version runs on a copy of its folder, on a server of its own
// started from this checkout, so no run changes the folder and no spec shares a
// workspace.
const WORKSPACES = process.env.VUKEM_UPGRADE_WORKSPACES;

const MAIN = "prj_1000000001";
const STARTER = "prj_1000000002";
const CLIP = "itm_1000000001";
const CAPTION = "itm_1000000002";
const TITLE = "itm_1000000003";

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");
const repoRoot = resolve(webRoot, "..");

function versions(): string[] {
	if (!WORKSPACES || !existsSync(WORKSPACES)) return [];
	return readdirSync(WORKSPACES, { withFileTypes: true })
		.filter((e) => e.isDirectory() && /^v\d+\.\d+\.\d+$/.test(e.name))
		.map((e) => e.name)
		.sort((a, b) => {
			const pa = a.slice(1).split(".").map(Number);
			const pb = b.slice(1).split(".").map(Number);
			return (
				(pa[0] ?? 0) - (pb[0] ?? 0) ||
				(pa[1] ?? 0) - (pb[1] ?? 0) ||
				(pa[2] ?? 0) - (pb[2] ?? 0)
			);
		});
}

// scenario.txt: step name to true when that release did the step.
function scenario(dir: string): Record<string, boolean> {
	const steps: Record<string, boolean> = {};
	for (const line of readFileSync(join(dir, "scenario.txt"), "utf8")
		.split(/\r?\n/)
		.filter((l) => l.trim() !== "")) {
		const [name = "", status = ""] = line.trim().split(" ");
		steps[name] = status === "ok";
	}
	return steps;
}

function freePort(): Promise<number> {
	return new Promise((done, fail) => {
		const probe = createServer();
		probe.on("error", fail);
		probe.listen(0, "127.0.0.1", () => {
			const address = probe.address();
			const port = typeof address === "object" && address ? address.port : 0;
			probe.close(() => done(port));
		});
	});
}

function ffmpegEnv(): Record<string, string> {
	const dir = process.env.STUARTY_TEST_FFMPEG_DIR;
	if (!dir) return {};
	const exe = process.platform === "win32" ? ".exe" : "";
	const bin = existsSync(join(dir, "bin", `ffmpeg${exe}`))
		? join(dir, "bin")
		: dir;
	return {
		STUARTY_FFMPEG: join(bin, `ffmpeg${exe}`),
		STUARTY_FFPROBE: join(bin, `ffprobe${exe}`),
	};
}

// The candidate: this checkout, with the end-to-end build of the editor embedded
// (the same one the webServer of playwright.config.ts runs).
let candidate: string | null = null;
function candidateBinary(): string {
	if (candidate) return candidate;
	const dir = join(tmpdir(), `stuarty-upgrade-open-${process.pid}`);
	mkdirSync(dir, { recursive: true });
	const out = join(
		dir,
		process.platform === "win32" ? "stuarty.exe" : "stuarty",
	);
	execFileSync("go", ["build", "-trimpath", "-o", out, "./web/cmd/app"], {
		cwd: repoRoot,
		stdio: "inherit",
	});
	candidate = out;
	return out;
}

interface Running {
	home: string;
	baseURL: string;
	stop(): Promise<void>;
	api: APIRequestContext;
}

interface Started {
	baseURL: string;
	output(): string;
	exited(): Promise<number | null>;
	kill(): Promise<void>;
}

// The candidate on a workspace, on a free port. The caller waits for /up or for
// the exit.
async function start(home: string): Promise<Started> {
	const port = await freePort();
	const child: ChildProcess = spawn(candidateBinary(), ["serve"], {
		env: {
			...process.env,
			APP_KEY: `base64:${Buffer.alloc(32, "e2e").toString("base64")}`,
			HOST: "127.0.0.1",
			PORT: String(port),
			STUARTY_HOME: home,
			STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE: "1000",
			STUARTY_RATE_LIMIT_PER_MINUTE: "100000",
			STUARTY_RATE_LIMIT_SESSION_PER_MINUTE: "100000",
			...ffmpegEnv(),
		},
		stdio: ["ignore", "pipe", "pipe"],
	});
	let output = "";
	child.stdout?.on("data", (d: Buffer) => {
		output += d.toString();
	});
	child.stderr?.on("data", (d: Buffer) => {
		output += d.toString();
	});
	const exited = new Promise<number | null>((done) =>
		child.once("exit", (code) => done(code)),
	);
	return {
		baseURL: `http://127.0.0.1:${port}`,
		output: () => output,
		exited: () => exited,
		kill: async () => {
			if (child.exitCode === null) child.kill();
			await exited;
		},
	};
}

// A copy of a workspace nobody holds: the lock, the runtime file and the token
// belong to a process.
function copyWorkspace(label: string, source: string): string {
	const home = join(tmpdir(), `stuarty-upgrade-open-${process.pid}-${label}`);
	cpSync(source, home, {
		recursive: true,
		filter: (path) =>
			!["lock", "runtime.json", "token"].includes(basename(path)),
	});
	return home;
}

async function serve(label: string, source: string): Promise<Running> {
	const home = copyWorkspace(label, source);
	const started = await start(home);
	const deadline = Date.now() + 60_000;
	for (;;) {
		const gone = await Promise.race([
			started.exited(),
			new Promise<"running">((r) => setTimeout(() => r("running"), 250)),
		]);
		if (gone !== "running") {
			throw new Error(
				`the candidate exited with ${gone} on ${label}:\n${started.output()}`,
			);
		}
		try {
			if ((await fetch(`${started.baseURL}/up`)).ok) break;
		} catch {
			// Not listening yet.
		}
		if (Date.now() > deadline) {
			await started.kill();
			throw new Error(
				`the candidate did not start on ${label}:\n${started.output()}`,
			);
		}
	}
	const token = (await readFile(join(home, "token"), "utf8")).trim();
	const api = await playwrightRequest.newContext({
		baseURL: started.baseURL,
		extraHTTPHeaders: { Authorization: `Bearer ${token}` },
	});
	return {
		home,
		baseURL: started.baseURL,
		api,
		stop: async () => {
			await api.dispose();
			await started.kill();
			rmSync(home, { recursive: true, force: true });
		},
	};
}

// What a build that saves schema 999 would have left: the document and the
// column of the main project name a version this build does not know.
function saveAsNewerVersion(home: string): void {
	const db = new DatabaseSync(join(home, "db.sqlite"));
	try {
		db.exec(
			"UPDATE timelines SET document = json_set(document, '$.schema_version', 999), schema_version = 999 WHERE id = 'prj_1000000001'",
		);
	} finally {
		db.close();
	}
}

interface Doc {
	items: Record<string, { type?: string; keyframes?: Record<string, unknown> }>;
	tracks: Record<string, unknown>;
}

async function projectOf(
	api: APIRequestContext,
	id: string,
): Promise<{ revision: number; document: Doc }> {
	const response = await api.get(`/api/v1/projects/${id}?detail=document`);
	expect(response.ok(), `GET ${id}`).toBe(true);
	return (await response.json()) as { revision: number; document: Doc };
}

// A page logged in the way `stuarty open` does it, with every error of the
// page collected: the upgrade path must not raise one.
async function openPage(
	page: Page,
	running: Running,
	path: string,
): Promise<string[]> {
	await page.addInitScript(() => {
		try {
			localStorage.setItem("vv.shortcutsSeen", "1");
		} catch {
			// Storage can be blocked: the tips show.
		}
	});
	const launch = await running.api.post("/api/v1/system/launch");
	expect(launch.ok()).toBe(true);
	const { url } = (await launch.json()) as { url: string };
	await page.goto(url, { waitUntil: "networkidle" });
	// Listening starts once the launch page has settled: the next navigation
	// aborts what it was still loading and Firefox logs each abort (a font, a
	// dynamic import) as an error that is not the upgrade path's.
	const browserName = page.context().browser()?.browserType().name();
	const errors: string[] = [];
	page.on("pageerror", (e) => errors.push(`pageerror: ${e.message}`));
	page.on("console", (m) => {
		if (m.type() !== "error") return;
		// The core refuses a frame of a revision the document has left (409): the
		// preview asked for it before an edit and the browser logs every 4xx.
		if (m.location().url.includes("/frame?") && m.text().includes("409"))
			return;
		// WebKit reports the style-src-attr violation of every element React
		// mounts with a style prop although the styles apply (see cspViolations).
		if (
			browserName === "webkit" &&
			m.text().includes("Refused to apply a stylesheet")
		)
			return;
		errors.push(`console: ${m.text()}`);
	});
	await page.goto(`${running.baseURL}${path}`);
	return errors;
}

// The canvas of the preview has drawn something: more than one colour.
async function drawnColours(page: Page): Promise<number> {
	const canvas = page.getByRole("main", { name: "Preview" }).locator("canvas");
	await expect(canvas).toBeVisible();
	const shot = await canvas.screenshot();
	return page.evaluate(async (base64) => {
		const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
		const bitmap = await createImageBitmap(
			new Blob([bytes], { type: "image/png" }),
		);
		const c = document.createElement("canvas");
		c.width = bitmap.width;
		c.height = bitmap.height;
		const g = c.getContext("2d");
		if (!g) throw new Error("no 2d context");
		g.drawImage(bitmap, 0, 0);
		const data = g.getImageData(0, 0, c.width, c.height).data;
		const seen = new Set<number>();
		for (let i = 0; i < data.length && seen.size < 8; i += 4) {
			seen.add(
				((data[i] ?? 0) << 16) | ((data[i + 1] ?? 0) << 8) | (data[i + 2] ?? 0),
			);
		}
		return seen.size;
	}, shot.toString("base64"));
}

const test = base;

test.afterAll(() => {
	if (candidate) rmSync(dirname(candidate), { recursive: true, force: true });
});

test.describe("upgrade-open", () => {
	test.skip(
		versions().length === 0,
		"VUKEM_UPGRADE_WORKSPACES is not set or holds no <version> folder: the upgrade job (scripts/upgrade-test.sh) makes the workspaces",
	);

	for (const version of versions()) {
		test(`the workspace saved by ${version} opens intact`, async ({ page }) => {
			test.setTimeout(180_000);
			const source = join(WORKSPACES as string, version);
			const steps = scenario(source);
			const running = await serve(version, source);
			try {
				// What the scenario of this release left in the main project.
				const expected: string[] = [];
				if (steps["edit.captions"]) expected.push(CAPTION, TITLE);
				if (steps["edit.clip"]) expected.push(CLIP);

				const stored = await projectOf(running.api, MAIN);
				expect(Object.keys(stored.document.items).sort()).toEqual(
					expected.sort(),
				);

				// The projects page lists what the scenario made, the templates too.
				const listing = await openPage(page, running, "/projects");
				await expect(
					page.getByRole("link", { name: /Open.*Upgrade main/ }),
				).toBeVisible();
				if (steps["template.instantiate"]) {
					await expect(
						page.getByRole("link", { name: /Open.*From starter/ }),
					).toBeVisible();
					expect((await projectOf(running.api, STARTER)).document).toBeTruthy();
				}
				if (steps["template.save"]) {
					await expect(
						page.getByRole("button", {
							name: /New from template.*Upgrade template/,
						}),
					).toBeVisible();
				}
				expect(listing).toEqual([]);

				// The editor: the timeline holds the items of the manifest.
				const errors = await openPage(page, running, `/editor/${MAIN}?debug=1`);
				await expect(page.locator("[data-item-id]")).toHaveCount(
					expected.length,
				);
				for (const id of expected) {
					await expect(page.locator(`[data-item-id="${id}"]`)).toBeVisible();
				}

				// Frame 0 is drawn.
				expect(await drawnColours(page)).toBeGreaterThan(1);

				// Keyframes saved by a release that had them.
				if (steps["edit.keyframes"] && steps["edit.clip"]) {
					await page.locator(`[data-item-id="${CLIP}"]`).click();
					await expect(page.locator("[data-keyframe-row]")).toHaveCount(2);
				}

				// The history the release wrote is still listed, with its actor.
				await page.keyboard.press("Alt+3");
				const history = page.getByRole("region", { name: "History" });
				await expect(history.locator("[data-actor]").first()).toBeVisible();

				// The export of that release is still a finished job with its file.
				if (steps.export) {
					const jobs = await running.api.get(`/api/v1/projects/${MAIN}/jobs`);
					expect(jobs.ok()).toBe(true);
					const listed = (await jobs.json()) as {
						jobs: { job_id: string; kind: string; state: string }[];
					};
					const finished = listed.jobs.filter(
						(j) => j.kind === "export" && j.state === "succeeded",
					);
					expect(finished.length).toBeGreaterThan(0);
					const file = await running.api.get(
						`/api/v1/jobs/${finished[0]?.job_id}/file`,
						{ headers: { Range: "bytes=0-15" } },
					);
					expect([200, 206]).toContain(file.status());
				}

				// A split followed by an undo returns to the same document.
				const target = steps["edit.clip"] ? CLIP : TITLE;
				const before = await page.evaluate(() => ({
					revision: window.__vv?.revision,
					document: window.__vv?.document,
				}));
				await page.locator(`[data-item-id="${target}"]`).click();
				const playhead = page.getByRole("slider", { name: "Playhead" });
				await playhead.focus();
				await page.keyboard.press("ArrowRight");
				await page.getByRole("button", { name: "Split" }).click();
				await expect
					.poll(() => page.evaluate(() => window.__vv?.revision))
					.toBeGreaterThan(before.revision ?? 0);
				await expect(page.locator("[data-item-id]")).toHaveCount(
					expected.length + 1,
				);
				await page.getByRole("button", { name: "Undo", exact: true }).click();
				await expect(page.locator("[data-item-id]")).toHaveCount(
					expected.length,
				);
				const after = await page.evaluate(() => window.__vv?.document);
				expect(after).toEqual(before.document);

				expect(errors).toEqual([]);
			} finally {
				await running.stop();
			}
		});
	}

	// The newest workspace of the matrix stands in for the one a later release wrote.
	const latest = versions().at(-1);

	test("a workspace written by a newer version is refused at start with the message of the core", async () => {
		test.setTimeout(120_000);
		const home = copyWorkspace(
			"newer-start",
			join(WORKSPACES as string, latest as string),
		);
		saveAsNewerVersion(home);
		const started = await start(home);
		try {
			const code = await started.exited();
			expect(code).toBe(1);
			expect(started.output()).toContain("newer version");
			expect(started.output()).toContain("999");
		} finally {
			await started.kill();
			rmSync(home, { recursive: true, force: true });
		}
	});

	test("a project a newer version saved shows the code, the message and the hint, not a blank page", async ({
		page,
	}) => {
		test.setTimeout(120_000);
		const running = await serve(
			"newer-open",
			join(WORKSPACES as string, latest as string),
		);
		try {
			// The editor page is served, then the document a newer version wrote is
			// what the core is asked for: the real answer of the core, not a script.
			// The bump goes between the page and the fetch of the document: a route
			// on the page navigation (WebKit on Windows does not route the fetch
			// the editor makes, though it reports it).
			let bumped = false;
			await page.route(`**/editor/${MAIN}`, async (route) => {
				const response = await route.fetch();
				if (!bumped) {
					bumped = true;
					saveAsNewerVersion(running.home);
				}
				await route.fulfill({ response });
			});
			const errors = await openPage(page, running, `/editor/${MAIN}`);
			const refusal = await running.api.get(
				`/api/v1/projects/${MAIN}?detail=document`,
			);
			expect(refusal.ok()).toBe(false);
			const body = (await refusal.json()) as {
				code: string;
				message: string;
				hint?: string;
			};
			const alert = page.getByRole("alert");
			await expect(alert).toContainText(body.message);
			await expect(alert).toContainText(body.code);
			if (body.hint) await expect(alert).toContainText(body.hint);
			// Not a blank page: the editor around the banner is still there.
			await expect(page.getByRole("main", { name: "Preview" })).toBeVisible();
			// The refusal of the core is the only error the page raises.
			expect(
				errors.filter((e) => !e.includes(String(refusal.status()))),
			).toEqual([]);
		} finally {
			await running.stop();
		}
	});
});

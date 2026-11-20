import { type ChildProcess, execFileSync, spawn } from "node:child_process";
import { existsSync, mkdirSync, openSync, rmSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, home, pointAtServerHome } from "../specs/helpers";

// The launcher secret of this server (not the one of the shared e2e server), so a spec can sign in as a person.
export const LAUNCHER_SECRET = "first-flow-launcher-secret-not-a-credential";

// The server of the first-flow specs (first-flow and a11y-first-flow-keyboard): the real server with a test
// transcriber, a fake AI provider and a fake social network on loopback (internal/tools/firstflowfake). It builds
// both programs, starts them on a workspace of its own and stops them at the end. Nothing leaves the machine.
export const REPO = fileURLToPath(new URL("../../../", import.meta.url));
const exe = process.platform === "win32" ? ".exe" : "";

export interface Running {
	dir: string;
	server: ChildProcess;
	fake: ChildProcess;
	providerUrl: string;
	socialUrl: string;
}

function ffmpegEnv(): Record<string, string> {
	const dir = process.env.STUARTY_TEST_FFMPEG_DIR;
	if (!dir) return {};
	const bin = existsSync(join(dir, "bin", `ffmpeg${exe}`))
		? join(dir, "bin")
		: dir;
	return {
		STUARTY_FFMPEG: join(bin, `ffmpeg${exe}`),
		STUARTY_FFPROBE: join(bin, `ffprobe${exe}`),
	};
}

function build(out: string, pkg: string): void {
	execFileSync("go", ["build", "-o", out, pkg], {
		cwd: REPO,
		stdio: "inherit",
	});
}

function startFake(
	bin: string,
	mediaRoot: string,
): Promise<{
	process: ChildProcess;
	providerUrl: string;
	socialUrl: string;
}> {
	return new Promise((resolve, reject) => {
		const child = spawn(bin, ["-media-root", mediaRoot], {
			stdio: ["ignore", "pipe", "inherit"],
		});
		let seen = "";
		child.once("error", reject);
		child.once("exit", (code) =>
			reject(new Error(`the fake services stopped with ${code}`)),
		);
		child.stdout?.on("data", (chunk: Buffer) => {
			seen += chunk.toString();
			const ready = /ready provider=(\S+) social=(\S+)/.exec(seen);
			if (ready?.[1] && ready[2]) {
				child.removeAllListeners("exit");
				resolve({ process: child, providerUrl: ready[1], socialUrl: ready[2] });
			}
		});
	});
}

async function waitUp(port: number): Promise<void> {
	await expect
		.poll(
			async () => {
				try {
					return (await fetch(`http://127.0.0.1:${port}/up`)).ok;
				} catch {
					return false; // not listening yet
				}
			},
			{ timeout: 60_000, intervals: [250] },
		)
		.toBe(true);
}

export async function startFirstFlow(
	port: number,
	tag: string,
): Promise<Running> {
	const dir = `${home}-${tag}`;
	rmSync(dir, { recursive: true, force: true });
	mkdirSync(join(dir, "bin"), { recursive: true });
	build(join(dir, "bin", `stuarty${exe}`), "./web/cmd/app");
	build(
		join(dir, "bin", `firstflowfake${exe}`),
		"./internal/tools/firstflowfake",
	);
	const fake = await startFake(join(dir, "bin", `firstflowfake${exe}`), dir);
	const logFd = openSync(join(dir, "server.log"), "a");
	const server = spawn(join(dir, "bin", `stuarty${exe}`), ["serve"], {
		cwd: REPO,
		// The log of the server stays next to its workspace, for whoever debugs a failure.
		stdio: ["ignore", logFd, logFd],
		env: {
			...process.env,
			APP_ENV: "test",
			STUARTY_ADDR: `127.0.0.1:${port}`,
			STUARTY_HOME: join(dir, "workspace"),
			STUARTY_MASTER_KEY_FILE: join(dir, "master.key"),
			STUARTY_TEST_SOCIAL_URL: fake.socialUrl,
			STUARTY_TEST_TRANSCRIBER: "stub",
			STUARTY_LAUNCHER_SECRET: LAUNCHER_SECRET,
			STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE: "1000",
			STUARTY_RATE_LIMIT_PER_MINUTE: "100000",
			STUARTY_RATE_LIMIT_SESSION_PER_MINUTE: "100000",
			...ffmpegEnv(),
		},
	});
	await waitUp(port);
	pointAtServerHome(join(dir, "workspace"));
	return {
		dir,
		server,
		fake: fake.process,
		providerUrl: fake.providerUrl,
		socialUrl: fake.socialUrl,
	};
}

// A date and time as a datetime-local field holds it, in the zone of the machine that runs the browser.
export function localField(date: Date): string {
	const pad = (n: number): string => String(n).padStart(2, "0");
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export interface Upload {
	id: string;
	title: string;
	path: string;
	bytes: number;
}

export async function uploads(socialUrl: string): Promise<Upload[]> {
	const response = await fetch(`${socialUrl}/_uploads`);
	return ((await response.json()) as { uploads: Upload[] }).uploads;
}

// A person decides in the desktop app, whose launch code carries the launcher secret (SDD-007 section 3.4
// item 7); the approval queue refuses the local token an agent holds.
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

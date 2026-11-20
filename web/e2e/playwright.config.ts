// End-to-end tests run against the real server and the real core, never
// against mocks (SDD-012 section 20.1). The server starts on a throwaway
// workspace. Locally, stop the Vite dev server and delete
// resources/public/hot first: a leftover hot file makes the app load its
// scripts from the dev server.
import { existsSync, readdirSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "@playwright/test";
import { CHROMIUM_ONLY, NOT_IN_MATRIX } from "./support/browsers";
import { LAUNCHER_SECRET } from "./support/launcher";
import {
	GOOGLE_PORT,
	NETWORK_ADMIN,
	NETWORK_PORT,
	NETWORK_URL,
	SIGNIN_PORT,
} from "./support/network";

// The server serves resources/public/build, which is not the sources: a build
// older than them runs the specs against an old editor (a stale one once drew
// text at a scale of 1), so the run stops and says so. `bun run build:e2e` in
// web/ makes the build.
function newestSource(dir: string): number {
	let newest = 0;
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const path = join(dir, entry.name);
		newest = Math.max(
			newest,
			entry.isDirectory() ? newestSource(path) : statSync(path).mtimeMs,
		);
	}
	return newest;
}

function assertBuildIsFresh(): void {
	const resources = fileURLToPath(new URL("../resources/", import.meta.url));
	const manifest = join(resources, "public", "build", ".vite", "manifest.json");
	if (!existsSync(manifest)) {
		throw new Error("no editor build: run `bun run build:e2e` in web/ first");
	}
	const built = statSync(manifest).mtimeMs;
	for (const source of ["js", "css"]) {
		if (newestSource(join(resources, source)) > built) {
			throw new Error(
				`the editor build is older than resources/${source}: run \`bun run build:e2e\` in web/ first`,
			);
		}
	}
}

if (process.env.STUARTY_DETECT_ONLY !== "1") assertBuildIsFresh();

const port = Number(process.env.STUARTY_E2E_PORT ?? 3101);
// The pinned FFmpeg of the tests (scripts/install-ffmpeg.sh prints its folder):
// the server finds it through STUARTY_FFMPEG and STUARTY_FFPROBE.
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

// The WebKit run of the detection page needs only the detection server.
const detectOnly = process.env.STUARTY_DETECT_ONLY === "1";

// The detection server (desktop/detect) listens on 7979 when nothing says
// otherwise (CI, the desktop-detect workflow). With STUARTY_E2E_PORT set, a run
// owns a block of ports and the detection server takes one of them (above the
// sign-in server), so a local run never touches 7979.
const detectPort = Number(
	process.env.STUARTY_E2E_DETECT_PORT ??
		(process.env.STUARTY_E2E_PORT ? port + 6 : 7979),
);
const detectUrl = `http://127.0.0.1:${detectPort}`;

// A server starts only when a project that uses it runs. The projects chosen
// with --project (both `--project=x` and `--project x`) tell which; with none
// chosen every server starts, as in the CI runs that pass the list themselves.
function chosenProjects(): string[] {
	const chosen: string[] = [];
	const argv = process.argv;
	for (let i = 0; i < argv.length; i++) {
		const arg = argv[i];
		if (arg === "--project" || arg === "-p") {
			if (argv[i + 1]) chosen.push(argv[i + 1]);
		} else if (arg.startsWith("--project=")) {
			chosen.push(arg.slice("--project=".length));
		}
	}
	return chosen;
}
const chosen = chosenProjects();
const runs = (name: string): boolean =>
	chosen.length === 0 || chosen.includes(name);
const needsFailingEncoder = runs("failing-encoder");
const needsDetect = runs("desktop-detect") || runs("desktop-detect-webkit");
// Every other project (the browsers, perf, soak) uses the app and the network servers.
const needsApp =
	chosen.length === 0 ||
	chosen.some(
		(name) => !name.startsWith("desktop-detect") && name !== "failing-encoder",
	);

const home = process.env.STUARTY_E2E_HOME ?? join(tmpdir(), "stuarty-e2e-home");

// A second server whose encoder cannot work, for scenario 9 (export-failure).
// Its workspace follows the main one, so two runs with their own homes (two
// checkouts, two agents) never open the same workspace.
const failPort = port + 1;
const failHome = `${home}-failing-encoder`;

// A third server in network mode, for the login and keys pages (P2-UI-029). It
// starts with one administrator made from the environment (SDD-007 section 4.1).
const networkHome = `${home}-network`;

export default defineConfig({
	testDir: "./specs",
	fullyParallel: true,
	// Never retry: a test that passes on a second try is a flaky test, which is a defect (REVIEW.md).
	retries: 0,
	reporter: process.env.CI ? [["github"], ["list"]] : "list",
	use: {
		baseURL: `http://127.0.0.1:${port}`,
		trace: "retain-on-failure",
	},
	// Without a vsync, the interval between animation frames measures the work of
	// a frame, not the 16.7 ms of the display (the performance specs read it).
	projects: [
		{
			name: "chromium",
			testIgnore: NOT_IN_MATRIX,
			use: {
				browserName: "chromium",
				launchOptions: {
					args: ["--disable-gpu-vsync", "--disable-frame-rate-limit"],
				},
			},
		},
		// Firefox and WebKit run every spec not tagged CHROMIUM_ONLY (ADR-0023
		// item 5, SDD-012 section 21.1). Their CI jobs are P3-REL-005.
		{
			name: "firefox",
			testIgnore: NOT_IN_MATRIX,
			grepInvert: new RegExp(CHROMIUM_ONLY),
			use: {
				browserName: "firefox",
				// Headless Firefox on Linux has no WebGL ("Exhausted GL driver
				// options"), so the preview falls back to a Canvas2D renderer that
				// skips every filter (masks, blur, color) and the golden specs cannot
				// pass. A headed Firefox on a virtual display (xvfb-run) has WebGL2
				// on Mesa; the CI job sets this variable and wraps the run in it.
				headless: process.env.STUARTY_FIREFOX_HEADED !== "1",
			},
		},
		{
			name: "webkit",
			testIgnore: NOT_IN_MATRIX,
			grepInvert: new RegExp(CHROMIUM_ONLY),
			use: { browserName: "webkit" },
		},
		// The budgets of SDD-012 section 19 are those of the reference machine. They
		// depend on the load and on a real GPU, so they run only when asked for
		// (STUARTY_PERF=1), alone, in one file and so one worker, after every other
		// spec has finished. The shared CI runner does not ask.
		...(process.env.STUARTY_PERF === "1"
			? [
					{
						name: "perf",
						testMatch: /(perf|video-layers)\.spec\.ts/,
						dependencies: ["chromium"],
						fullyParallel: false,
						use: {
							browserName: "chromium" as const,
							launchOptions: {
								args: ["--disable-gpu-vsync", "--disable-frame-rate-limit"],
							},
						},
					},
				]
			: []),
		// The soak of NFR-008 (SDD-012 section 19.1): ten minutes of editing on the
		// 5 minute project, alone, only when asked for (STUARTY_SOAK=1, the
		// nightly job). It needs Chromium: CDP gives the process ids to read.
		...(process.env.STUARTY_SOAK === "1"
			? [
					{
						name: "soak",
						testMatch: /soak-5min\.spec\.ts/,
						fullyParallel: false,
						use: { browserName: "chromium" as const },
					},
				]
			: []),
		{
			name: "failing-encoder",
			testMatch: /export-failure\.spec\.ts/,
			metadata: { home: failHome },
			use: {
				browserName: "chromium",
				baseURL: `http://127.0.0.1:${failPort}`,
			},
		},
		// The feature-detection page of desktop/detect (SDD-012 section 21.2).
		{
			name: "desktop-detect",
			testMatch: /desktop-detect\.spec\.ts/,
			use: { browserName: "chromium", baseURL: detectUrl },
		},
		// The same page in WebKit, for the desktop shell choice (ADR-0023). It runs
		// only with STUARTY_DETECT_ONLY=1 (workflow desktop-detect.yml).
		...(detectOnly
			? [
					{
						name: "desktop-detect-webkit",
						testMatch: /desktop-detect-webkit\.spec\.ts/,
						use: {
							browserName: "webkit" as const,
							baseURL: detectUrl,
						},
					},
				]
			: []),
	],
	webServer: [
		{
			command: "go run ./web/cmd/app serve",
			cwd: "../..",
			url: `http://127.0.0.1:${port}/up`,
			reuseExistingServer: !process.env.CI,
			timeout: 180_000,
			env: {
				// A throwaway key, computed so that no key-like literal is committed.
				APP_KEY: `base64:${Buffer.alloc(32, "e2e").toString("base64")}`,
				HOST: "127.0.0.1",
				PORT: String(port),
				STUARTY_HOME: home,
				// The master key of the vault (SDD-014 section 4) lives next to the workspace, never in it, so
				// the connections spec does not write into the keychain of the machine that runs it.
				STUARTY_MASTER_KEY_FILE: `${home}.master.key`,
				// The test addresses below are honoured only with APP_ENV=test (SDD-007 T-67).
				APP_ENV: "test",
				// A fake social network of the youtube-connect spec, on loopback (SDD-020 section 12).
				STUARTY_TEST_SOCIAL_URL: `http://127.0.0.1:${port + 3}`,
				// A fake authorization server of the provider-sign-in spec, on loopback (SDD-016 section 12.5).
				STUARTY_TEST_SIGNIN_URL: `http://127.0.0.1:${SIGNIN_PORT}`,
				// The desktop shell's secret, so a spec can sign in as a person (the approval queue).
				STUARTY_LAUNCHER_SECRET: LAUNCHER_SECRET,
				// One token drives the whole suite, faster and with more exports a minute
				// than one person would (the limits of SDD-007 4.4 apply in local mode).
				STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE: "1000",
				STUARTY_RATE_LIMIT_PER_MINUTE: "100000",
				STUARTY_RATE_LIMIT_SESSION_PER_MINUTE: "100000",
				...ffmpegEnv(),
			},
		},
		{
			command: "go run ./web/cmd/app serve",
			cwd: "../..",
			url: `http://127.0.0.1:${failPort}/up`,
			reuseExistingServer: !process.env.CI,
			timeout: 180_000,
			env: {
				APP_KEY: `base64:${Buffer.alloc(32, "e2e").toString("base64")}`,
				HOST: "127.0.0.1",
				PORT: String(failPort),
				STUARTY_HOME: failHome,
				STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE: "1000",
				STUARTY_RATE_LIMIT_PER_MINUTE: "100000",
				STUARTY_RATE_LIMIT_SESSION_PER_MINUTE: "100000",
				// Only this encoder is tried and it is never replaced (ADR-0011).
				STUARTY_ENCODER: "no_such_encoder",
				...ffmpegEnv(),
			},
		},
		{
			command: "go run ./web/cmd/app serve",
			cwd: "../..",
			url: `${NETWORK_URL}/up`,
			reuseExistingServer: !process.env.CI,
			timeout: 180_000,
			env: {
				APP_KEY: `base64:${Buffer.alloc(32, "e2e").toString("base64")}`,
				APP_URL: NETWORK_URL,
				STUARTY_MODE: "network",
				STUARTY_ADDR: `127.0.0.1:${NETWORK_PORT}`,
				STUARTY_HOME: networkHome,
				STUARTY_ADMIN_EMAIL: NETWORK_ADMIN.email,
				STUARTY_ADMIN_PASSWORD: NETWORK_ADMIN.password,
				// Only for a container running as root; harmless elsewhere.
				STUARTY_INSECURE_ALLOW_ROOT: "true",
				// The vault key of the connection of the Google sign-in spec, next to the workspace and never in it.
				STUARTY_MASTER_KEY_FILE: `${networkHome}.master.key`,
				// The test address below is honoured only with APP_ENV=test (SDD-007 T-67).
				APP_ENV: "test",
				// The fake Google of the google-sign-in spec, on loopback (SDD-014 section 12.1).
				STUARTY_TEST_GOOGLE_URL: `http://127.0.0.1:${GOOGLE_PORT}`,
				...ffmpegEnv(),
			},
		},
		{
			command: `go run ./desktop/detect -addr 127.0.0.1:${detectPort} -results ${join(tmpdir(), "stuarty-e2e-detect-results")}`,
			cwd: "../..",
			url: `${detectUrl}/`,
			reuseExistingServer: !process.env.CI,
			timeout: 180_000,
		},
	].filter((server) => {
		if (detectOnly) return server.command.includes("desktop/detect");
		if (server.url === `${detectUrl}/`) return needsDetect;
		if (server.url === `http://127.0.0.1:${failPort}/up`)
			return needsFailingEncoder;
		return needsApp;
	}),
});

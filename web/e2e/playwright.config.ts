// End-to-end tests run against the real server and the real core, never
// against mocks (SDD-012 section 20.1). The server starts on a throwaway
// workspace. Locally, stop the Vite dev server and delete
// resources/public/hot first: a leftover hot file makes the app load its
// scripts from the dev server.
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { defineConfig } from "@playwright/test";

const port = Number(process.env.VUKEM_VIDEO_E2E_PORT ?? 3101);
// The pinned FFmpeg of the tests (scripts/install-ffmpeg.sh prints its folder):
// the server finds it through VUKEM_VIDEO_FFMPEG and VUKEM_VIDEO_FFPROBE.
function ffmpegEnv(): Record<string, string> {
	const dir = process.env.VUKEM_VIDEO_TEST_FFMPEG_DIR;
	if (!dir) return {};
	const exe = process.platform === "win32" ? ".exe" : "";
	const bin = existsSync(join(dir, "bin", `ffmpeg${exe}`))
		? join(dir, "bin")
		: dir;
	return {
		VUKEM_VIDEO_FFMPEG: join(bin, `ffmpeg${exe}`),
		VUKEM_VIDEO_FFPROBE: join(bin, `ffprobe${exe}`),
	};
}

// The WebKit run of the detection page needs only the detection server.
const detectOnly = process.env.VUKEM_VIDEO_DETECT_ONLY === "1";

const home =
	process.env.VUKEM_VIDEO_E2E_HOME ?? join(tmpdir(), "vukem-video-e2e-home");

// A second server whose encoder cannot work, for scenario 9 (export-failure).
const failPort = port + 1;
const failHome = join(tmpdir(), "vukem-video-e2e-home-failing-encoder");

export default defineConfig({
	testDir: "./specs",
	fullyParallel: true,
	retries: process.env.CI ? 1 : 0,
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
			testIgnore:
				/(perf|export-failure|desktop-detect|desktop-detect-webkit)\.spec\.ts/,
			use: {
				browserName: "chromium",
				launchOptions: {
					args: ["--disable-gpu-vsync", "--disable-frame-rate-limit"],
				},
			},
		},
		// The budgets of SDD-012 section 19 are those of the reference machine. They
		// depend on the load and on a real GPU, so they run only when asked for
		// (VUKEM_VIDEO_PERF=1), alone, in one file and so one worker, after every other
		// spec has finished. The shared CI runner does not ask.
		...(process.env.VUKEM_VIDEO_PERF === "1"
			? [
					{
						name: "perf",
						testMatch: /perf\.spec\.ts/,
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
			use: { browserName: "chromium", baseURL: "http://127.0.0.1:7979" },
		},
		// The same page in WebKit, for the desktop shell choice (ADR-0023). It runs
		// only with VUKEM_VIDEO_DETECT_ONLY=1 (workflow desktop-detect.yml).
		...(detectOnly
			? [
					{
						name: "desktop-detect-webkit",
						testMatch: /desktop-detect-webkit\.spec\.ts/,
						use: {
							browserName: "webkit" as const,
							baseURL: "http://127.0.0.1:7979",
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
				VUKEM_VIDEO_HOME: home,
				// One token drives the whole suite, faster and with more exports a minute
				// than one person would (the limits of SDD-007 4.4 apply in local mode).
				VUKEM_VIDEO_RATE_LIMIT_HEAVY_PER_MINUTE: "1000",
				VUKEM_VIDEO_RATE_LIMIT_PER_MINUTE: "100000",
				VUKEM_VIDEO_RATE_LIMIT_SESSION_PER_MINUTE: "100000",
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
				VUKEM_VIDEO_HOME: failHome,
				VUKEM_VIDEO_RATE_LIMIT_HEAVY_PER_MINUTE: "1000",
				VUKEM_VIDEO_RATE_LIMIT_PER_MINUTE: "100000",
				VUKEM_VIDEO_RATE_LIMIT_SESSION_PER_MINUTE: "100000",
				// Only this encoder is tried and it is never replaced (ADR-0011).
				VUKEM_VIDEO_ENCODER: "no_such_encoder",
				...ffmpegEnv(),
			},
		},
		{
			command: `go run ./desktop/detect -results ${join(tmpdir(), "vukem-video-e2e-detect-results")}`,
			cwd: "../..",
			url: "http://127.0.0.1:7979/",
			reuseExistingServer: !process.env.CI,
			timeout: 180_000,
		},
	].filter(
		(server) => !detectOnly || server.command.includes("desktop/detect"),
	),
});

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

const home =
	process.env.VUKEM_VIDEO_E2E_HOME ?? join(tmpdir(), "vukem-video-e2e-home");

export default defineConfig({
	testDir: "./specs",
	fullyParallel: true,
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? [["github"], ["list"]] : "list",
	use: {
		baseURL: `http://127.0.0.1:${port}`,
		trace: "retain-on-failure",
	},
	projects: [{ name: "chromium", use: { browserName: "chromium" } }],
	webServer: {
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
			...ffmpegEnv(),
		},
	},
});

import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { type APIRequestContext, test as base } from "@playwright/test";

// The workspace the server of playwright.config.ts runs on.
export const home =
	process.env.VUKEM_VIDEO_E2E_HOME ?? join(tmpdir(), "vukem-video-e2e-home");

export interface ProjectSummary {
	id: string;
	name: string;
	width: number;
	height: number;
}

// The local API is open to whoever reads the token file of the workspace
// (SDD-007 section 3), the same way the CLI and the MCP server do it.
async function token(): Promise<string> {
	return (await readFile(join(home, "token"), "utf8")).trim();
}

// listProjects reads the projects through the HTTP API, not through the page
// under test, so a spec checks what the core stored.
export async function listProjects(
	request: APIRequestContext,
): Promise<ProjectSummary[]> {
	const response = await request.get("/api/v1/projects", {
		headers: { Authorization: `Bearer ${await token()}` },
	});
	if (!response.ok()) throw new Error(`GET /projects: ${response.status()}`);
	const body = (await response.json()) as { projects: ProjectSummary[] };
	return body.projects;
}

// The editor authenticates a browser with a launch code redeemed at
// /auth/launch, which sets an HttpOnly session cookie (SDD-007 section 3.4).
// Every page of a spec starts logged in, the way `vukem-video open` does it.
export const test = base.extend({
	page: async ({ page, request }, use) => {
		const launch = await request.post("/api/v1/system/launch", {
			headers: { Authorization: `Bearer ${await token()}` },
		});
		if (!launch.ok())
			throw new Error(`POST /system/launch: ${launch.status()}`);
		const { url } = (await launch.json()) as { url: string };
		await page.goto(url);
		await use(page);
	},
});

export { expect } from "@playwright/test";

// ffmpegExe is the pinned FFmpeg of the tests (see playwright.config.ts).
function ffmpegExe(): string {
	const dir = process.env.VUKEM_VIDEO_TEST_FFMPEG_DIR;
	if (!dir) throw new Error("VUKEM_VIDEO_TEST_FFMPEG_DIR is not set");
	const exe = process.platform === "win32" ? ".exe" : "";
	const bin = existsSync(join(dir, "bin", `ffmpeg${exe}`))
		? join(dir, "bin")
		: dir;
	return join(bin, `ffmpeg${exe}`);
}

let clipPath: string | undefined;

// testClip makes, once per run, a 3 s 320x568 clip with a moving test pattern
// and a tone, so a spec has real media to import and render.
export function testClip(): string {
	if (clipPath) return clipPath;
	const dir = join(tmpdir(), "vukem-video-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, "clip.mp4");
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				"testsrc2=size=320x568:rate=30:duration=3",
				"-f",
				"lavfi",
				"-i",
				"sine=frequency=440:duration=3",
				"-c:v",
				"libopenh264",
				"-pix_fmt",
				"yuv420p",
				"-c:a",
				"aac",
				"-shortest",
				out,
			],
			{ stdio: "ignore" },
		);
	}
	clipPath = out;
	return out;
}

export interface Seeded {
	id: string;
	assetId: string;
	trackIds: string[];
	itemIds: string[];
}

// caller returns a function that talks to the HTTP API with the token and the
// client label, so the actor of the edits is "local:<label>" (SDD-007 3.7).
export async function caller(
	request: APIRequestContext,
	label = "e2e",
): Promise<
	(
		method: "GET" | "POST",
		path: string,
		data?: unknown,
	) => Promise<Record<string, unknown>>
> {
	const headers = {
		Authorization: `Bearer ${await token()}`,
		"Vukem-Video-Client": label,
	};
	return async (method, path, data) => {
		const response = await request.fetch(`/api/v1${path}`, {
			method,
			data,
			headers,
		});
		if (!response.ok()) {
			throw new Error(
				`${method} ${path}: ${response.status()} ${await response.text()}`,
			);
		}
		return (await response.json()) as Record<string, unknown>;
	};
}

// seedProject creates a project through the API with the test clip imported,
// `tracks` visual tracks and `perTrack` one-second clips on each, end to end.
export async function seedProject(
	request: APIRequestContext,
	options: { name?: string; tracks?: number; perTrack?: number } = {},
): Promise<Seeded> {
	const call = await caller(request);
	const created = await call("POST", "/projects", {
		name: options.name ?? `E2E ${Date.now()}`,
	});
	const id = created.project_id as string;

	const imported = (await call("POST", `/projects/${id}/media`, {
		files: [{ path: testClip() }],
		wait_ms: 20000,
	})) as { revision: number; assets: { asset_id: string }[] };
	const assetId = imported.assets[0]?.asset_id;
	if (!assetId) throw new Error(`no asset after import for ${id}`);

	let revision = imported.revision;
	const edit = async (operations: unknown[]): Promise<string[]> => {
		const out = (await call("POST", `/projects/${id}/edit`, {
			base_revision: revision,
			operations,
		})) as { revision: number; created: string[] };
		revision = out.revision;
		return out.created;
	};

	const trackIds: string[] = [];
	const tracks = options.tracks ?? 1;
	if (tracks > 0) {
		trackIds.push(
			...(await edit(
				Array.from({ length: tracks }, (_, t) => ({
					op: "track.add",
					args: { kind: "visual", name: `Video ${t + 1}` },
				})),
			)),
		);
	}

	const perTrack = options.perTrack ?? 1;
	const itemIds: string[] = [];
	const clips = trackIds.flatMap((trackId) =>
		Array.from({ length: perTrack }, (_, i) => ({
			op: "clip.add",
			args: {
				track_id: trackId,
				asset_id: assetId,
				start: `${i}s`,
				source_in: "0s",
				duration: "1s",
			},
		})),
	);
	for (let i = 0; i < clips.length; i += 50) {
		itemIds.push(...(await edit(clips.slice(i, i + 50))));
	}
	return { id, assetId, trackIds, itemIds };
}

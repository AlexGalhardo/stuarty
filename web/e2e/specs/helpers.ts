import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { type APIRequestContext, test as base, expect } from "@playwright/test";
import { cspViolations } from "../support/browsers";

// The workspace the server of playwright.config.ts runs on.
export let home =
	process.env.STUARTY_E2E_HOME ?? join(tmpdir(), "stuarty-e2e-home");

// A project that runs against another server (the failing encoder of
// export-failure) names its workspace in its metadata; the worker of that
// project only ever runs that project.
export function pointAtServerHome(dir: string): void {
	home = dir;
}

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
// Every page of a spec starts logged in, the way `stuarty open` does it.
// The first-run tips (SDD-012 section 18.5) would sit over every other spec, so
// a page starts with them already seen; onboarding.spec.ts sets `tipsSeen` to
// false.
export const test = base.extend<{ tipsSeen: boolean }>({
	tipsSeen: [true, { option: true }],
	page: async ({ page, request, tipsSeen, browserName }, use, testInfo) => {
		// Every spec doubles as a CSP check (SDD-012 section 2.2, P4-UI-003): the
		// directives of every violation the page reports across navigations are
		// collected on the Node side and asserted empty at the end.
		const violations: string[] = [];
		await page.exposeFunction("__vvCspReport", (directive: string) => {
			violations.push(directive);
		});
		await page.addInitScript(() => {
			document.addEventListener("securitypolicyviolation", (event) => {
				(
					window as unknown as { __vvCspReport(directive: string): void }
				).__vvCspReport(event.violatedDirective);
			});
		});
		if (tipsSeen) {
			await page.addInitScript(() => {
				try {
					localStorage.setItem("vv.shortcutsSeen", "1");
				} catch {
					// Storage can be blocked: the tips show.
				}
			});
		}
		// STUARTY_E2E_EXECUTOR=rust|pixi runs a spec with that preview executor
		// forced (SDD-012 section 11.1); without it the editor chooses by itself.
		const executor = process.env.STUARTY_E2E_EXECUTOR;
		if (executor === "rust" || executor === "pixi") {
			await page.addInitScript((value) => {
				try {
					localStorage.setItem("stuarty.preview.executor", value);
				} catch {
					// Storage can be blocked: the editor chooses by itself.
				}
			}, executor);
		}
		const other = testInfo.project.metadata.home;
		if (typeof other === "string") pointAtServerHome(other);
		const launch = await request.post("/api/v1/system/launch", {
			headers: { Authorization: `Bearer ${await token()}` },
		});
		if (!launch.ok())
			throw new Error(`POST /system/launch: ${launch.status()}`);
		const { url } = (await launch.json()) as { url: string };
		await page.goto(url);
		await use(page);
		// The end-to-end build verifies every applied diff against a fetched
		// document (SDD-012 section 5.4): a diff that did not describe the whole
		// change fails the test that made it.
		const mismatches = await page
			.evaluate(() => window.__vv?.mismatches ?? [])
			.catch(() => []);
		expect(mismatches, "patched document differs from the fetched one").toEqual(
			[],
		);
		expect(
			cspViolations(browserName, violations),
			"securitypolicyviolation events",
		).toEqual([]);
	},
});

export { expect } from "@playwright/test";

// ffmpegExe is the pinned FFmpeg of the tests (see playwright.config.ts).
export function ffmpegExe(): string {
	const dir = process.env.STUARTY_TEST_FFMPEG_DIR;
	// Without the pinned folder the server uses the ffmpeg on PATH, so do we.
	if (!dir) return "ffmpeg";
	const exe = process.platform === "win32" ? ".exe" : "";
	const bin = existsSync(join(dir, "bin", `ffmpeg${exe}`))
		? join(dir, "bin")
		: dir;
	return join(bin, `ffmpeg${exe}`);
}

// testClip makes, once per run, a 10 s 320x568 clip with a moving smooth gradient
// and a tone, so a spec has real media to import and render. Each variant has
// other pixels, so the core does not deduplicate two of them into one asset.
export function testClip(variant = 0): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, `gradient10-${variant}.mp4`);
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				`gradients=size=320x568:rate=30:duration=10:seed=${variant + 1},hue=h=${variant * 40}`,
				"-f",
				"lavfi",
				"-i",
				`sine=frequency=${440 + variant * 60}:duration=10`,
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
	return out;
}

// audioClip makes, once per run, a 4 s tone as an audio-only file.
export function audioClip(variant = 0): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, `tone4-${variant}.wav`);
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				`sine=frequency=${220 + variant * 90}:duration=4`,
				out,
			],
			{ stdio: "ignore" },
		);
	}
	return out;
}

// silenceClip makes, once per run, a 10 s clip whose sound is a tone with true
// silence from 3 s to 6 s, for the silence removal (SDD-002 C.27).
export function silenceClip(): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, "gradient10-silence.mp4");
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				"gradients=size=320x568:rate=30:duration=10:seed=77",
				"-f",
				"lavfi",
				"-i",
				"sine=frequency=500:duration=10,volume=enable='between(t,3,6)':volume=0",
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
	return out;
}

// markerClip makes a 10 s clip of the given size on a smooth gradient with a red
// and a blue solid box at fixed places: the markers of the displacement check
// of preview-vs-export (SDD-012 section 20.3).
export function markerClip(width: number, height: number): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, `markers3-${width}x${height}.mp4`);
	if (!existsSync(out)) {
		const box = Math.round(Math.min(width, height) / 5);
		const red = `drawbox=x=${Math.round(width * 0.38)}:y=${Math.round(height * 0.2)}:w=${box}:h=${box}:color=red:t=fill`;
		const blue = `drawbox=x=${Math.round(width * 0.53)}:y=${Math.round(height * 0.65)}:w=${box}:h=${box}:color=blue:t=fill`;
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				`gradients=size=${width}x${height}:rate=30:duration=10:seed=9,hue=s=0,${red},${blue}`,
				"-f",
				"lavfi",
				"-i",
				"sine=frequency=330:duration=10",
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
	return out;
}

// alphaClip makes a 1 s 160x90 WebM (VP9 yuva420p) whose left half is fully
// transparent and whose right half is opaque red: the source of the alpha path
// of the preview (VV-013).
export function alphaClip(): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, "alpha-half-red2.webm");
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				"color=c=red:s=160x90:r=10:d=1,format=yuva420p,geq=lum='lum(X,Y)':cb='cb(X,Y)':cr='cr(X,Y)':a='if(gte(X,80),255,0)'",
				"-c:v",
				"libvpx-vp9",
				"-pix_fmt",
				"yuva420p",
				"-auto-alt-ref",
				"0",
				"-b:v",
				"100k",
				out,
			],
			{ stdio: "ignore" },
		);
	}
	return out;
}

// solidClip makes a 1 s 160x90 opaque clip of one colour (an ffmpeg colour name).
export function solidClip(color: string): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, `solid-${color}.mp4`);
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				`color=c=${color}:s=160x90:r=10:d=1`,
				"-c:v",
				"libopenh264",
				"-pix_fmt",
				"yuv420p",
				out,
			],
			{ stdio: "ignore" },
		);
	}
	return out;
}

export interface Seeded {
	id: string;
	assetId: string;
	trackIds: string[];
	// The audio tracks asked for with `audioTracks`.
	audioTrackIds: string[];
	itemIds: string[];
	// The assets of the import, in the order of `files`.
	assetIds: string[];
}

// caller returns a function that talks to the HTTP API with the token and the
// client label, so the actor of the edits is "local:<label>" (SDD-007 3.7).
export async function caller(
	request: APIRequestContext,
	label = "e2e",
): Promise<
	(
		method: "GET" | "POST" | "PUT",
		path: string,
		data?: unknown,
	) => Promise<Record<string, unknown>>
> {
	const headers = {
		Authorization: `Bearer ${await token()}`,
		"Stuarty-Client": label,
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
	options: {
		name?: string;
		tracks?: number;
		perTrack?: number;
		clipDuration?: string;
		assets?: number;
		audioTracks?: number;
		// Files to import instead of the gradient clips.
		files?: string[];
	} = {},
): Promise<Seeded> {
	const call = await caller(request);
	const created = await call("POST", "/projects", {
		name: options.name ?? `E2E ${Date.now()}`,
	});
	const id = created.project_id as string;

	const imported = (await call("POST", `/projects/${id}/media`, {
		files: (
			options.files ??
			Array.from({ length: options.assets ?? 1 }, (_, v) => testClip(v))
		).map((path) => ({ path })),
		wait_ms: 20000,
	})) as { revision: number; assets: { asset_id: string }[] };
	const assetIds = imported.assets.map((a) => a.asset_id);
	const assetId = assetIds[0];
	if (!assetId) throw new Error(`no asset after import for ${id}`);

	// The import answers before an ingest that another project shares has put the
	// asset in this document (the three preview-vs-export cases import one file at
	// once), and a clip.add on the missing asset is a 404. Wait for the document
	// to hold every asset, and take the revision from it.
	let revision = imported.revision;
	const deadline = Date.now() + 60_000;
	for (;;) {
		const head = (await call("GET", `/projects/${id}?detail=document`)) as {
			revision: number;
			document?: { assets?: Record<string, unknown> };
		};
		if (assetIds.every((a) => head.document?.assets?.[a] !== undefined)) {
			revision = head.revision;
			break;
		}
		if (Date.now() > deadline) {
			throw new Error(`assets of ${id} never reached the document`);
		}
		await new Promise((resolve) => setTimeout(resolve, 100));
	}
	const edit = async (operations: unknown[]): Promise<string[]> => {
		const out = (await call("POST", `/projects/${id}/edit`, {
			base_revision: revision,
			operations,
		})) as {
			revision: number;
			effective: { created?: string[] }[];
		};
		revision = out.revision;
		// The top-level `created` is sorted by (random) id; the per-operation lists
		// keep the order of the operations, which the callers index into.
		return out.effective.flatMap((effect) => effect.created ?? []);
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

	const audioTrackIds: string[] = [];
	const audioTracks = options.audioTracks ?? 0;
	if (audioTracks > 0) {
		audioTrackIds.push(
			...(await edit(
				Array.from({ length: audioTracks }, (_, t) => ({
					op: "track.add",
					args: { kind: "audio", name: `Audio ${t + 1}` },
				})),
			)),
		);
	}

	const perTrack = options.perTrack ?? 1;
	const itemIds: string[] = [];
	const clips = trackIds.flatMap((trackId, t) =>
		Array.from({ length: perTrack }, (_, i) => ({
			op: "clip.add",
			args: {
				track_id: trackId,
				asset_id: assetIds[t % assetIds.length],
				start: `${i}s`,
				source_in: "0s",
				duration: options.clipDuration ?? "1s",
			},
		})),
	);
	for (let i = 0; i < clips.length; i += 50) {
		itemIds.push(...(await edit(clips.slice(i, i + 50))));
	}
	return { id, assetId, assetIds, trackIds, audioTrackIds, itemIds };
}

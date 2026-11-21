import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
	type APIRequestContext,
	test as base,
	expect,
	type Page,
} from "@playwright/test";
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
// The first-run tour (SDD-012 section 18.5) would sit over every other spec, so
// a page starts with it already seen; first-run-tour.spec.ts sets `tipsSeen`
// to false.
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
					// Storage can be blocked: the tour shows.
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

// A 1 s 320x180 clip made from an ffmpeg video source, once per run, under a name of its own.
function sourceClip(name: string, source: string): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, name);
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				source,
				"-c:v",
				"libopenh264",
				"-b:v",
				"2M",
				"-pix_fmt",
				"yuv420p",
				out,
			],
			{ stdio: "ignore" },
		);
	}
	return out;
}

// greenScreenClip is a green screen (#66DD09, the colour of VV-093) with a red box at its centre: the
// subject of a chroma key.
export function greenScreenClip(): string {
	return sourceClip(
		"green-screen-box-v1.mp4",
		"color=c=0x66DD09:s=320x180:r=30:d=1,drawbox=x=110:y=50:w=100:h=80:color=0xE02020:t=fill",
	);
}

// backdropClip is a plain mid blue clip: what must show behind a keyed subject (neither black nor white).
export function backdropClip(): string {
	return sourceClip(
		"backdrop-blue-v1.mp4",
		"color=c=0x2060C0:s=320x180:r=30:d=1",
	);
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
		method: "GET" | "POST" | "PUT" | "DELETE",
		path: string,
		data?: unknown,
	) => Promise<Record<string, unknown>>
> {
	const headers = {
		Authorization: `Bearer ${await token()}`,
		"Stuarty-Client": label,
	};
	const send = async (
		method: string,
		path: string,
		data?: unknown,
	): Promise<Record<string, unknown>> => {
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
	return async (method, path, data) => {
		// A project needs a description and a goal since epic 56, and no timeline exists until a video is
		// added. A spec that only names the project gets both here: the context, and one timeline with the
		// label as its title (the first timeline has the id of the project).
		const body = data as { name?: string; description?: string } | undefined;
		if (
			method === "POST" &&
			path === "/projects" &&
			body?.name !== undefined &&
			body.description === undefined
		) {
			const created = await send(method, path, {
				...(data as object),
				...projectBody(body.name),
			});
			await send("POST", `/projects/${created.project_id as string}/items`, {
				kind: "timeline",
				title: body.name,
			});
			return created;
		}
		return send(method, path, data);
	};
}

// seedProject creates a project through the API with the test clip imported,
export const GOAL = {
	specific: "Publish one short video that explains the idea",
	measurable: "At least one hundred views in the first week",
	achievable: "The footage and the script already exist",
	relevant: "It brings the first audience to the channel",
	time_bound: "Edit on Monday and publish on Tuesday",
	due: "2999-12-31",
};
export const DESCRIPTION =
	"A short vertical video made for the end to end tests: it has a script, some footage and captions, for the people who follow the channel.";

// A name the core accepts (4 to 32 letters, digits and single spaces) from any label.
export function validName(label: string): string {
	const clean = label
		.replace(/[^\p{L}\p{M}\p{N}]+/gu, " ")
		.trim()
		.slice(0, 32)
		.trim();
	return clean.length >= 4 ? clean : `Project ${clean}`.trim();
}

// The body of a valid POST /projects: the description and the goal are required (epic 56).
export function projectBody(name: string): Record<string, unknown> {
	return { name: validName(name), description: DESCRIPTION, goal: GOAL };
}

export async function openProjectForm(page: Page): Promise<void> {
	// With projects in the list the form waits behind a button (VV-364); the page may still be drawing.
	const open = page.getByRole("button", { name: "Start a new project" });
	const name = page.getByLabel("Name", { exact: true });
	await expect(open.or(name)).toBeVisible();
	if (await open.isVisible()) {
		// The keyboard, so the field tips that open on a keyboard focus still do.
		await open.focus();
		await page.keyboard.press("Enter");
	}
}

// fillProjectForm fills the create form of the projects page with a valid context (epic 56).
export async function fillProjectForm(page: Page, name: string): Promise<void> {
	await openProjectForm(page);
	await page.getByLabel("Name", { exact: true }).fill(validName(name));
	await page.getByLabel("Description", { exact: true }).fill(DESCRIPTION);
	await page.getByLabel("Specific", { exact: true }).fill(GOAL.specific);
	await page.getByLabel("Measurable", { exact: true }).fill(GOAL.measurable);
	await page.getByLabel("Achievable", { exact: true }).fill(GOAL.achievable);
	await page.getByLabel("Relevant", { exact: true }).fill(GOAL.relevant);
	await page.getByLabel("Time-bound", { exact: true }).fill(GOAL.time_bound);
	await page.getByLabel("Due date", { exact: true }).fill(GOAL.due);
}

// createThroughTheForm makes a project with the form, adds its timeline on the page of the project (the size is
// asked there) and opens the editor; it returns the id of the project.
export async function createThroughTheForm(
	page: Page,
	name: string,
): Promise<string> {
	await page.goto("/projects");
	await fillProjectForm(page, name);
	await page.getByRole("button", { name: "Create project" }).click();
	await page.waitForURL(/\/projects\/prj_[0-9a-hjkmnp-tv-z]{10}$/);
	const id = page.url().split("/").pop() ?? "";
	await page.getByLabel("Title", { exact: true }).fill(name);
	await page.getByRole("button", { name: "Add item" }).click();
	await page.goto(`/editor/${id}`);
	return id;
}

// A project through the API, with one timeline named after the label, as the editor specs need; the first
// timeline of a project has the id of the project.
export async function makeProject(
	call: (
		method: "GET" | "POST" | "PUT" | "DELETE",
		path: string,
		data?: unknown,
	) => Promise<Record<string, unknown>>,
	label: string,
): Promise<Record<string, unknown>> {
	const created = await call("POST", "/projects", projectBody(label));
	await call("POST", `/projects/${created.project_id as string}/items`, {
		kind: "timeline",
		title: label,
	});
	return created;
}

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
	const created = await makeProject(call, options.name ?? `E2E ${Date.now()}`);
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

import { execFileSync } from "node:child_process";
import {
	existsSync,
	mkdirSync,
	readdirSync,
	readFileSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import type { APIRequestContext, Page } from "@playwright/test";
import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import {
	type Centroid,
	centroid,
	chromaCentroid,
	globalShift,
	lumaSsim,
	type Pixels,
} from "../support/ssim";
import {
	caller,
	expect,
	ffmpegExe,
	markerClip,
	seedProject,
	test,
} from "./helpers";

// SDD-012 section 20.3 and SDD-004 section 11: for each golden case of
// engine/export/testdata/golden (the document, the description of its media and
// the listed frames), load the case into the workspace as a project, step the
// preview to each listed frame, capture the canvas without the truth-on-pause
// overlay, fetch ViewFrame at the width of the canvas and compare in Node:
// luma SSIM at least 0.97 and no layer displaced by more than 1 pixel.
//
// ViewFrame is requested at the width of the canvas and compared at the size of
// the canvas, so no image is resampled: the gate measures the preview. Compared
// after downscaling both to a thumbnail the SSIM of the same frame falls to 0.96.
const MIN_SSIM = 0.97;
const MAX_DISPLACEMENT = 1;
// The colour cases (lookup tables, grade, animated colour) also compare the mean
// colour per channel, in levels of 255; the float shaders may differ by a rounding step.
const COLOUR_CASE = /^(lut-|grade-|color-keyframes)/;
const MAX_MEAN_COLOUR_DIFF = 4;
// Below this much edge per pixel (see globalShift) the offset of the whole
// picture is noise and the markers give the displacement instead.
const MIN_TEXTURE = 0.05;

const GOLDEN = fileURLToPath(
	new URL("../../../engine/export/testdata/golden/", import.meta.url),
);

interface GoldenItem {
	type: "clip" | "text" | "caption" | "shape";
	track: string;
	start: number;
	duration: number;
	asset?: string;
	source_in?: number;
	fit?: string;
	crop?: Record<string, number>;
	x?: number;
	y?: number;
	scale?: number;
	rotation?: number;
	opacity?: number;
	brightness?: number;
	contrast?: number;
	saturation?: number;
	hue?: number;
	temperature?: number;
	tint?: number;
	hsl?: Record<string, unknown>[];
	wheels?: Record<string, unknown>;
	curves?: Record<string, unknown>;
	lut?: { asset: string; intensity: number };
	filter?: string;
	filter_amount?: number;
	blur?: number;
	speed?: { num: number; den: number };
	keyframes?: Record<string, { t: number; value: number; easing?: string }[]>;
	transition_in?: { kind: string; duration: number };
	transition_out?: { kind: string; duration: number };
	text?: string;
	style?: Record<string, unknown>;
	words?: { text: string; start: number; duration: number }[];
	shape?: string;
	width?: number;
	height?: number;
	fill?: string;
	radius?: number;
	stroke_color?: string;
	stroke_width?: number;
}

interface GoldenDoc {
	settings: {
		width: number;
		height: number;
		background?: string;
		fps?: { num: number; den: number };
	};
	tracks: Record<string, { order: string }>;
	items: Record<string, GoldenItem>;
}

interface Fixture {
	name: string;
	sticker?: boolean;
	delays_cs?: number[];
	// A lookup table asset: "warm" or "teal" (lutFixture in engine/export).
	lut?: string;
	width: number;
	height: number;
	fps: number;
	seconds: number;
}

// The case folders that the Go suite generates: a case added there is compared
// here without a change of this file.
function goldenCases(): string[] {
	return readdirSync(GOLDEN, { withFileTypes: true })
		.filter(
			(e) => e.isDirectory() && existsSync(join(GOLDEN, e.name, "frames.json")),
		)
		.map((e) => e.name)
		.sort();
}

// A smooth grey gradient with a red and a blue box at fixed places. A test
// pattern with one-pixel detail would measure the loss of the proxy codec and of
// the JPEG, not the preview; the boxes are the edges the displacement check
// locks onto.
function markedGradient(spec: Fixture): string {
	const box = Math.round(Math.min(spec.width, spec.height) / 5);
	const at = (fx: number, fy: number): string =>
		`x=${Math.round(spec.width * fx)}:y=${Math.round(spec.height * fy)}:w=${box}:h=${box}`;
	return `gradients=size=${spec.width}x${spec.height}:rate=${spec.fps}:duration=${spec.seconds}:seed=7,hue=s=0,drawbox=${at(0.2, 0.2)}:color=red:t=fill,drawbox=${at(0.55, 0.62)}:color=blue:t=fill`;
}

// The generated media of a case, with the arguments of engine/ingest/mediagen
// (a test pattern and a tone), made once per run.
function fixtureClip(spec: Fixture): string {
	const dir = join(tmpdir(), "vukem-video-e2e-golden-markers");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, spec.name);
	if (spec.lut !== undefined) {
		writeFileSync(out, lutFixture(spec.lut));
		return out;
	}
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-hide_banner",
				"-v",
				"error",
				"-nostdin",
				"-y",
				"-f",
				"lavfi",
				"-i",
				markedGradient(spec),
				"-f",
				"lavfi",
				"-i",
				`sine=f=440:r=44100:d=${spec.seconds}`,
				"-c:v",
				"mpeg4",
				"-dct",
				"int",
				"-q:v",
				"4",
				"-c:a",
				"aac",
				"-f",
				"mp4",
				out,
			],
			{ stdio: "ignore" },
		);
	}
	return out;
}

const frames = (n: number): string => `${n}f`;

// The operations that build the document of a golden case in an empty project:
// the same edits a person or an agent would send, with ids of our own so that a
// later operation can name the item.
function buildOperations(
	doc: GoldenDoc,
	assets: Record<string, string>,
): unknown[] {
	const ops: unknown[] = [];
	const settings: Record<string, unknown> = {
		width: doc.settings.width,
		height: doc.settings.height,
	};
	if (doc.settings.background) settings.background = doc.settings.background;
	ops.push({ op: "settings.set", args: settings });
	if (doc.settings.fps) {
		ops.push({
			op: "settings.set_fps",
			args: { fps: `${doc.settings.fps.num}/${doc.settings.fps.den}` },
		});
	}

	// A smaller order is drawn on top; a new track goes on top, so the bottom one
	// is added first.
	const trackIds = new Map<string, string>();
	const ordered = Object.keys(doc.tracks).sort((a, b) =>
		doc.tracks[b].order.localeCompare(doc.tracks[a].order),
	);
	for (const [i, id] of ordered.entries()) {
		const mine = `trk_k${String(i + 1).padStart(9, "0")}`;
		trackIds.set(id, mine);
		ops.push({
			op: "track.add",
			args: { kind: "visual", name: `Layer ${i + 1}`, id: mine },
		});
	}

	let n = 0;
	for (const item of Object.values(doc.items)) {
		const id = `itm_g${String(++n).padStart(9, "0")}`;
		const track = trackIds.get(item.track);
		if (!track) throw new Error(`item on unknown track ${item.track}`);
		const start = frames(item.start);
		if (item.type === "clip") {
			const asset = item.asset ? assets[item.asset] : undefined;
			if (!asset) throw new Error(`clip of unknown asset ${item.asset}`);
			// A clip is added as long as the source it plays: the speed is set
			// after, and the document keeps the length the case lists.
			const ratio = item.speed ? item.speed.num / item.speed.den : 1;
			const add: Record<string, unknown> = {
				track_id: track,
				asset_id: asset,
				start,
				duration: frames(Math.round(item.duration * ratio)),
				id,
			};
			if (item.source_in) add.source_in = frames(item.source_in);
			if (item.fit) add.fit = item.fit;
			ops.push({ op: "clip.add", args: add });
			if (item.speed) {
				ops.push({
					op: "clip.set_speed",
					args: { item_id: id, speed: item.speed },
				});
			}
			if (item.crop) {
				ops.push({
					op: "clip.set_framing",
					args: { item_id: id, crop: item.crop },
				});
			}
			const color = {
				brightness: item.brightness,
				contrast: item.contrast,
				saturation: item.saturation,
				hue: item.hue,
				temperature: item.temperature,
				tint: item.tint,
				hsl: item.hsl,
				wheels: item.wheels,
				curves: item.curves,
				lut_asset: item.lut ? assets[item.lut.asset] : undefined,
				lut_intensity: item.lut?.intensity,
			};
			if (Object.values(color).some((v) => v !== undefined)) {
				ops.push({
					op: "clip.set_color",
					args: { item_id: id, ...color },
				});
			}
			if (item.filter) {
				ops.push({
					op: "clip.set_filter",
					args: {
						item_id: id,
						filter: item.filter,
						...(item.filter_amount === undefined
							? {}
							: { amount: item.filter_amount }),
					},
				});
			}
			if (item.blur !== undefined) {
				ops.push({
					op: "clip.set_blur",
					args: { item_id: id, radius: item.blur },
				});
			}
		} else if (item.type === "text") {
			ops.push({
				op: "text.add",
				args: {
					track_id: track,
					start,
					duration: frames(item.duration),
					text: item.text,
					style: item.style,
					id,
				},
			});
		} else if (item.type === "shape") {
			ops.push({
				op: "shape.add",
				args: {
					track_id: track,
					start,
					duration: frames(item.duration),
					shape: item.shape,
					width: item.width,
					height: item.height,
					fill: item.fill,
					stroke_color: item.stroke_color,
					stroke_width: item.stroke_width,
					radius: item.shape === "rounded_rect" ? item.radius : undefined,
					x: item.x,
					y: item.y,
					id,
				},
			});
		} else {
			ops.push({
				op: "caption.add",
				args: {
					track_id: track,
					start,
					words: (item.words ?? []).map((w) => ({
						text: w.text,
						start: frames(w.start),
						duration: frames(w.duration),
					})),
					id,
				},
			});
			if (item.style) {
				ops.push({
					op: "caption.set_style",
					args: { item_id: id, style: item.style },
				});
			}
		}
		const transform: Record<string, unknown> = { item_id: id };
		for (const key of ["x", "y", "scale", "rotation", "opacity"] as const) {
			if (item[key] !== undefined && item.type !== "shape") {
				transform[key] = item[key];
			}
		}
		if (Object.keys(transform).length > 1) {
			ops.push({ op: "item.set_transform", args: transform });
		}
		for (const [property, keys] of Object.entries(item.keyframes ?? {})) {
			for (const k of keys) {
				ops.push({
					op: "keyframe.set",
					args: {
						item_id: id,
						property,
						at: frames(item.start + k.t),
						value: k.value,
						...(k.easing ? { easing: k.easing } : {}),
					},
				});
			}
		}
		for (const [edge, t] of [
			["in", item.transition_in],
			["out", item.transition_out],
		] as const) {
			if (t) {
				ops.push({
					op: "item.set_transition",
					args: {
						item_id: id,
						edge,
						kind: t.kind,
						duration: frames(t.duration),
					},
				});
			}
		}
	}
	return ops;
}

// The sticker cases need an alpha PNG and a GIF with a delay per frame, which
// the Go suite writes itself (engine/export golden_alpha_test.go); this spec
// makes only the marked-gradient clips.
function needsImageFixtures(name: string): boolean {
	const fixtures = JSON.parse(
		readFileSync(join(GOLDEN, name, "fixtures.json"), "utf8"),
	) as Record<string, Fixture>;
	return Object.values(fixtures).some((f) => f.sticker || f.delays_cs);
}

// The text of a generated lookup table, the same as lutFixture in
// engine/export/lut_test.go (integer arithmetic printed with six decimals): a
// "warm" 17 point table and a "teal" 9 point one that mixes the channels.
function lutFixture(kind: string): string {
	const n = kind === "warm" ? 17 : 9;
	const lines = [
		"# generated for the golden cases",
		`TITLE ${JSON.stringify(kind)}`,
		`LUT_3D_SIZE ${n}`,
	];
	const div = Math.floor;
	const six = (v: number): string =>
		`${div(v / 1000000)}.${String(v % 1000000).padStart(6, "0")}`;
	for (let b = 0; b < n; b++) {
		for (let g = 0; g < n; g++) {
			for (let r = 0; r < n; r++) {
				const o =
					kind === "warm"
						? [
								div(
									(Math.min(div((r * 11) / 10) + div(n / 10), n - 1) *
										1000000) /
										(n - 1),
								),
								div((g * 1000000) / (n - 1)),
								div((div((b * 85) / 100) * 1000000) / (n - 1)),
							]
						: [
								div(((7 * g + 3 * b) * 100000) / (n - 1)),
								div(((8 * r + 2 * g) * 100000) / (n - 1)),
								div(((5 * r + 5 * b) * 100000) / (n - 1)),
							];
				lines.push(o.map(six).join(" "));
			}
		}
	}
	return `${lines.join("\n")}\n`;
}

// Loads a golden case into a new project and returns its id and listed frames.
async function loadCase(
	request: APIRequestContext,
	name: string,
): Promise<{ id: string; frames: number[] }> {
	const dir = join(GOLDEN, name);
	const doc = JSON.parse(
		readFileSync(join(dir, "project.json"), "utf8"),
	) as GoldenDoc;
	const fixtures = JSON.parse(
		readFileSync(join(dir, "fixtures.json"), "utf8"),
	) as Record<string, Fixture>;
	const listed = JSON.parse(
		readFileSync(join(dir, "frames.json"), "utf8"),
	) as number[];

	const call = await caller(request);
	const created = await call("POST", "/projects", { name: `Golden ${name}` });
	const id = created.project_id as string;
	const fixtureIds = Object.keys(fixtures);
	const files = fixtureIds.map((k) => ({ path: fixtureClip(fixtures[k]) }));
	let assets: Record<string, string> = {};
	let revision = (
		(await call("GET", `/projects/${id}`)) as { revision: number }
	).revision;
	if (files.length > 0) {
		const imported = (await call("POST", `/projects/${id}/media`, {
			files,
			wait_ms: 20000,
		})) as { revision: number; assets: { asset_id: string }[] };
		const ids = imported.assets.map((a) => a.asset_id);
		assets = Object.fromEntries(fixtureIds.map((k, i) => [k, ids[i]]));
		// The import can answer before the asset reaches the document.
		const deadline = Date.now() + 60_000;
		for (;;) {
			const head = (await call("GET", `/projects/${id}?detail=document`)) as {
				revision: number;
				document?: { assets?: Record<string, unknown> };
			};
			if (ids.every((a) => head.document?.assets?.[a] !== undefined)) {
				revision = head.revision;
				break;
			}
			if (Date.now() > deadline) throw new Error(`assets of ${name} late`);
			await new Promise((resolve) => setTimeout(resolve, 100));
		}
	}
	await call("POST", `/projects/${id}/edit`, {
		base_revision: revision,
		operations: buildOperations(doc, assets),
	});

	// The project holds what the case lists: the items and their lengths.
	const head = (await call("GET", `/projects/${id}?detail=document`)) as {
		document: { items: Record<string, { duration: number }> };
	};
	const have = Object.values(head.document.items)
		.map((i) => i.duration)
		.sort((a, b) => a - b);
	const want = Object.values(doc.items)
		.map((i) => i.duration)
		.sort((a, b) => a - b);
	expect(have, `${name}: item durations of the loaded project`).toEqual(want);
	return { id, frames: listed };
}

interface Capture {
	canvas: Pixels;
	frame: Pixels;
	// What each side looked like, attached to a failing frame so that a failure
	// shows which side was wrong.
	canvasPng: Buffer;
	frameJpeg: Buffer;
}

// Waits until the canvas says it shows the frame, then until it stops changing,
// and returns its pixels and those of
// ViewFrame at the same width.
async function capture(
	page: Page,
	id: string,
	frame: number,
): Promise<Capture> {
	const canvas = page.getByRole("main", { name: "Preview" }).locator("canvas");
	let previous: Buffer | null = null;
	let shot: Buffer = Buffer.alloc(0);
	const deadline = Date.now() + 20_000;
	// The executor marks the canvas with the frame it shows once every layer of
	// that frame is on it (a loaded texture, a video that has decoded the frame).
	// A capture before that is a blank or half-drawn canvas that would settle too.
	await expect(
		canvas,
		`the canvas never showed frame ${frame} complete`,
	).toHaveAttribute("data-drawn", String(frame), { timeout: 20_000 });
	for (;;) {
		// The pixels of the buffer itself (463 wide where the canvas is 462.94 CSS
		// pixels), not a screenshot of the element, which the browser resamples to
		// whole pixels (464): ViewFrame is asked for the same width below, so no
		// image is resampled. The exact frame <img> above the canvas is not part
		// of the buffer.
		const png = await canvas.evaluate(
			(el) => (el as HTMLCanvasElement).toDataURL("image/png").split(",")[1],
		);
		shot = Buffer.from(png ?? "", "base64");
		if (previous?.equals(shot)) break;
		if (Date.now() > deadline) {
			throw new Error(`the canvas at frame ${frame} never settled`);
		}
		previous = shot;
		await page.waitForTimeout(250);
	}
	const width = shot.readUInt32BE(16);
	const decoded = await page.evaluate(
		async ({ projectId, shotBase64, at, w }) => {
			// Every worker of the run is the same caller (local:ui) and the core lets a
			// caller render 2 frames at once (SDD-007 section 4.4), so with several
			// specs in parallel a request is refused with E_QUEUE_FULL while the others
			// render. That is the rule working: wait (the Retry-After the answer
			// carries, else a jittered pause) and ask again.
			let frameResponse: Response;
			const giveUp = Date.now() + 90_000;
			for (;;) {
				frameResponse = await fetch(
					`/api/v1/projects/${projectId}/frame?at=${at}f&width=${w}`,
					{ headers: { Accept: "application/json" } },
				);
				if (frameResponse.status !== 429 || Date.now() > giveUp) break;
				const header = Number(frameResponse.headers.get("Retry-After"));
				const pause =
					Number.isFinite(header) && header > 0
						? Math.min(header, 5) * 1000
						: 200 + Math.random() * 600;
				await new Promise((resolve) => setTimeout(resolve, pause));
			}
			if (!frameResponse.ok)
				throw new Error(
					`frame ${frameResponse.status} ${await frameResponse.text()}`,
				);
			const body = (await frameResponse.json()) as { image_base64: string };
			// fetch() of a data: URL is refused by the connect-src of the CSP.
			const load = (base64: string, type: string): Promise<ImageBitmap> => {
				const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
				return createImageBitmap(new Blob([bytes], { type }));
			};
			const a = await load(shotBase64, "image/png");
			const b = await load(body.image_base64, "image/jpeg");
			const pixels = (bitmap: ImageBitmap): string => {
				// No resampling: the frame has the pixels of the canvas.
				// A DOM canvas: this WebKit has no OffscreenCanvas.
				const c = document.createElement("canvas");
				c.width = a.width;
				c.height = a.height;
				const g = c.getContext("2d");
				if (!g) throw new Error("no 2d context");
				g.drawImage(bitmap, 0, 0, a.width, a.height);
				const data = g.getImageData(0, 0, a.width, a.height).data;
				let s = "";
				for (let i = 0; i < data.length; i += 0x8000) {
					s += String.fromCharCode(...data.subarray(i, i + 0x8000));
				}
				return btoa(s);
			};
			return {
				width: a.width,
				height: a.height,
				canvas: pixels(a),
				frame: pixels(b),
				frameBase64: body.image_base64,
			};
		},
		{ projectId: id, shotBase64: shot.toString("base64"), at: frame, w: width },
	);
	const toPixels = (base64: string): Pixels => ({
		width: decoded.width,
		height: decoded.height,
		data: new Uint8Array(Buffer.from(base64, "base64")),
	});
	return {
		canvas: toPixels(decoded.canvas),
		frame: toPixels(decoded.frame),
		canvasPng: shot,
		frameJpeg: Buffer.from(decoded.frameBase64, "base64"),
	};
}

async function openCase(page: Page, id: string): Promise<void> {
	// The preview keeps its video elements out of the document: remember them, so
	// that a frame that misses the gate can say what each one was doing.
	await page.addInitScript(() => {
		const videos: HTMLVideoElement[] = [];
		(window as unknown as { __videos: HTMLVideoElement[] }).__videos = videos;
		const create = document.createElement.bind(document);
		document.createElement = ((
			tag: string,
			options?: ElementCreationOptions,
		) => {
			const el = create(tag, options);
			if (tag.toLowerCase() === "video") videos.push(el as HTMLVideoElement);
			return el;
		}) as typeof document.createElement;
	});
	await page.goto(`/editor/${id}`);
	await expect(
		page.getByRole("main", { name: "Preview" }).locator("canvas"),
	).toBeVisible();
}

// Steps the playhead forward to each listed frame in order.
async function stepThrough(
	page: Page,
	listed: number[],
	visit: (frame: number) => Promise<void>,
): Promise<void> {
	let at = 0;
	for (const frame of [...listed].sort((a, b) => a - b)) {
		while (at < frame) {
			await page
				.getByRole("button", { name: "Step forward one frame" })
				.click();
			at++;
		}
		await visit(frame);
	}
}

// Where a blur leaves no edge to follow, the displacement is that of the
// centroids of the red and the blue marker, the worse of the two.
function markerDistance(
	a: Pixels,
	b: Pixels,
): { dx: number; dy: number; distance: number } {
	let worst = { dx: 0, dy: 0, distance: 0 };
	for (const channel of ["red", "blue"] as const) {
		const p = chromaCentroid(a, channel);
		const q = chromaCentroid(b, channel);
		const d = {
			dx: q.x - p.x,
			dy: q.y - p.y,
			distance: Math.hypot(q.x - p.x, q.y - p.y),
		};
		if (!(d.distance <= worst.distance)) worst = d;
	}
	return worst;
}

// The shape case draws solid fills: the centroid of each of the three shapes
// (one in each third of the frame) is the displacement of a layer centre.
function markerShifts(
	name: string,
	a: Pixels,
	b: Pixels,
): { label: string; distance: number }[] {
	if (name !== "shape") return [];
	// The fill #E11D48.
	const fill = (r: number, g: number, bl: number): boolean =>
		r > 200 && g < 60 && bl > 50 && bl < 110;
	const out: { label: string; distance: number }[] = [];
	for (let band = 0; band < 3; band++) {
		const top = Math.floor((a.height * band) / 3);
		const bottom = Math.floor((a.height * (band + 1)) / 3);
		const p: Centroid = centroid(a, fill, top, bottom);
		const q: Centroid = centroid(b, fill, top, bottom);
		out.push({
			label: `shape ${band + 1} centre (${p.count} / ${q.count} px)`,
			distance:
				p.count === 0 || q.count === 0
					? Number.POSITIVE_INFINITY
					: Math.hypot(p.x - q.x, p.y - q.y),
		});
	}
	return out;
}

test.describe("preview-vs-export measures", () => {
	// A grey picture with a bright box: the measures must see the box move.
	const picture = (boxX: number): Pixels => {
		const width = 96;
		const height = 96;
		const data = new Uint8Array(width * height * 4).fill(90);
		for (let row = 30; row < 60; row++) {
			for (let col = boxX; col < boxX + 30; col++) {
				const i = (row * width + col) * 4;
				data[i] = 250;
				data[i + 1] = 40;
				data[i + 2] = 40;
			}
		}
		for (let i = 3; i < data.length; i += 4) data[i] = 255;
		return { width, height, data };
	};

	test("equal pictures have SSIM 1 and no displacement", () => {
		expect(lumaSsim(picture(20), picture(20))).toBeCloseTo(1, 6);
		expect(globalShift(picture(20), picture(20)).distance).toBeLessThan(0.01);
	});

	test("a box moved by 2 pixels lowers the SSIM and is measured as 2 pixels", () => {
		expect(lumaSsim(picture(20), picture(22))).toBeLessThan(0.99);
		const shift = globalShift(picture(20), picture(22));
		expect(shift.distance).toBeGreaterThan(1.5);
		expect(shift.distance).toBeLessThan(2.5);
	});
});

// Cases that miss the gate today, measured on the reference machine (frame 0 of
// each, canvas 463 x 823 over a 180 x 320 project). Playwright reports a case
// listed here that starts to pass as a failure, so the entry is removed when the
// cause is fixed. None of them is a defect of the preview geometry.
const KNOWN_DIVERGENCES: Record<string, string> = {};

test.describe("preview-vs-export", () => {
	test.use({ viewport: { width: 1920, height: 1300 } });
	test.describe.configure({ timeout: 300_000 });
	test.beforeEach(({ browserName }) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
	});

	// A frame that fails is reported by case and frame.
	for (const name of goldenCases()) {
		test(`golden ${name}: SSIM of at least ${MIN_SSIM} on luma and no layer displaced by more than ${MAX_DISPLACEMENT} px`, async ({
			page,
			request,
		}) => {
			test.skip(
				needsImageFixtures(name),
				"the fixtures are an alpha PNG or a GIF with delays that only the Go suite generates",
			);
			test.fail(name in KNOWN_DIVERGENCES, KNOWN_DIVERGENCES[name]);
			const loaded = await loadCase(request, name);
			await openCase(page, loaded.id);
			await stepThrough(page, loaded.frames, async (frame) => {
				const {
					canvas,
					frame: exported,
					canvasPng,
					frameJpeg,
				} = await capture(page, loaded.id, frame);
				const ssim = lumaSsim(canvas, exported);
				if (ssim < MIN_SSIM) {
					const videos = await page.evaluate(() =>
						(
							(window as unknown as { __videos: HTMLVideoElement[] })
								.__videos ?? []
						).map((v) => {
							// What the element itself shows, apart from the preview.
							const probe = document.createElement("canvas");
							probe.width = 16;
							probe.height = 16;
							const g = probe.getContext("2d");
							g?.drawImage(v, 0, 0, 16, 16);
							const px = g?.getImageData(0, 0, 16, 16).data ?? [];
							let sum = 0;
							for (let i = 0; i < px.length; i += 4) sum += px[i + 1] ?? 0;
							// And what a fresh WebGL texture uploaded from it holds.
							const glCanvas = document.createElement("canvas");
							glCanvas.width = 16;
							glCanvas.height = 16;
							const gl = glCanvas.getContext("webgl2");
							let glGreen = -1;
							if (gl) {
								const tex = gl.createTexture();
								gl.bindTexture(gl.TEXTURE_2D, tex);
								gl.texImage2D(
									gl.TEXTURE_2D,
									0,
									gl.RGBA,
									gl.RGBA,
									gl.UNSIGNED_BYTE,
									v,
								);
								const fb = gl.createFramebuffer();
								gl.bindFramebuffer(gl.FRAMEBUFFER, fb);
								gl.framebufferTexture2D(
									gl.FRAMEBUFFER,
									gl.COLOR_ATTACHMENT0,
									gl.TEXTURE_2D,
									tex,
									0,
								);
								const out = new Uint8Array(4);
								gl.readPixels(
									v.videoWidth >> 1,
									v.videoHeight >> 1,
									1,
									1,
									gl.RGBA,
									gl.UNSIGNED_BYTE,
									out,
								);
								glGreen = out[1] ?? -1;
							}
							const quality = v.getVideoPlaybackQuality();
							return {
								meanGreen: Math.round(sum / 256),
								glGreen,
								totalFrames: quality.totalVideoFrames,
								corrupted: quality.corruptedVideoFrames,
								src: v.currentSrc.split("/media/")[1],
								readyState: v.readyState,
								networkState: v.networkState,
								seeking: v.seeking,
								paused: v.paused,
								currentTime: v.currentTime,
								videoWidth: v.videoWidth,
								error: v.error?.code ?? null,
							};
						}),
					);
					console.log(
						`preview-vs-export ${name} frame ${frame} missed the gate; video elements: ${JSON.stringify(videos)}`,
					);
					for (const [suffix, body, contentType] of [
						["canvas.png", canvasPng, "image/png"],
						["viewframe.jpg", frameJpeg, "image/jpeg"],
					] as const) {
						const path = test.info().outputPath(`${name}-${frame}-${suffix}`);
						writeFileSync(path, body);
						await test.info().attach(`${name}-${frame}-${suffix}`, {
							path,
							contentType,
						});
					}
				}
				const edges = globalShift(exported, canvas);
				const shift =
					edges.texture < MIN_TEXTURE
						? markerDistance(exported, canvas)
						: edges;
				const markers = markerShifts(name, canvas, exported);
				const meanRgb = (p: Pixels): number[] => {
					const sum = [0, 0, 0];
					for (let i = 0; i < p.data.length; i += 4) {
						for (let c = 0; c < 3; c++) sum[c] += p.data[i + c];
					}
					return sum.map((v) => v / (p.data.length / 4));
				};
				const canvasMean = meanRgb(canvas);
				const exportMean = meanRgb(exported);
				const fmt = (m: number[]): string =>
					m.map((v) => v.toFixed(1)).join("/");
				console.log(
					`preview-vs-export ${name} frame ${frame}: mean RGB canvas ${fmt(canvasMean)} export ${fmt(exportMean)}`,
				);
				// SSIM on luma cannot see a missing lookup table or a wrong grade (the
				// luma barely moves), so the colour cases also compare the mean colour.
				if (COLOUR_CASE.test(name)) {
					for (let c = 0; c < 3; c++) {
						expect
							.soft(
								Math.abs(canvasMean[c] - exportMean[c]),
								`${name} frame ${frame}: mean ${"RGB"[c]} difference`,
							)
							.toBeLessThanOrEqual(MAX_MEAN_COLOUR_DIFF);
					}
				}
				console.log(
					`preview-vs-export ${name} frame ${frame}: SSIM(luma) ${ssim.toFixed(4)}, shift ${shift.distance.toFixed(2)} px (${shift.dx.toFixed(2)}, ${shift.dy.toFixed(2)}) texture ${edges.texture.toFixed(3)}${markers.map((m) => `, ${m.label} ${m.distance.toFixed(2)} px`).join("")}`,
				);
				expect
					.soft(ssim, `${name} frame ${frame}: SSIM(luma)`)
					.toBeGreaterThanOrEqual(MIN_SSIM);
				expect
					.soft(shift.distance, `${name} frame ${frame}: picture displacement`)
					.toBeLessThanOrEqual(MAX_DISPLACEMENT);
				for (const m of markers) {
					expect
						.soft(m.distance, `${name} frame ${frame}: ${m.label}`)
						.toBeLessThanOrEqual(MAX_DISPLACEMENT);
				}
			});
		});
	}
});

// The three Prototype cases of SDD-012 section 20.3 on a clip with markers: cover
// fit, contain fit and one caption. Red and blue boxes in the clip give the
// displacement check a centroid of their own: weighted by how red or blue each
// pixel is, so that the antialiased edge and the JPEG of ViewFrame do not move
// it (the centroid of the pixels over a threshold jitters by half a pixel).
const PROTOTYPE: {
	name: string;
	fit: "cover" | "contain";
	caption?: string;
}[] = [
	{ name: "cover", fit: "cover" },
	{ name: "contain", fit: "contain" },
	{ name: "caption", fit: "cover", caption: "Meet the new editor" },
];

test.describe("preview-vs-export prototype cases", () => {
	test.use({ viewport: { width: 1920, height: 1300 } });
	test.describe.configure({ timeout: 300_000 });
	test.beforeEach(({ browserName }) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
	});

	for (const scene of PROTOTYPE) {
		test(`${scene.name}: SSIM of at least ${MIN_SSIM} on luma and markers within ${MAX_DISPLACEMENT} px`, async ({
			page,
			request,
		}) => {
			const seeded = await seedProject(request, {
				tracks: 1,
				perTrack: 1,
				clipDuration: "5s",
				files: [markerClip(640, 360)],
			});
			const call = await caller(request);
			const head = (await call("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			const operations: unknown[] = [
				{
					op: "clip.set_framing",
					args: { item_id: seeded.itemIds[0], fit: scene.fit },
				},
			];
			if (scene.caption) {
				// A caption needs its own track: it would overlap the clip on theirs.
				operations.push(
					{
						op: "track.add",
						args: { kind: "visual", name: "Captions", id: "trk_c4p710n2m3" },
					},
					{
						op: "caption.add",
						args: {
							text: scene.caption,
							start: "0s",
							duration: "4s",
							track_id: "trk_c4p710n2m3",
						},
					},
				);
			}
			await call("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations,
			});

			await openCase(page, seeded.id);
			await stepThrough(page, [30], async (frame) => {
				const { canvas, frame: exported } = await capture(
					page,
					seeded.id,
					frame,
				);
				const ssim = lumaSsim(canvas, exported);
				const red = (r: number, g: number, b: number): boolean =>
					r > 180 && g < 80 && b < 80;
				const blue = (r: number, g: number, b: number): boolean =>
					b > 180 && r < 80 && g < 80;
				const shifts = (
					[
						["red", red],
						["blue", blue],
					] as const
				).map(([label, hit]) => {
					const p = centroid(canvas, hit);
					const q = centroid(exported, hit);
					const wp = chromaCentroid(canvas, label);
					const wq = chromaCentroid(exported, label);
					return {
						label,
						q,
						distance: Math.hypot(p.x - q.x, p.y - q.y),
						weighted: Math.hypot(wp.x - wq.x, wp.y - wq.y),
					};
				});
				console.log(
					`preview-vs-export ${scene.name}: SSIM(luma) ${ssim.toFixed(4)}, ${shifts.map((s) => `${s.label} marker ${s.distance.toFixed(2)} px, weighted ${s.weighted.toFixed(2)} px (${s.q.count} px)`).join(", ")}`,
				);
				expect(ssim).toBeGreaterThanOrEqual(MIN_SSIM);
				for (const s of shifts) {
					expect(
						s.q.count,
						`${s.label} marker visible in ViewFrame`,
					).toBeGreaterThan(20);
					expect(
						s.weighted,
						`${s.label} marker displacement`,
					).toBeLessThanOrEqual(MAX_DISPLACEMENT);
				}
			});
		});
	}
});

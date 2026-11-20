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
import { caller, expect, ffmpegExe, test } from "../specs/helpers";
import { differencePicture, encodePng } from "./png";
import {
	type Centroid,
	centroid,
	chromaCentroid,
	globalShift,
	lumaSsim,
	type Pixels,
} from "./ssim";

// The golden cases of engine/export/testdata/golden as projects of the editor, and
// the comparison of a preview frame with the export's frame of the same plan
// (SDD-012 section 20.3, SDD-004 section 11). Shared by preview-vs-export and
// preview-export-parity.
export const MIN_SSIM = 0.97;
export const MAX_DISPLACEMENT = 1;
// The colour cases (lookup tables, grade, animated colour) also compare the mean
// colour per channel, in levels of 255; the float shaders may differ by a rounding step.
export const COLOUR_CASE = /^(lut-|grade-|color-keyframes)/;
export const MAX_MEAN_COLOUR_DIFF = 4;
// The nested projects of epic 52 (a composition drawn as a group, see
// support/nested.ts): compared by SSIM and mean colour.
export const GROUP_CASE = /^nested-/;
// The cases on a transparent canvas: the export's JPEG cannot hold an alpha, so
// it holds the frame over black (premultiplied colour). The canvas is compared
// the same way, and the mean colour tells an alpha that is wrong (VV-278).
export const TRANSPARENT_CASE = /transparent/;
// Below this much edge per pixel (see globalShift) the offset of the whole
// picture is noise and the markers give the displacement instead.
export const MIN_TEXTURE = 0.05;

export const GOLDEN = fileURLToPath(
	new URL("../../../engine/export/testdata/golden/", import.meta.url),
);

export interface GoldenItem {
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
	keyframes?: Record<
		string,
		{ t: number; value: number; easing?: string; bezier?: number[] }[]
	>;
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
	blend?: string;
	chroma_key?: Record<string, unknown>;
	flip_x?: boolean;
	flip_y?: boolean;
	mask?: Record<string, unknown>;
	effects?: Record<string, unknown>[];
	shadow?: Record<string, unknown>;
	outline?: Record<string, unknown>;
}

export interface GoldenDoc {
	settings: {
		width: number;
		height: number;
		background?: string;
		fps?: { num: number; den: number };
	};
	tracks: Record<string, { order: string }>;
	items: Record<string, GoldenItem>;
}

export interface Fixture {
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
export function goldenCases(): string[] {
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
export function markedGradient(spec: Fixture): string {
	const box = Math.round(Math.min(spec.width, spec.height) / 5);
	const at = (fx: number, fy: number): string =>
		`x=${Math.round(spec.width * fx)}:y=${Math.round(spec.height * fy)}:w=${box}:h=${box}`;
	return `gradients=size=${spec.width}x${spec.height}:rate=${spec.fps}:duration=${spec.seconds}:seed=7,hue=s=0,drawbox=${at(0.2, 0.2)}:color=red:t=fill,drawbox=${at(0.55, 0.62)}:color=blue:t=fill`;
}

// The generated media of a case, with the arguments of engine/ingest/mediagen
// (a test pattern and a tone), made once per run.
// A sticker: a disc with a soft edge in four straight colours, transparent
// around it (the corners are fully transparent). A preview that draws the
// transparent area black, or that stacks the alpha, shows over the clip below.
export function stickerPng(size: number): Buffer {
	const data = new Uint8Array(size * size * 4);
	const centre = (size - 1) / 2;
	const radius = size * 0.42;
	const colours: [number, number, number][] = [
		[225, 29, 72],
		[37, 99, 235],
		[34, 197, 94],
		[250, 204, 21],
	];
	for (let y = 0; y < size; y++) {
		for (let x = 0; x < size; x++) {
			const edge = radius - Math.hypot(x - centre, y - centre);
			const alpha = Math.max(0, Math.min(1, edge / 4 + 0.5));
			const [r, g, b] = colours[
				(y < centre ? 0 : 2) + (x < centre ? 0 : 1)
			] ?? [0, 0, 0];
			const i = (y * size + x) * 4;
			data[i] = r;
			data[i + 1] = g;
			data[i + 2] = b;
			data[i + 3] = Math.round(alpha * 255);
		}
	}
	return encodePng({ width: size, height: size, data });
}

export function fixtureClip(spec: Fixture): string {
	const dir = join(tmpdir(), "stuarty-e2e-golden-markers");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, spec.name);
	if (spec.sticker) {
		writeFileSync(out, stickerPng(spec.width));
		return out;
	}
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

export const frames = (n: number): string => `${n}f`;

// The operations that build the document of a golden case in an empty project:
// the same edits a person or an agent would send, with ids of our own so that a
// later operation can name the item.
export function buildOperations(
	doc: GoldenDoc,
	assets: Record<string, string>,
	full = false,
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
			// The blend, the key, the flips, the mask, the effects, the shadow and the
			// outline of a case are sent only when asked for: the spec of the PixiJS
			// executor was written without them and its measures stay as they were.
			if (full) {
				if (item.blend) {
					ops.push({
						op: "clip.set_blend",
						args: { item_id: id, mode: item.blend },
					});
				}
				if (item.chroma_key) {
					ops.push({
						op: "clip.set_chroma_key",
						args: { item_id: id, ...item.chroma_key },
					});
				}
				if (item.flip_x || item.flip_y) {
					ops.push({
						op: "clip.set_flip",
						args: {
							item_id: id,
							horizontal: item.flip_x === true,
							vertical: item.flip_y === true,
						},
					});
				}
				if (item.mask) {
					ops.push({
						op: "clip.set_mask",
						args: { item_id: id, mask: item.mask },
					});
				}
				if (item.effects) {
					ops.push({
						op: "clip.set_effects",
						args: { item_id: id, effects: item.effects },
					});
				}
				if (item.shadow) {
					ops.push({
						op: "clip.set_shadow",
						args: { item_id: id, shadow: item.shadow },
					});
				}
				if (item.outline) {
					ops.push({
						op: "clip.set_outline",
						args: { item_id: id, outline: item.outline },
					});
				}
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
			// Without the mask (sent only when full) its keyframes have nothing to
			// animate and the core refuses them (E_INVALID_STATE).
			if (!full && property.startsWith("mask")) continue;
			for (const k of keys) {
				ops.push({
					op: "keyframe.set",
					args: {
						item_id: id,
						property,
						at: frames(item.start + k.t),
						value: k.value,
						...(k.easing ? { easing: k.easing } : {}),
						...(k.bezier ? { bezier: k.bezier } : {}),
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

// The animated GIF needs a delay per frame, which only the Go suite writes
// (engine/export golden_alpha_test.go); the alpha PNG of the sticker cases is
// made here (stickerPng).
export function needsImageFixtures(name: string): boolean {
	const fixtures = JSON.parse(
		readFileSync(join(GOLDEN, name, "fixtures.json"), "utf8"),
	) as Record<string, Fixture>;
	return Object.values(fixtures).some((f) => f.delays_cs);
}

// The text of a generated lookup table, the same as lutFixture in
// engine/export/lut_test.go (integer arithmetic printed with six decimals): a
// "warm" 17 point table and a "teal" 9 point one that mixes the channels.
export function lutFixture(kind: string): string {
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
export async function loadCase(
	request: APIRequestContext,
	name: string,
	full = false,
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
		await expect
			.poll(
				async () => {
					const head = (await call(
						"GET",
						`/projects/${id}?detail=document`,
					)) as {
						revision: number;
						document?: { assets?: Record<string, unknown> };
					};
					const there = ids.every(
						(a) => head.document?.assets?.[a] !== undefined,
					);
					if (there) revision = head.revision;
					return there;
				},
				{
					message: `assets of ${name} late`,
					timeout: 60_000,
					intervals: [100],
				},
			)
			.toBe(true);
	}
	await call("POST", `/projects/${id}/edit`, {
		base_revision: revision,
		operations: buildOperations(doc, assets, full),
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

// The canvas over black, as the export's JPEG holds the frame: colour times alpha.
// A straight RGB hides the alpha (a fully transparent pixel keeps its colour).
export function premultiplied(p: Pixels): Pixels {
	const data = new Uint8Array(p.data.length);
	for (let i = 0; i < p.data.length; i += 4) {
		const a = p.data[i + 3] ?? 0;
		for (let c = 0; c < 3; c++) {
			data[i + c] = Math.round(((p.data[i + c] ?? 0) * a) / 255);
		}
		data[i + 3] = 255;
	}
	return { width: p.width, height: p.height, data };
}

export interface Capture {
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
export async function capture(
	page: Page,
	id: string,
	frame: number,
	// A project with compositions has more than one timeline, and the frame
	// route asks which: the project's own (its id).
	ownTimeline = false,
): Promise<Capture> {
	const canvas = page.getByRole("main", { name: "Preview" }).locator("canvas");
	let previous: Buffer | null = null;
	let shot: Buffer = Buffer.alloc(0);
	// The executor marks the canvas with the frame it shows once every layer of
	// that frame is on it (a loaded texture, a video that has decoded the frame).
	// A capture before that is a blank or half-drawn canvas that would settle too.
	await expect(
		canvas,
		`the canvas never showed frame ${frame} complete`,
	).toHaveAttribute("data-drawn", String(frame), { timeout: 20_000 });
	await expect
		.poll(
			async () => {
				// The pixels of the buffer itself (463 wide where the canvas is 462.94
				// CSS pixels), not a screenshot of the element, which the browser
				// resamples to whole pixels (464): ViewFrame is asked for the same width
				// below, so no image is resampled. The exact frame <img> above the
				// canvas is not part of the buffer.
				const png = await canvas.evaluate(
					(el) =>
						(el as HTMLCanvasElement).toDataURL("image/png").split(",")[1],
				);
				shot = Buffer.from(png ?? "", "base64");
				const settled = previous?.equals(shot) === true;
				previous = shot;
				return settled;
			},
			{
				message: `the canvas at frame ${frame} never settled`,
				timeout: 20_000,
				intervals: [250],
			},
		)
		.toBe(true);
	const width = shot.readUInt32BE(16);
	// Every worker of the run is the same caller (local:ui) and the core lets a
	// caller render 2 frames at once (SDD-007 section 4.4), so with several specs in
	// parallel a request is refused with E_QUEUE_FULL while the others render. That
	// is the rule working: ask again until the answer is not a 429.
	let exportedBase64 = "";
	await expect
		.poll(
			async () => {
				const answer = await page.evaluate(
					async ({ projectId, at, w, own }) => {
						const response = await fetch(
							`/api/v1/projects/${projectId}/frame?at=${at}f&width=${w}${own ? `&timeline_id=${projectId}` : ""}`,
							{ headers: { Accept: "application/json" } },
						);
						if (response.status === 429) return { status: 429, image: "" };
						if (!response.ok) {
							throw new Error(
								`frame ${response.status} ${await response.text()}`,
							);
						}
						const body = (await response.json()) as { image_base64: string };
						return { status: 200, image: body.image_base64 };
					},
					{ projectId: id, at: frame, w: width, own: ownTimeline },
				);
				if (answer.status === 200) exportedBase64 = answer.image;
				return answer.status;
			},
			{ timeout: 90_000, intervals: [250, 500, 1000, 2000, 5000] },
		)
		.toBe(200);
	const decoded = await page.evaluate(
		async ({ shotBase64, frameBase64 }) => {
			const body = { image_base64: frameBase64 };
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
		{ shotBase64: shot.toString("base64"), frameBase64: exportedBase64 },
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

export async function openCase(
	page: Page,
	id: string,
	query = "",
): Promise<void> {
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
	await page.goto(`/editor/${id}${query}`);
	await expect(
		page.getByRole("main", { name: "Preview" }).locator("canvas"),
	).toBeVisible();
}

// Steps the playhead forward to each listed frame in order.
export async function stepThrough(
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
export function markerDistance(
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
export function markerShifts(
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

// One listed frame of a golden case: the canvas and ViewFrame at the same width,
// the measures of the gate and, for a frame that misses it, both pictures attached
// to the report. `label` names the executor in the log.
export async function compareFrame(
	page: Page,
	name: string,
	id: string,
	frame: number,
	label: string,
	ownTimeline = false,
): Promise<void> {
	const shot = await capture(page, id, frame, ownTimeline);
	const { frame: exported, canvasPng, frameJpeg } = shot;
	// Over black, like the export: an alpha that is wrong shows as a colour.
	const canvas = premultiplied(shot.canvas);
	const ssim = lumaSsim(canvas, exported);
	if (ssim < MIN_SSIM) {
		const videos = await page.evaluate(() =>
			(
				(window as unknown as { __videos: HTMLVideoElement[] }).__videos ?? []
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
			`${label} ${name} frame ${frame} missed the gate; video elements: ${JSON.stringify(videos)}`,
		);
	}
	const edges = globalShift(exported, canvas);
	const shift =
		edges.texture < MIN_TEXTURE ? markerDistance(exported, canvas) : edges;
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
	const fmt = (m: number[]): string => m.map((v) => v.toFixed(1)).join("/");
	console.log(
		`${label} ${name} frame ${frame}: mean RGB canvas ${fmt(canvasMean)} export ${fmt(exportMean)}`,
	);
	// SSIM on luma cannot see a missing lookup table or a wrong grade (the
	// luma barely moves), so the colour cases also compare the mean colour.
	if (
		COLOUR_CASE.test(name) ||
		GROUP_CASE.test(name) ||
		TRANSPARENT_CASE.test(name)
	) {
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
		`${label} ${name} frame ${frame}: SSIM(luma) ${ssim.toFixed(4)}, shift ${shift.distance.toFixed(2)} px (${shift.dx.toFixed(2)}, ${shift.dy.toFixed(2)}) texture ${edges.texture.toFixed(3)}${markers.map((m) => `, ${m.label} ${m.distance.toFixed(2)} px`).join("")}`,
	);
	expect
		.soft(ssim, `${name} frame ${frame}: SSIM(luma)`)
		.toBeGreaterThanOrEqual(MIN_SSIM);
	// The nested projects are made of the smooth gradient clips: they have too
	// little edge for the global shift (a texture of 0.002 to 0.01, under
	// MIN_TEXTURE) and no red or blue marker, so the chroma centroids of the
	// fallback move with the blend and the opacity and say nothing of where the
	// group is. Their displacement is not measured: the SSIM and the mean colour
	// hold the picture, and the group-picture case (support/nested.ts), a picture
	// with edges in a group, measures the displacement of a group.
	const moved = GROUP_CASE.test(name) ? 0 : shift.distance;
	expect
		.soft(moved, `${name} frame ${frame}: picture displacement`)
		.toBeLessThanOrEqual(MAX_DISPLACEMENT);
	for (const m of markers) {
		expect
			.soft(m.distance, `${name} frame ${frame}: ${m.label}`)
			.toBeLessThanOrEqual(MAX_DISPLACEMENT);
	}
	const missed =
		ssim < MIN_SSIM ||
		moved > MAX_DISPLACEMENT ||
		markers.some((m) => m.distance > MAX_DISPLACEMENT);
	if (missed) {
		// Both sides and where they differ, so a failure shows which one is wrong.
		for (const [suffix, body, contentType] of [
			["canvas.png", canvasPng, "image/png"],
			["viewframe.jpg", frameJpeg, "image/jpeg"],
			[
				"difference.png",
				encodePng(differencePicture(canvas, exported)),
				"image/png",
			],
		] as const) {
			const path = test.info().outputPath(`${name}-${frame}-${suffix}`);
			writeFileSync(path, body);
			await test.info().attach(`${name}-${frame}-${suffix}`, {
				path,
				contentType,
			});
		}
	}
}

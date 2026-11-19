import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import type {
	FromRenderWorker,
	PixelSource,
	RenderInput,
	ToRenderWorker,
} from "./protocol";
import { type CoreExports, instantiateCore } from "./renderCore";
import {
	isOpaque,
	type Pixels,
	premultiply,
	RenderService,
} from "./renderService";

const moduleBytes = readFileSync(
	new URL("../../../../../../engine/renderhost/renderer.wasm", import.meta.url),
);
const digest = readFileSync(
	new URL(
		"../../../../../../engine/renderhost/renderer.wasm.sha256",
		import.meta.url,
	),
	"utf8",
).trim();

async function sha256(bytes: ArrayBuffer): Promise<string> {
	const hash = await crypto.subtle.digest("SHA-256", bytes);
	return [...new Uint8Array(hash)]
		.map((b) => b.toString(16).padStart(2, "0"))
		.join("");
}

class Source implements PixelSource {
	closed = 0;
	constructor(
		readonly width: number,
		readonly height: number,
		readonly rgba: number[],
	) {}
	close(): void {
		this.closed++;
	}
}

function solid(width: number, height: number, c: number[]): Source {
	const rgba: number[] = [];
	for (let i = 0; i < width * height; i++) rgba.push(...c);
	return new Source(width, height, rgba);
}

interface Rig {
	service: RenderService<Source, string>;
	posted: FromRenderWorker<string>[];
	send(message: ToRenderWorker<Source>): Promise<void>;
}

function rig(
	wasm: Uint8Array = moduleBytes,
	instantiate?: (bytes: ArrayBuffer) => Promise<CoreExports>,
): Rig {
	const posted: FromRenderWorker<string>[] = [];
	const service = new RenderService<Source, string>(
		{
			fetchModule: async () =>
				wasm.buffer.slice(
					wasm.byteOffset,
					wasm.byteOffset + wasm.byteLength,
				) as ArrayBuffer,
			sha256,
			read: async (source): Promise<Pixels> => ({
				width: source.width,
				height: source.height,
				rgba: new Uint8Array(source.rgba),
			}),
			toBitmap: async (rgba, width, height) =>
				JSON.stringify({ width, height, pixels: [...rgba] }),
			instantiate,
		},
		{ post: (message) => void posted.push(message) },
	);
	return { service, posted, send: (m) => service.receive(m) };
}

const init: ToRenderWorker<Source> = {
	type: "init",
	module: "/renderer.wasm",
	digest,
	prefer: "cpu",
};

function input(layers: unknown[], width = 4, height = 4): RenderInput {
	return {
		version: 1,
		output: { width, height, scale: 1 },
		plan: { frame: 0, width, height, background: "#000000", layers },
	};
}

const layer = (
	item: string,
	extra: Record<string, unknown> = {},
): Record<string, unknown> => ({
	item,
	kind: "media",
	asset: "a",
	asset_kind: "video",
	cx: 2,
	cy: 2,
	w: 4,
	h: 4,
	rotation: 0,
	scale: 1,
	opacity: 1,
	...extra,
});

function pixels(frame: FromRenderWorker<string> | undefined): number[] {
	if (frame?.type !== "frame") {
		throw new Error(`not a frame: ${JSON.stringify(frame)}`);
	}
	return (JSON.parse(frame.bitmap) as { pixels: number[] }).pixels;
}

describe("the render worker logic", () => {
	it("pins the module by digest and refuses another file", async () => {
		const good = rig();
		await good.send(init);
		expect(good.posted[0]).toMatchObject({
			type: "ready",
			apiVersion: 3,
			path: "cpu",
		});

		const other = rig();
		await other.send({ ...init, digest: "0".repeat(64) });
		expect(other.posted[0]).toMatchObject({
			type: "error",
			reason: "module_digest",
		});
		expect(await sha256(moduleBytes.slice().buffer)).toBe(digest);
	});

	it("draws a still frame from an uploaded texture", async () => {
		const r = rig();
		await r.send(init);
		await r.send({
			type: "texture",
			id: "itm_a",
			frame: solid(2, 2, [200, 10, 30, 255]),
		});
		await r.send({ type: "render", seq: 7, input: input([layer("itm_a")]) });
		const frame = r.posted.at(-1);
		expect(frame).toMatchObject({ type: "frame", seq: 7 });
		const out = pixels(frame);
		expect(out).toHaveLength(4 * 4 * 4);
		expect(out.slice(0, 4)).toEqual([200, 10, 30, 255]);
	});

	it("answers missing_texture by naming the item, and draws after the upload", async () => {
		const r = rig();
		await r.send(init);
		await r.send({ type: "render", seq: 1, input: input([layer("itm_b")]) });
		expect(r.posted.at(-1)).toMatchObject({
			type: "error",
			seq: 1,
			reason: "missing_texture",
			detail: "itm_b",
		});
		await r.send({
			type: "texture",
			id: "itm_b",
			frame: solid(1, 1, [0, 255, 0, 255]),
		});
		await r.send({ type: "render", seq: 2, input: input([layer("itm_b")]) });
		expect(pixels(r.posted.at(-1)).slice(0, 4)).toEqual([0, 255, 0, 255]);
	});

	it("refuses a layer with an unknown field as invalid_plan", async () => {
		const r = rig();
		await r.send(init);
		await r.send({
			type: "render",
			seq: 3,
			input: input([layer("x", { colour: 1 })]),
		});
		expect(r.posted.at(-1)).toMatchObject({
			type: "error",
			seq: 3,
			reason: "invalid_plan",
		});
	});

	it("draws a bitmap layer and forgets a released one", async () => {
		const r = rig();
		await r.send(init);
		await r.send({
			type: "bitmap",
			key: "k1",
			bitmap: solid(2, 2, [255, 255, 255, 255]),
			natural: [2, 2],
			center: [0, 0],
		});
		const bitmapLayer = {
			item: "t",
			kind: "bitmap",
			bitmap: { kind: "text" },
			bitmap_key: "k1",
			cx: 2,
			cy: 2,
			w: 2,
			h: 2,
			rotation: 0,
			opacity: 1,
		};
		await r.send({ type: "render", seq: 1, input: input([bitmapLayer]) });
		const out = pixels(r.posted.at(-1));
		expect(out.slice((1 * 4 + 1) * 4, (1 * 4 + 1) * 4 + 4)).toEqual([
			255, 255, 255, 255,
		]);
		await r.send({ type: "release", ids: ["k1"] });
		await r.send({ type: "render", seq: 2, input: input([bitmapLayer]) });
		expect(r.posted.at(-1)).toMatchObject({
			reason: "missing_texture",
			detail: "k1",
		});
	});

	it("closes every source, on the error path too, and holds none after 500 renders", async () => {
		const r = rig();
		await r.send(init);
		const sources: Source[] = [];
		for (let i = 0; i < 500; i++) {
			const s = solid(2, 2, [i % 256, 0, 0, 255]);
			sources.push(s);
			void r.send({ type: "texture", id: "itm_a", frame: s });
			void r.send({ type: "render", seq: i, input: input([layer("itm_a")]) });
		}
		const bad = solid(1, 1, [0, 0, 0, 255]);
		sources.push(bad);
		// An id of 65 bytes is refused by the module: the source is closed anyway.
		await r.send({ type: "texture", id: "x".repeat(65), frame: bad });
		expect(r.posted.at(-1)).toMatchObject({
			type: "error",
			reason: "invalid_argument",
		});
		expect(r.service.openSources).toBe(0);
		expect(sources.every((s) => s.closed === 1)).toBe(true);
		expect(r.posted.filter((m) => m.type === "frame")).toHaveLength(500);
	});

	it("discards an instance that trapped and draws again on a new one (SDD-013 4.5)", async () => {
		let instances = 0;
		let trapNext = false;
		const r = rig(moduleBytes, async (bytes) => {
			instances++;
			const real = await instantiateCore(bytes);
			return {
				...real,
				memory: real.memory,
				vr_render: (...args: Parameters<CoreExports["vr_render"]>) => {
					if (trapNext) {
						trapNext = false;
						throw new WebAssembly.RuntimeError("unreachable");
					}
					return real.vr_render(...args);
				},
			};
		});
		await r.send(init);
		await r.send({
			type: "texture",
			id: "itm_a",
			frame: solid(1, 1, [9, 8, 7, 255]),
		});
		trapNext = true;
		await r.send({ type: "render", seq: 1, input: input([layer("itm_a")]) });
		expect(r.posted.at(-1)).toMatchObject({
			type: "error",
			seq: 1,
			reason: "render_trap",
		});
		expect(instances).toBe(2);
		// The new instance holds nothing of the old one: the page uploads again.
		await r.send({ type: "render", seq: 2, input: input([layer("itm_a")]) });
		expect(r.posted.at(-1)).toMatchObject({
			reason: "missing_texture",
			detail: "itm_a",
		});
		await r.send({
			type: "texture",
			id: "itm_a",
			frame: solid(1, 1, [9, 8, 7, 255]),
		});
		await r.send({ type: "render", seq: 3, input: input([layer("itm_a")]) });
		expect(pixels(r.posted.at(-1)).slice(0, 4)).toEqual([9, 8, 7, 255]);
	});

	it("an upload before init is an error, and a disposed worker closes late sources", async () => {
		const r = rig();
		const early = solid(1, 1, [0, 0, 0, 255]);
		await r.send({ type: "texture", id: "a", frame: early });
		expect(r.posted.at(-1)).toMatchObject({
			type: "error",
			reason: "invalid_argument",
		});
		expect(early.closed).toBe(1);
		await r.send(init);
		await r.send({ type: "dispose" });
		const late = solid(1, 1, [0, 0, 0, 255]);
		await r.send({ type: "texture", id: "a", frame: late });
		expect(late.closed).toBe(1);
		expect(r.service.openSources).toBe(0);
	});
});

describe("pixel helpers", () => {
	it("premultiplies with rounding and leaves opaque pixels alone", () => {
		const px = new Uint8Array([255, 128, 0, 128, 10, 20, 30, 255]);
		premultiply(px);
		expect([...px]).toEqual([128, 64, 0, 128, 10, 20, 30, 255]);
		expect(isOpaque(px)).toBe(false);
		expect(isOpaque(new Uint8Array([1, 2, 3, 255]))).toBe(true);
	});
});

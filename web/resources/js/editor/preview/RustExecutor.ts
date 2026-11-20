import { fetchBitmap } from "../api/bitmaps";
import type { ApiClient } from "../api/client";
import { fetchLut } from "../api/lut";
import type { PlanLayer, PlanMediaLayer, PlanWindow } from "../api/types";
import { chunksUrl, proxyUrl, thumbnailUrl } from "../api/urls";
import { AssetOwners } from "./assetOwners";

import { AudioGraph } from "./audioGraph";
import { DecodeClient, decoderPreference } from "./decode/client";
import { createDecodeWorker } from "./decode/createWorker";
import { Lru } from "./lru";
import { MediaPool, type PoolTimers } from "./mediaPool";
import {
	audioClockReady,
	audioDrive,
	MAX_VIDEO_LAYERS,
	needsCorrection,
	PlaybackClock,
	rankVideoLayers,
	videoDrive,
} from "./mediaSync";
import {
	bitmapKeyAt,
	cropAt,
	geometryOf,
	layersAt,
	valueAt,
} from "./planValue";
import { PlanWindows } from "./planWindows";
import { RenderClient } from "./render/client";
import { createRenderWorker } from "./render/createRenderWorker";
import { frameInput } from "./render/frameInput";
import { drawable, expandGroups, GroupIds, leavesOf } from "./render/groups";
import { RENDERER_DIGEST, RENDERER_MODULE_URL } from "./render/module";
import type { PixelSource } from "./render/protocol";
import { STILL_POSTER_WIDTH, stillFrameRect } from "./stillFrame";
import type {
	LayerGeometry,
	PreviewExecutor,
	PreviewState,
	ProjectInfo,
	Unsubscribe,
} from "./types";

const BITMAP_CACHE = 64;
// Renders kept in the table of what each one was asked for; older ones are
// answered already or dropped.
const PRESENTED_KEPT = 16;
// A failed render resets what was uploaded and draws again; more than this many
// in a row is a bug and stops, so a broken frame cannot loop.
const MAX_RESYNCS = 3;

interface DrawMode {
	playing: boolean;
	scrubbing: boolean;
}

interface Presented {
	frame: number;
	complete: boolean;
	decodedUs: number;
}

const browserTimers: PoolTimers = {
	setTimeout: (fn, ms) => setTimeout(fn, ms),
	clearTimeout: (handle) => clearTimeout(handle as number),
};

// The preview drawn by the renderer of the export (ADR-0029): the plan window
// is expanded to the render input of the frame (SDD-013 section 2.4), the pixels
// each layer needs are uploaded to the worker, and the worker's core module
// draws the frame, so the preview and the export are drawn by the same code.
// What this class decides is when, not what: no pixel is worked out here.
export class RustExecutor implements PreviewExecutor {
	private info: ProjectInfo | null = null;
	private buffer: { width: number; height: number } | null = null;
	private revision = 0;
	private frame = 0;
	private state: PreviewState = "idle";
	private playing = false;
	private raf = 0;
	private canvas: HTMLCanvasElement | null = null;
	private context2d: CanvasRenderingContext2D | null = null;
	private render: RenderClient<PixelSource, ImageBitmap> | null = null;
	// False while a layer of the frame being drawn lacks pixels.
	private complete = true;
	// The frame, revision and size of the last render whose layers were all in
	// it. Playback draws once per animation frame, several times per frame of a
	// 30 fps project; a frame already drawn whole is not drawn again.
	private drawnWhole: string | null = null;
	private generation = 0;
	private disposed = false;
	private audioContext: AudioContext | null = null;
	private audioClock = true;
	private readonly audioGraph = new AudioGraph();
	private clock: PlaybackClock | null = null;
	private windows: PlanWindows;
	private readonly videos: MediaPool;
	// Where the assets of compositions live (see AssetOwners).
	private readonly owners = new AssetOwners();
	private readonly sounds: MediaPool;
	private readonly decoder: DecodeClient<VideoFrame>;
	private decodedUs = -1;
	// What the worker holds for each item: the tag of the pixels uploaded.
	private readonly uploaded = new Map<string, string>();
	// The short ids of what a group puts in the worker (render/groups.ts), the
	// surfaces the worker holds, and the output scale of the frame of the layer
	// being prepared (a group draws its layers at its own scale).
	private readonly groupIds = new GroupIds();
	private surfaceIds = new Set<string>();
	private leafScale = 1;
	// Uploads being made (a bitmap decoded off the main thread, a still cut).
	private readonly making = new Set<string>();
	private readonly bitmaps = new Lru<string, true>(BITMAP_CACHE, (key) =>
		this.render?.release([key]),
	);
	private readonly luts = new Map<string, "loading" | "ready" | "refused">();
	private readonly stillImages = new Map<string, HTMLImageElement>();
	private readonly presented = new Map<number, Presented>();
	private resyncs = 0;
	// Frames the worker drew that reached the canvas, for the playback specs.
	private shown = 0;
	private readonly frameListeners = new Set<(frame: number) => void>();
	private readonly stateListeners = new Set<(state: PreviewState) => void>();

	constructor(
		private readonly client: ApiClient,
		timers?: PoolTimers,
		private readonly sources: (
			asset: string,
		) => { width: number; height: number } | null = () => null,
	) {
		const poolTimers = timers ?? browserTimers;
		this.windows = new PlanWindows(() =>
			Promise.reject(new Error("no project")),
		);
		const redraw = (): void => this.redraw();
		this.decoder = new DecodeClient<VideoFrame>({
			preference: decoderPreference(window.location.search),
			createWorker: createDecodeWorker,
			indexUrl: (asset) =>
				chunksUrl(this.owners.of(asset, this.info?.projectId ?? ""), asset),
			proxyUrl: (asset) =>
				proxyUrl(this.owners.of(asset, this.info?.projectId ?? ""), asset),
			onChange: redraw,
			now: () => performance.now(),
		});
		this.videos = new MediaPool(
			"video",
			() => this.audioContext,
			() => this.audioGraph.input,
			poolTimers,
			redraw,
		);
		this.sounds = new MediaPool(
			"audio",
			() => this.audioContext,
			() => this.audioGraph.input,
			poolTimers,
			redraw,
		);
	}

	get stats(): PreviewExecutor["stats"] {
		const decode = this.decoder.stats;
		return {
			videoElements: this.videos.size,
			planWindows: this.windows.size,
			droppedFrames: this.videos.droppedFrames(),
			...decode,
			openFrames: decode.openFrames + (this.render?.stats.workerOpen ?? 0),
		};
	}

	// Rejects when the worker or the module cannot start, so the caller can use
	// the other executor.
	async mount(host: HTMLElement): Promise<void> {
		const canvas = document.createElement("canvas");
		canvas.className = "block size-full";
		canvas.width = this.buffer?.width ?? this.info?.width ?? 1080;
		canvas.height = this.buffer?.height ?? this.info?.height ?? 1920;
		const worker = createRenderWorker();
		const context = canvas.getContext("2d");
		if (worker === null || context === null) {
			throw new Error("the render worker cannot start");
		}
		const render = new RenderClient<PixelSource, ImageBitmap>(
			worker,
			{
				onFrame: (bitmap, seq) => this.present(bitmap, seq),
				onError: (failure) => this.failed(failure.reason, failure.detail),
			},
			(bitmap) => bitmap.close(),
		);
		try {
			await render.init(RENDERER_MODULE_URL, RENDERER_DIGEST, "cpu");
		} catch (err) {
			render.dispose();
			throw err instanceof Error ? err : new Error(String(err));
		}
		if (this.disposed) {
			render.dispose();
			return;
		}
		// The size asked for while the worker started (the first resize arrives
		// before it has) is applied now.
		if (this.buffer !== null) {
			canvas.width = this.buffer.width;
			canvas.height = this.buffer.height;
		}
		canvas.setAttribute("data-executor", "rust");
		host.append(canvas);
		this.canvas = canvas;
		this.context2d = context;
		this.render = render;
		this.openAudio();
		this.redraw();
	}

	private openAudio(): void {
		if (typeof AudioContext !== "function") return;
		try {
			this.audioContext = new AudioContext();
			this.audioGraph.attach(this.audioContext);
		} catch {
			this.audioContext = null;
		}
	}

	dispose(): void {
		this.disposed = true;
		this.generation++;
		cancelAnimationFrame(this.raf);
		this.videos.dispose();
		this.sounds.dispose();
		this.decoder.dispose();
		this.render?.dispose();
		this.render = null;
		this.bitmaps.clear();
		this.uploaded.clear();
		this.canvas?.remove();
		this.canvas = null;
		this.context2d = null;
		this.audioGraph.detach();
		void this.audioContext?.close();
		this.audioContext = null;
		this.frameListeners.clear();
		this.stateListeners.clear();
	}

	setVolume(volume: number, muted: boolean): void {
		this.audioGraph.setLevel(volume, muted);
	}

	setProject(info: ProjectInfo): void {
		this.info = info;
		this.drawnWhole = null;
		this.clock = new PlaybackClock(
			() =>
				this.audioClock && this.audioContext !== null
					? this.audioContext.currentTime
					: performance.now() / 1000,
			info.fps,
		);
		this.windows = new PlanWindows((from, to, rev, width, signal) =>
			this.client.getPlan(info.projectId, from, to, rev, width, signal),
		);
		this.windows.setRevision(this.revision);
		this.windows.setWidth(this.planWidth());
		this.redraw();
	}

	setRevision(revision: number): void {
		if (revision === this.revision) return;
		this.revision = revision;
		this.windows.setRevision(revision);
		this.redraw();
	}

	refresh(): void {
		this.windows.invalidate();
		this.redraw();
	}

	resize(bufferWidth: number, bufferHeight: number): void {
		this.buffer = { width: bufferWidth, height: bufferHeight };
		this.windows.setWidth(this.planWidth());
		const canvas = this.canvas;
		if (canvas !== null) {
			if (canvas.width !== bufferWidth) canvas.width = bufferWidth;
			if (canvas.height !== bufferHeight) canvas.height = bufferHeight;
			// Resizing clears the canvas: nothing is drawn until the next frame.
			canvas.removeAttribute("data-drawn");
		}
		this.redraw();
	}

	// The plan takes a width of 16 to 4096 (SDD-004 section 10).
	private planWidth(): number | undefined {
		if (this.buffer === null) return undefined;
		return Math.min(4096, Math.max(16, Math.round(this.buffer.width)));
	}

	seek(frame: number, options: { scrubbing?: boolean } = {}): void {
		this.frame = Math.max(frame, 0);
		if (this.playing) {
			this.clock?.start(this.frame);
			return;
		}
		this.setState("seeking");
		const generation = ++this.generation;
		this.windows
			.ensure(this.frame)
			.then((plan) => {
				if (generation !== this.generation || this.disposed) return;
				this.draw(plan, this.frame, {
					playing: false,
					scrubbing: options.scrubbing === true,
				});
				this.setState("idle");
			})
			.catch(() => {
				if (generation === this.generation) this.setState("idle");
			});
	}

	async play(): Promise<void> {
		if (this.playing || this.state === "unsupported" || this.clock === null) {
			return;
		}
		this.audioClock = await audioClockReady(this.audioContext);
		if (this.disposed) return;
		this.canvas?.setAttribute("data-clock", this.audioClock ? "audio" : "wall");
		this.playing = true;
		this.generation++;
		this.clock.start(this.frame);
		this.setState("playing");
		this.tick();
	}

	pause(): void {
		if (!this.playing) return;
		this.playing = false;
		cancelAnimationFrame(this.raf);
		this.videos.pauseAll();
		this.sounds.pauseAll();
		this.setState("idle");
		this.emitFrame();
		// The paused frame is drawn as a paused one, which marks the canvas with
		// it: the last render of playback may have come back before the pause.
		this.redraw();
	}

	onFrame(cb: (frame: number) => void): Unsubscribe {
		this.frameListeners.add(cb);
		return () => this.frameListeners.delete(cb);
	}

	onState(cb: (state: PreviewState) => void): Unsubscribe {
		this.stateListeners.add(cb);
		return () => this.stateListeners.delete(cb);
	}

	getLayerGeometry(frame: number): LayerGeometry[] {
		const plan = this.windows.peek(frame);
		if (plan === undefined) return [];
		return layersAt(plan, frame).map((layer) => geometryOf(layer, frame));
	}

	getSafeArea(frame: number): [number, number, number, number] | null {
		return this.windows.peek(frame)?.safe_area ?? null;
	}

	// A read of the pixels the canvas shows, for the eyedropper.
	readColour(x: number, y: number): string | null {
		const context = this.context2d;
		const info = this.info;
		const canvas = this.canvas;
		if (context === null || info === null || canvas === null) return null;
		if (canvas.getAttribute("data-drawn") === null) return null;
		const ratio = canvas.width / info.width;
		const px = Math.min(canvas.width - 1, Math.max(0, Math.floor(x * ratio)));
		const py = Math.min(canvas.height - 1, Math.max(0, Math.floor(y * ratio)));
		const [r, g, b] = context.getImageData(px, py, 1, 1).data;
		const hex = (n: number | undefined): string =>
			(n ?? 0).toString(16).padStart(2, "0");
		return `#${hex(r)}${hex(g)}${hex(b)}`.toUpperCase();
	}

	whenPlan(frame: number): Promise<void> {
		return this.windows.ensure(frame).then(
			() => undefined,
			() => undefined,
		);
	}

	private setState(state: PreviewState): void {
		if (state === this.state) return;
		this.state = state;
		for (const cb of this.stateListeners) cb(state);
	}

	private emitFrame(): void {
		for (const cb of this.frameListeners) cb(this.frame);
	}

	private readonly tick = (): void => {
		if (!this.playing || this.clock === null) return;
		this.frame = this.clock.frame();
		const plan = this.windows.peek(this.frame);
		if (plan === undefined) {
			this.setState("buffering");
			this.windows.ensure(this.frame).catch(() => undefined);
		} else {
			if (this.state === "buffering") this.setState("playing");
			if (plan.end_frame > 0 && this.frame >= plan.end_frame) {
				this.frame = plan.end_frame;
				this.pause();
				return;
			}
			if (this.drawKey(this.frame) !== this.drawnWhole) {
				this.draw(plan, this.frame, { playing: true, scrubbing: false });
			}
			this.windows.prefetch(this.frame);
		}
		this.emitFrame();
		this.raf = requestAnimationFrame(this.tick);
	};

	private drawKey(frame: number): string {
		return `${frame}:${this.revision}:${this.buffer?.width}x${this.buffer?.height}`;
	}

	private redraw(): void {
		if (this.render === null || this.playing || this.info === null) return;
		const plan = this.windows.peek(this.frame);
		if (plan === undefined) this.seek(this.frame);
		else this.draw(plan, this.frame, { playing: false, scrubbing: false });
	}

	private draw(plan: PlanWindow, frame: number, mode: DrawMode): void {
		this.owners.note(plan);
		const render = this.render;
		const info = this.info;
		const buffer = this.buffer;
		if (render === null || info === null || buffer === null) return;

		// A composition drawn as a group is expanded into the layers of its own
		// frame and its surface (SDD-013 section 3.12): the pixels each layer needs
		// are uploaded as for any other, at the scale of the frame that holds it,
		// and the worker draws the surfaces before the frame.
		const scale = buffer.width / info.width;
		const nodes = expandGroups(plan.layers, frame, scale, this.groupIds);
		const decoding = rankVideoLayers(leavesOf(nodes), MAX_VIDEO_LAYERS);
		const usedVideos = new Set<string>();
		const used = new Set<string>();
		this.complete = true;

		const { layers: ready, surfaces } = drawable(
			nodes,
			frame,
			(layer, layerScale) => {
				this.leafScale = layerScale;
				const drawn = this.prepare(
					layer,
					frame,
					decoding,
					usedVideos,
					mode,
					info,
				);
				if (drawn === null) {
					this.complete = false;
					return null;
				}
				used.add(layer.item);
				return drawn;
			},
		);

		// What left the frame is forgotten by the module; it is uploaded again
		// if the layer comes back. A surface is drawn again for every frame, so
		// only the ones of groups that left are released.
		const kept = new Set(surfaces.map((surface) => surface.id));
		const gone = [
			...[...this.uploaded.keys()].filter((id) => !used.has(id)),
			...[...this.surfaceIds].filter((id) => !kept.has(id)),
		];
		for (const id of gone) this.uploaded.delete(id);
		this.surfaceIds = kept;
		render.release(gone);

		this.videos.idle(usedVideos);
		this.syncAudio(plan, frame, mode.playing);

		const input = frameInput(plan, frame, ready, {
			width: buffer.width,
			height: buffer.height,
			scale,
		});
		const seq = render.render(input, surfaces);
		this.drawnWhole = this.complete ? this.drawKey(frame) : null;
		this.presented.set(seq, {
			frame,
			complete: this.complete,
			decodedUs: this.decodedUs,
		});
		for (const old of this.presented.keys()) {
			if (old <= seq - PRESENTED_KEPT) this.presented.delete(old);
		}
		// A frame leaves the decode cache only after the render that may read it;
		// the worker holds its own copy of what was uploaded.
		this.decoder.sweep();
		this.publishDecode();
	}

	// The frame is on the canvas. It says which frame it shows once every layer
	// of that frame was in it, so a test waits on that instead of a timer.
	private present(bitmap: ImageBitmap, seq: number): void {
		const canvas = this.canvas;
		const context = this.context2d;
		const result = this.presented.get(seq);
		this.presented.delete(seq);
		this.resyncs = 0;
		if (canvas === null || context === null) {
			bitmap.close();
			return;
		}
		if (bitmap.width === canvas.width && bitmap.height === canvas.height) {
			context.clearRect(0, 0, canvas.width, canvas.height);
			context.drawImage(bitmap, 0, 0);
		}
		bitmap.close();
		this.shown++;
		// While playing, frames come too fast to be worth an attribute write each.
		if (!this.playing || this.shown % 10 === 0) {
			canvas.setAttribute("data-presented", String(this.shown));
		}
		if (result === undefined || this.playing) return;
		if (result.decodedUs >= 0 && result.complete) {
			canvas.setAttribute("data-frame-us", String(result.decodedUs));
		}
		if (!result.complete) {
			canvas.removeAttribute("data-drawn");
			return;
		}
		const value = String(result.frame);
		if (canvas.getAttribute("data-drawn") !== value) {
			canvas.setAttribute("data-drawn", value);
		}
	}

	// A render the module refused. A texture it does not hold (a restarted
	// module, a release that raced a draw) is uploaded again, not drawn black
	// (SDD-013 section 4.7); a reason that repeats stops.
	private failed(reason: string, detail: string): void {
		console.debug("preview render failed", reason, detail);
		this.drawnWhole = null;
		this.uploaded.clear();
		this.bitmaps.clear();
		this.luts.clear();
		this.canvas?.removeAttribute("data-drawn");
		if (this.resyncs++ >= MAX_RESYNCS) return;
		this.redraw();
	}

	// What the page shows about the decode path, for the end-to-end specs.
	private publishDecode(): void {
		const canvas = this.canvas;
		if (canvas === null) return;
		const { openFrames, peakOpenFrames, fallbackAssets } = this.stats;
		const values: Record<string, string> = {
			"data-open-frames": String(openFrames),
			"data-peak-frames": String(peakOpenFrames),
			"data-fallback-assets": String(fallbackAssets),
		};
		for (const [name, value] of Object.entries(values)) {
			if (canvas.getAttribute(name) !== value) canvas.setAttribute(name, value);
		}
	}

	// Makes sure the worker holds the pixels of a layer. Returns the layer to
	// draw (without a lookup table that is not there yet), or null when the
	// worker has nothing of it to draw.
	private prepare(
		layer: PlanLayer,
		frame: number,
		decoding: ReadonlySet<string>,
		usedVideos: Set<string>,
		mode: DrawMode,
		info: ProjectInfo,
	): PlanLayer | null {
		if (layer.kind === "bitmap") {
			const key = bitmapKeyAt(layer, frame);
			if (key === undefined) return null;
			if (this.bitmaps.get(key) !== undefined) return layer;
			this.makeBitmap(info.projectId, key, layer.natural);
			return null;
		}
		const has = this.pixelsOf(layer, frame, decoding, usedVideos, mode, info);
		if (!has) return null;
		if (layer.lut === undefined) return layer;
		const state = this.lutState(info.projectId, layer.lut.asset);
		if (state === "ready") return layer;
		if (state === "loading") this.complete = false;
		return { ...layer, lut: undefined };
	}

	private pixelsOf(
		layer: PlanMediaLayer,
		frame: number,
		decoding: ReadonlySet<string>,
		usedVideos: Set<string>,
		mode: DrawMode,
		info: ProjectInfo,
	): boolean {
		if (layer.asset_kind === "image" || !decoding.has(layer.item)) {
			return this.still(
				this.owners.of(layer.asset, info.projectId),
				layer,
				frame,
			);
		}
		const decoded = this.decodedTexture(layer, frame, mode, info);
		if (decoded !== "fallback") return decoded;
		usedVideos.add(layer.asset);
		const { element } = this.videos.acquire(
			layer.asset,
			proxyUrl(this.owners.of(layer.asset, info.projectId), layer.asset),
		);
		const video = element as HTMLVideoElement;
		this.driveVideo(video, layer, frame, mode, info);
		if (
			video.seeking ||
			video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA
		) {
			this.complete = false;
		}
		return this.videoTexture(video, layer, frame);
	}

	// A decoded frame as the texture of the layer: false while the first frame
	// is not there, "fallback" when this asset is drawn by <video> instead.
	private decodedTexture(
		layer: PlanMediaLayer,
		frame: number,
		mode: DrawMode,
		info: ProjectInfo,
	): boolean | "fallback" {
		if (!this.decoder.handles(layer.asset)) return "fallback";
		const at = Math.min(
			Math.max(frame - layer.first, 0),
			layer.source_seconds.length - 1,
		);
		const seconds = layer.source_seconds[at] ?? 0;
		const forward =
			mode.playing && !videoDrive(layer.source_seconds, at, info.fps).seek;
		const step = Math.round((1_000_000 * info.fps.den) / info.fps.num);
		const result = this.decoder.request(
			layer.asset,
			Math.round(seconds * 1_000_000),
			forward ? "play" : "seek",
			step,
		);
		if (result.state === "fallback") return "fallback";
		if (result.state === "pending") {
			this.complete = false;
			return this.uploaded.has(layer.item);
		}
		if (result.exact) this.decodedUs = result.timestamp;
		else this.complete = false;
		const crop = cropAt(layer, frame);
		if (crop === null) return false;
		const size = this.deliveredSize(layer, frame);
		const tag = `frame:${layer.asset}:${result.timestamp}:${crop.join(",")}:${size?.join("x") ?? ""}`;
		if (this.uploaded.get(layer.item) !== tag) {
			// The clone is the worker's: it closes it once the pixels are copied.
			const copy = new VideoFrame(result.frame);
			this.render?.texture(
				layer.item,
				copy as unknown as PixelSource,
				{
					x: crop[0],
					y: crop[1],
					w: crop[2],
					h: crop[3],
				},
				size,
			);
			this.uploaded.set(layer.item, tag);
		}
		return true;
	}

	// The frame of a <video> element: a bitmap cut from it, uploaded when it
	// is a new picture.
	private videoTexture(
		video: HTMLVideoElement,
		layer: PlanMediaLayer,
		frame: number,
	): boolean {
		if (video.videoWidth === 0) return false;
		const crop = cropAt(layer, frame);
		if (crop === null) return false;
		const size = this.deliveredSize(layer, frame);
		const tag = `video:${layer.asset}:${video.currentTime}:${crop.join(",")}:${size?.join("x") ?? ""}`;
		if (this.uploaded.get(layer.item) === tag) return true;
		if (
			video.seeking ||
			video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA
		) {
			return this.uploaded.has(layer.item);
		}
		this.upload(
			layer.item,
			tag,
			() =>
				createImageBitmap(
					video,
					Math.round(crop[0]),
					Math.round(crop[1]),
					Math.max(1, Math.round(crop[2])),
					Math.max(1, Math.round(crop[3])),
				),
			size,
		);
		this.complete = false;
		return this.uploaded.has(layer.item);
	}

	// The poster of an asset that has no decoding element (beyond
	// MAX_VIDEO_LAYERS) or that is an image, cut to the layer's crop.
	private still(
		projectId: string,
		layer: PlanMediaLayer,
		frame: number,
	): boolean {
		const crop = cropAt(layer, frame);
		if (crop === null) return false;
		const size = this.deliveredSize(layer, frame);
		const tag = `still:${layer.asset}:${crop.join(",")}:${size?.join("x") ?? ""}`;
		if (this.uploaded.get(layer.item) === tag) return true;
		const image = this.stillImages.get(layer.asset);
		if (image === undefined) {
			this.loadStill(projectId, layer.asset);
			this.complete = false;
			return false;
		}
		const source = this.sources(layer.asset);
		const rect = stillFrameRect(
			crop,
			image.naturalWidth,
			image.naturalHeight,
			source?.width ?? image.naturalWidth,
			source?.height ?? image.naturalHeight,
		);
		this.upload(
			layer.item,
			tag,
			() =>
				createImageBitmap(
					image,
					Math.round(rect.x),
					Math.round(rect.y),
					Math.max(1, Math.round(rect.width)),
					Math.max(1, Math.round(rect.height)),
				),
			size,
		);
		this.complete = false;
		return this.uploaded.has(layer.item);
	}

	// Makes a bitmap off the main thread and hands it to the worker; the layer
	// draws on the next frame (a redraw follows).
	private upload(
		item: string,
		tag: string,
		make: () => Promise<ImageBitmap>,
		size: [number, number] | undefined,
	): void {
		const id = `tex:${item}:${tag}`;
		if (this.making.has(id)) return;
		this.making.add(id);
		make()
			.then((bitmap) => {
				this.making.delete(id);
				if (this.disposed || this.render === null) {
					bitmap.close();
					return;
				}
				this.render.texture(
					item,
					bitmap as unknown as PixelSource,
					undefined,
					size,
				);
				this.uploaded.set(item, tag);
				this.redraw();
			})
			.catch(() => this.making.delete(id));
	}

	// The size the pixels of a media layer are delivered at, as the Go host
	// delivers a frame (`target` in engine/export/media.go, SDD-013 section 5.1
	// item 4): the drawn size in output pixels when the layer is not rotated, else
	// the cropped source size (undefined). The effects that work on the layer's
	// own pixels see the same image as in the export.
	private deliveredSize(
		layer: PlanMediaLayer,
		frame: number,
	): [number, number] | undefined {
		if (this.info === null || this.buffer === null) return undefined;
		if (valueAt(layer.rotation, frame, layer.first) !== 0) return undefined;
		const scale = this.leafScale;
		const pixels = (length: number): number =>
			Math.max(Math.floor(length * scale + 0.5), 1);
		return [
			pixels(valueAt(layer.w, frame, layer.first)),
			pixels(valueAt(layer.h, frame, layer.first)),
		];
	}

	private loadStill(projectId: string, asset: string): void {
		const key = `still:${asset}`;
		if (this.making.has(key)) return;
		this.making.add(key);
		const loading = new Image();
		loading.onload = () => {
			this.making.delete(key);
			if (this.disposed) return;
			this.stillImages.set(asset, loading);
			this.redraw();
		};
		loading.onerror = () => this.making.delete(key);
		loading.src = thumbnailUrl(projectId, asset, STILL_POSTER_WIDTH);
	}

	private makeBitmap(
		projectId: string,
		key: string,
		natural: [number, number],
	): void {
		const id = `bitmap:${key}`;
		if (this.making.has(id)) return;
		this.making.add(id);
		fetchBitmap(projectId, key)
			.then((bitmap) => {
				this.making.delete(id);
				if (this.disposed || this.render === null) {
					bitmap.close();
					return;
				}
				// The metrics of the rasterizer centre the block within half a pixel
				// of the bitmap (SDD-004 section 5.3): the preview draws it by its
				// own centre.
				this.render.bitmap(
					key,
					bitmap as unknown as PixelSource,
					natural,
					[0, 0],
				);
				this.bitmaps.set(key, true);
				this.redraw();
			})
			.catch(() => this.making.delete(id));
	}

	private lutState(
		projectId: string,
		asset: string,
	): "loading" | "ready" | "refused" {
		const key = `${projectId}:${asset}`;
		const known = this.luts.get(key);
		if (known !== undefined) return known;
		this.luts.set(key, "loading");
		fetchLut(projectId, asset).then((table) => {
			if (this.disposed || this.render === null) return;
			if (table === null) {
				this.luts.set(key, "refused");
			} else {
				const bytes = new Uint8Array(
					table.entries.buffer,
					table.entries.byteOffset,
					table.entries.byteLength,
				);
				this.render.lut(asset, table.size, bytes.slice());
				this.luts.set(key, "ready");
			}
			this.redraw();
		});
		return "loading";
	}

	private driveVideo(
		video: HTMLVideoElement,
		layer: PlanMediaLayer,
		frame: number,
		mode: DrawMode,
		info: ProjectInfo,
	): void {
		const at = Math.min(
			Math.max(frame - layer.first, 0),
			layer.source_seconds.length - 1,
		);
		const target = layer.source_seconds[at] ?? 0;
		if (mode.playing) {
			const drive = videoDrive(layer.source_seconds, at, info.fps);
			if (drive.seek) {
				video.pause();
				if (video.currentTime !== target) video.currentTime = target;
				return;
			}
			video.playbackRate = drive.rate;
			video.preservesPitch = true;
			if (needsCorrection(video.currentTime, target, info.fps)) {
				video.currentTime = target;
			}
			if (video.paused) void video.play().catch(() => undefined);
			return;
		}
		video.pause();
		if (mode.scrubbing && "fastSeek" in video) video.fastSeek(target);
		else if (video.currentTime !== target) video.currentTime = target;
	}

	private syncAudio(plan: PlanWindow, frame: number, playing: boolean): void {
		this.owners.note(plan);
		const info = this.info;
		if (info === null) return;
		const inUse = new Set<string>();
		for (const audio of plan.audio) {
			if (frame < audio.first || frame > audio.last) continue;
			inUse.add(audio.item);
			const entry = this.sounds.acquire(
				audio.item,
				proxyUrl(this.owners.of(audio.asset, info.projectId), audio.asset),
			);
			if (entry.gain !== null) {
				entry.gain.gain.value = valueAt(audio.gain, frame, audio.first);
			}
			const element = entry.element;
			const drive = audioDrive(audio, frame, info.fps);
			element.playbackRate = drive.rate;
			if (!playing || drive.silent) {
				element.pause();
				continue;
			}
			if (needsCorrection(element.currentTime, drive.seconds, info.fps)) {
				element.currentTime = drive.seconds;
			}
			if (element.paused) void element.play().catch(() => undefined);
		}
		this.sounds.idle(inUse);
	}
}

import "pixi.js/unsafe-eval";
import {
	Application,
	BlurFilter,
	BufferImageSource,
	ColorMatrixFilter,
	Container,
	ImageSource,
	MaskFilter,
	Rectangle,
	Sprite,
	Texture,
	VideoSource,
} from "pixi.js";

import { fetchBitmap } from "../api/bitmaps";
import type { ApiClient } from "../api/client";
import type { PlanLayer, PlanMediaLayer, PlanWindow } from "../api/types";
import { proxyUrl, thumbnailUrl } from "../api/urls";
import "pixi.js/advanced-blend-modes";

import { AudioGraph } from "./audioGraph";
import { pixiBlendMode } from "./blend";
import { ChromaKeyFilter } from "./chromaKey";
import { pixiColorMatrix } from "./layerFilters";
import { Lru } from "./lru";
import { coverageSize, coverageTexels } from "./mask";
import { MediaPool, type PoolTimers } from "./mediaPool";
import {
	audioSourceSeconds,
	MAX_VIDEO_LAYERS,
	needsCorrection,
	PlaybackClock,
	playbackRateFor,
	rankVideoLayers,
} from "./mediaSync";
import { bitmapKeyAt, geometryOf, layersAt, valueAt } from "./planValue";
import { PlanWindows } from "./planWindows";
import { detectRenderer } from "./rendererChoice";
import type {
	LayerGeometry,
	PreviewExecutor,
	PreviewState,
	ProjectInfo,
	Unsubscribe,
} from "./types";

const BITMAP_CACHE = 64;

// The filters of a layer in the order of the compositor: chroma key, colour
// matrix, filter
// matrix, blur (SDD-004 section 6 step 2.2). Each runs in a pass of its own, so
// the result of one is clamped to 0..1 before the next, as the export does.
interface Drawn {
	sprite: Sprite;
	key: ChromaKeyFilter | null;
	color: ColorMatrixFilter | null;
	look: ColorMatrixFilter | null;
	blur: BlurFilter | null;
	mask: MaskDrawn | null;
}

// The mask of a layer: its coverage texture, drawn on a sprite that follows the
// item, and the filter that multiplies the item by it. `key` names what the
// texture was built from.
interface MaskDrawn {
	sprite: Sprite;
	filter: MaskFilter;
	key: string;
}

interface DrawMode {
	playing: boolean;
	scrubbing: boolean;
}

const browserTimers: PoolTimers = {
	setTimeout: (fn, ms) => setTimeout(fn, ms),
	clearTimeout: (handle) => clearTimeout(handle as number),
};

// PixiJS v8 on a WebGPU or WebGL2 canvas drawing exactly what a plan window
// says: the
// background, then each layer at its centre, size, rotation, opacity and colour
// matrix, in plan order, and nothing else (rule T6; SDD-012 section 11.2).
export class PixiExecutor implements PreviewExecutor {
	private app: Application | null = null;
	private readonly stage = new Container();
	private info: ProjectInfo | null = null;
	// The last size asked for: the first resize arrives before the renderer has
	// finished its async init, and is applied when it has.
	private buffer: { width: number; height: number } | null = null;
	private revision = 0;
	private frame = 0;
	private state: PreviewState = "idle";
	private playing = false;
	private raf = 0;
	private canvas: HTMLCanvasElement | null = null;
	// False while a layer of the frame being drawn lacks pixels: a texture that
	// is still loading, or a video that has not yet decoded the frame it was
	// told to show.
	private complete = true;
	private generation = 0;
	private disposed = false;
	private context: AudioContext | null = null;
	private readonly audioGraph = new AudioGraph();
	private clock: PlaybackClock | null = null;
	private windows: PlanWindows;
	private readonly videos: MediaPool;
	private readonly sounds: MediaPool;
	private readonly drawn = new Map<string, Drawn>();
	private readonly bitmaps = new Lru<string, Texture>(BITMAP_CACHE, (_k, t) =>
		t.destroy(true),
	);
	private readonly loading = new Set<string>();
	private readonly stills = new Map<string, Texture>();
	private readonly frameListeners = new Set<(frame: number) => void>();
	private readonly stateListeners = new Set<(state: PreviewState) => void>();

	constructor(
		private readonly client: ApiClient,
		timers: PoolTimers = browserTimers,
	) {
		this.windows = new PlanWindows(() =>
			Promise.reject(new Error("no project")),
		);
		const redraw = (): void => this.redraw();
		this.videos = new MediaPool(
			"video",
			() => this.context,
			() => this.audioGraph.input,
			timers,
			redraw,
		);
		this.sounds = new MediaPool(
			"audio",
			() => this.context,
			() => this.audioGraph.input,
			timers,
			redraw,
		);
	}

	get stats(): PreviewExecutor["stats"] {
		return {
			videoElements: this.videos.size,
			planWindows: this.windows.size,
			droppedFrames: this.videos.droppedFrames(),
		};
	}

	async mount(host: HTMLElement): Promise<void> {
		const canvas = document.createElement("canvas");
		canvas.className = "block size-full";
		host.append(canvas);
		this.canvas = canvas;
		try {
			const preference = await detectRenderer();
			const app = new Application();
			await app.init({
				canvas,
				width: this.info?.width ?? 1080,
				height: this.info?.height ?? 1920,
				preference,
				antialias: false,
				autoStart: false,
				autoDensity: false,
				resolution: 1,
				// The end-to-end build reads the buffer pixels back (preview-vs-export),
				// which a WebGL canvas that clears after each composite cannot give.
				preserveDrawingBuffer: import.meta.env.VITE_E2E === "1",
			});
			if (this.disposed) {
				app.destroy(true);
				return;
			}
			app.stage.addChild(this.stage);
			this.app = app;
			this.applyBuffer();
			this.openAudio();
			this.redraw();
		} catch {
			// No WebGL2: the preview shows only paused frames (truth on pause).
			canvas.remove();
			this.canvas = null;
			this.setState("unsupported");
		}
	}

	// A browser without Web Audio (the WebKit build of Playwright on Windows) still
	// draws the canvas: the preview then plays without the audio graph.
	private openAudio(): void {
		if (typeof AudioContext !== "function") return;
		try {
			this.context = new AudioContext();
			this.audioGraph.attach(this.context);
		} catch {
			this.context = null;
		}
	}

	dispose(): void {
		this.disposed = true;
		this.generation++;
		cancelAnimationFrame(this.raf);
		this.videos.dispose();
		this.sounds.dispose();
		this.bitmaps.clear();
		for (const texture of this.stills.values()) texture.destroy(true);
		this.stills.clear();
		for (const entry of this.drawn.values()) this.dropMask(entry);
		this.app?.destroy({ removeView: true }, { children: true });
		this.app = null;
		this.audioGraph.detach();
		void this.context?.close();
		this.context = null;
		this.frameListeners.clear();
		this.stateListeners.clear();
	}

	setVolume(volume: number, muted: boolean): void {
		this.audioGraph.setLevel(volume, muted);
	}

	setProject(info: ProjectInfo): void {
		this.info = info;
		this.clock = new PlaybackClock(
			() => this.context?.currentTime ?? 0,
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

	resize(bufferWidth: number, bufferHeight: number): void {
		this.buffer = { width: bufferWidth, height: bufferHeight };
		this.windows.setWidth(this.planWidth());
		this.applyBuffer();
		this.redraw();
	}

	// The plan takes a width of 16 to 4096 (SDD-004 section 10).
	private planWidth(): number | undefined {
		if (this.buffer === null) return undefined;
		return Math.min(4096, Math.max(16, Math.round(this.buffer.width)));
	}

	private applyBuffer(): void {
		if (this.app === null || this.info === null || this.buffer === null) return;
		this.app.renderer.resize(this.buffer.width, this.buffer.height);
		// Resizing clears the canvas: nothing is drawn until the next draw.
		this.canvas?.removeAttribute("data-drawn");
		this.stage.scale.set(this.buffer.width / this.info.width);
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
		await this.context?.resume();
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

	// The audio clock drives the frame; the plan windows of the playhead and the
	// next one are kept (SDD-012 section 11.2 items 1 and 4).
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
			this.draw(plan, this.frame, { playing: true, scrubbing: false });
			this.windows.prefetch(this.frame);
		}
		this.emitFrame();
		this.raf = requestAnimationFrame(this.tick);
	};

	private redraw(): void {
		if (this.app === null || this.playing || this.info === null) return;
		const plan = this.windows.peek(this.frame);
		if (plan === undefined) this.seek(this.frame);
		else this.draw(plan, this.frame, { playing: false, scrubbing: false });
	}

	private draw(plan: PlanWindow, frame: number, mode: DrawMode): void {
		const app = this.app;
		const info = this.info;
		if (app === null || info === null) return;
		app.renderer.background.color = plan.background;

		const layers = layersAt(plan, frame);
		const decoding = rankVideoLayers(layers, MAX_VIDEO_LAYERS);
		const usedVideos = new Set<string>();
		const used = new Set<string>();
		this.complete = true;

		let index = 0;
		for (const layer of layers) {
			const texture = this.textureFor(
				layer,
				frame,
				decoding,
				usedVideos,
				mode,
				info,
			);
			if (texture === null) {
				this.complete = false;
				continue;
			}
			used.add(layer.item);
			let entry = this.drawn.get(layer.item);
			if (entry === undefined) {
				entry = {
					sprite: new Sprite(),
					key: null,
					color: null,
					look: null,
					blur: null,
					mask: null,
				};
				entry.sprite.anchor.set(0.5);
				this.drawn.set(layer.item, entry);
			}
			if (entry.sprite.texture !== texture) entry.sprite.texture = texture;
			this.place(entry, layer, frame);
			if (this.stage.children[index] !== entry.sprite) {
				this.stage.addChildAt(
					entry.sprite,
					Math.min(index, this.stage.children.length),
				);
			}
			index++;
		}

		for (const [item, entry] of this.drawn) {
			if (used.has(item)) continue;
			entry.sprite.removeFromParent();
			this.dropMask(entry);
			entry.sprite.destroy();
			this.drawn.delete(item);
		}
		this.videos.idle(usedVideos);
		this.syncAudio(plan, frame, mode.playing);
		app.render();
		this.markDrawn(frame);
	}

	// The canvas says which frame it shows once every layer of that frame is on
	// it, so a test (or a reader of the page) waits on that instead of a timer.
	// While playing, frames come too fast to be worth an attribute write.
	private markDrawn(frame: number): void {
		const canvas = this.canvas;
		if (canvas === null || this.playing) return;
		if (!this.complete) {
			canvas.removeAttribute("data-drawn");
			return;
		}
		const value = String(frame);
		if (canvas.getAttribute("data-drawn") !== value) {
			canvas.setAttribute("data-drawn", value);
		}
	}

	private place(entry: Drawn, layer: PlanLayer, frame: number): void {
		const g = geometryOf(layer, frame);
		const { sprite } = entry;
		sprite.position.set(g.cx, g.cy);
		sprite.width = g.w;
		sprite.height = g.h;
		sprite.angle = g.rotation;
		sprite.alpha = valueAt(layer.opacity, frame, layer.first);
		sprite.blendMode = pixiBlendMode(layer.blend);
		this.applyFilters(entry, layer, g);
	}

	private applyFilters(entry: Drawn, layer: PlanLayer, g: LayerGeometry): void {
		const media = layer.kind === "media" ? layer : null;
		const before = [
			entry.key,
			entry.color,
			entry.look,
			entry.blur,
			entry.mask?.filter ?? null,
		];
		if (media?.chroma_key) {
			entry.key ??= new ChromaKeyFilter();
			entry.key.set(media.chroma_key);
		} else {
			entry.key = null;
		}
		entry.color = this.matrixFilter(entry.color, media?.color_matrix ?? null);
		entry.look = this.matrixFilter(entry.look, media?.filter ?? null);
		// The blur is in project pixels; the stage scale is the output scale.
		const sigma = (media?.blur ?? 0) * this.stage.scale.x;
		if (sigma > 0) {
			entry.blur ??= new BlurFilter();
			entry.blur.strength = sigma;
		} else {
			entry.blur = null;
		}
		// The mask goes last: it is applied after the key, colour, filter and blur,
		// and the opacity and the blend of the sprite come after it (SDD-004 3.9).
		this.applyMask(entry, layer, g);
		const now = [
			entry.key,
			entry.color,
			entry.look,
			entry.blur,
			entry.mask?.filter ?? null,
		];
		if (now.some((f, i) => f !== before[i])) {
			entry.sprite.filters = now.filter((f) => f !== null);
		}
	}

	// The coverage texture covers the layer rectangle (the plan's W by H, in
	// output pixels, rounded so a keyframed scale does not rebuild it on every
	// frame) and its sprite takes the item's position, size and rotation, so the
	// mask is in the item's own frame.
	private applyMask(entry: Drawn, layer: PlanLayer, g: LayerGeometry): void {
		const mask = layer.mask;
		if (mask === undefined) {
			this.dropMask(entry);
			return;
		}
		const scale = this.stage.scale.x;
		const w = Math.max(1, Math.round(g.w * scale));
		const h = Math.max(1, Math.round(g.h * scale));
		const key = `${JSON.stringify(mask)}|${w}|${h}|${scale}`;
		let drawn = entry.mask;
		if (drawn === null || drawn.key !== key) {
			const size = coverageSize(w, h);
			const texture = new Texture({
				source: new BufferImageSource({
					resource: coverageTexels(mask, w, h, scale, size.width, size.height),
					width: size.width,
					height: size.height,
					format: "rgba8unorm",
					alphaMode: "premultiply-alpha-on-upload",
					scaleMode: "linear",
				}),
			});
			if (drawn === null) {
				const sprite = new Sprite(texture);
				sprite.anchor.set(0.5);
				drawn = {
					sprite,
					filter: new MaskFilter({ sprite, channel: "alpha" }),
					key,
				};
				entry.mask = drawn;
			} else {
				const old = drawn.sprite.texture;
				drawn.sprite.texture = texture;
				old.destroy(true);
				drawn.key = key;
			}
		}
		// The sprite is not in the scene, so its world transform is set here, in
		// output pixels (the stage is at the origin and only scaled).
		const { sprite } = drawn;
		sprite.position.set(g.cx * scale, g.cy * scale);
		sprite.width = g.w * scale;
		sprite.height = g.h * scale;
		sprite.angle = g.rotation;
		sprite.updateLocalTransform();
		sprite.worldTransform.copyFrom(sprite.localTransform);
	}

	private dropMask(entry: Drawn): void {
		const drawn = entry.mask;
		if (drawn === null) return;
		entry.mask = null;
		entry.sprite.filters = null;
		drawn.filter.destroy();
		drawn.sprite.destroy({ texture: true, textureSource: true });
	}

	private matrixFilter(
		current: ColorMatrixFilter | null,
		matrix: readonly number[] | null,
	): ColorMatrixFilter | null {
		if (matrix === null) return null;
		const filter = current ?? new ColorMatrixFilter();
		filter.matrix = pixiColorMatrix(matrix) as ColorMatrixFilter["matrix"];
		return filter;
	}

	private textureFor(
		layer: PlanLayer,
		frame: number,
		decoding: ReadonlySet<string>,
		usedVideos: Set<string>,
		mode: DrawMode,
		info: ProjectInfo,
	): Texture | null {
		if (layer.kind === "bitmap") {
			const key = bitmapKeyAt(layer, frame);
			if (key === undefined) return null;
			const cached = this.bitmaps.get(key);
			if (cached !== undefined) return cached;
			this.loadBitmap(info.projectId, key);
			return null;
		}
		if (layer.asset_kind === "image" || !decoding.has(layer.item)) {
			return this.still(info.projectId, layer);
		}
		usedVideos.add(layer.asset);
		const { element } = this.videos.acquire(
			layer.asset,
			proxyUrl(info.projectId, layer.asset),
		);
		const video = element as HTMLVideoElement;
		this.driveVideo(video, layer, frame, mode, info);
		if (
			video.seeking ||
			video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA
		) {
			this.complete = false;
		}
		return this.videoTexture(video, layer);
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
			video.playbackRate = playbackRateFor(layer.source_seconds, info.fps);
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

	private videoTexture(
		video: HTMLVideoElement,
		layer: PlanMediaLayer,
	): Texture | null {
		// Without autoLoad the source never learns the size of the video, and a
		// frame rectangle on a 1x1 source samples one texel: a flat color. The
		// element redraws when its metadata arrives (MediaPool).
		if (video.videoWidth === 0) return null;
		const key = `video:${layer.asset}:${layer.crop.join(",")}`;
		let texture = this.stills.get(key);
		if (texture === undefined) {
			const source = new VideoSource({
				resource: video,
				autoPlay: false,
				autoLoad: false,
			});
			source.resize(video.videoWidth, video.videoHeight);
			const [x, y, w, h] = layer.crop;
			texture = new Texture({ source, frame: new Rectangle(x, y, w, h) });
			this.stills.set(key, texture);
		}
		texture.source.update();
		return texture;
	}

	// The thumbnail still of an asset that has no decoding element (beyond
	// MAX_VIDEO_LAYERS) or that is an image.
	private still(projectId: string, layer: PlanMediaLayer): Texture | null {
		const key = `still:${layer.asset}`;
		const cached = this.stills.get(key);
		if (cached !== undefined) return cached;
		if (this.loading.has(key)) return null;
		this.loading.add(key);
		const image = new Image();
		image.onload = () => {
			this.loading.delete(key);
			if (this.disposed) return;
			this.stills.set(
				key,
				new Texture({ source: new ImageSource({ resource: image }) }),
			);
			this.redraw();
		};
		image.onerror = () => this.loading.delete(key);
		image.src = thumbnailUrl(projectId, layer.asset, 640);
		return null;
	}

	private loadBitmap(projectId: string, key: string): void {
		const id = `bitmap:${key}`;
		if (this.loading.has(id)) return;
		this.loading.add(id);
		fetchBitmap(projectId, key)
			.then((bitmap) => {
				this.loading.delete(id);
				if (this.disposed) return;
				this.bitmaps.set(
					key,
					new Texture({ source: new ImageSource({ resource: bitmap }) }),
				);
				this.redraw();
			})
			.catch(() => this.loading.delete(id));
	}

	private syncAudio(plan: PlanWindow, frame: number, playing: boolean): void {
		const info = this.info;
		if (info === null) return;
		const inUse = new Set<string>();
		for (const audio of plan.audio) {
			if (frame < audio.first || frame > audio.last) continue;
			inUse.add(audio.item);
			const entry = this.sounds.acquire(
				audio.item,
				proxyUrl(info.projectId, audio.asset),
			);
			if (entry.gain !== null) {
				entry.gain.gain.value = valueAt(audio.gain, frame, audio.first);
			}
			const element = entry.element;
			element.playbackRate = audio.speed;
			if (!playing) {
				element.pause();
				continue;
			}
			const target = audioSourceSeconds(audio, frame, info.fps);
			if (needsCorrection(element.currentTime, target, info.fps)) {
				element.currentTime = target;
			}
			if (element.paused) void element.play().catch(() => undefined);
		}
		this.sounds.idle(inUse);
	}
}

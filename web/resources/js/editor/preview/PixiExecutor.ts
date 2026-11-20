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
import { fetchLut, type LutTable } from "../api/lut";
import type { PlanLayer, PlanMediaLayer, PlanWindow } from "../api/types";
import { chunksUrl, proxyUrl, thumbnailUrl } from "../api/urls";
import { setProcessedAudioStates } from "../state/processedAudioStore";
import "pixi.js/advanced-blend-modes";

import { AudioGraph } from "./audioGraph";
import { uncoveredPixel } from "./background";
import { pixiBlendMode } from "./blend";
import { ChromaKeyFilter } from "./chromaKey";
import { DecodeClient, decoderPreference } from "./decode/client";
import { createDecodeWorker } from "./decode/createWorker";
import { EffectFilter } from "./effectFilter";
import { type EffectPass, effectPasses } from "./effects";
import { GradeFilter } from "./gradeFilter";
import { pixiColorMatrix } from "./layerFilters";
import { Lru } from "./lru";
import { LutFilter } from "./lutFilter";
import { coverageSize, coverageTexels } from "./mask";
import { MediaPool, type PoolTimers } from "./mediaPool";
import {
	audioClockReady,
	audioDrive,
	MAX_VIDEO_LAYERS,
	needsCorrection,
	PlaybackClock,
	processedDrive,
	rankVideoLayers,
	videoDrive,
} from "./mediaSync";
import {
	bitmapKeyAt,
	colorMatrixAt,
	cropAt,
	geometryOf,
	layersAt,
	maskAt,
	mirrorSigns,
	valueAt,
} from "./planValue";
import { PlanWindows } from "./planWindows";
import { ProcessedAudio } from "./processedAudio";
import { detectRenderer } from "./rendererChoice";
import {
	type PlacedPixels,
	type ShadowOutlineImage,
	shadowOutline,
} from "./shadowOutline";
import { STILL_POSTER_WIDTH, stillFrameRect } from "./stillFrame";
import type {
	LayerGeometry,
	PreviewExecutor,
	PreviewState,
	ProjectInfo,
	Unsubscribe,
} from "./types";

const BITMAP_CACHE = 64;

// The filters of a layer in the order of the compositor: chroma key, colour
// matrix, grade, lookup table, filter matrix, blur (SDD-004 section 6 step 2.2). Each runs in a pass of its own, so
// the result of one is clamped to 0..1 before the next, as the export does.
interface Drawn {
	// The filters, the opacity and the blend are on the holder, which has no
	// transform of its own: a filter area is in the local space of the object
	// that has it, and the passes of effects.ts need one they can place.
	holder: Container;
	sprite: Sprite;
	key: ChromaKeyFilter | null;
	color: ColorMatrixFilter | null;
	grade: GradeFilter | null;
	lut: LutFilter | null;
	look: ColorMatrixFilter | null;
	blur: BlurFilter | null;
	// The passes of the pixel effects and the transitions, reused from frame to
	// frame; the ones before the mask come first (effects.ts).
	fx: EffectFilter[];
	mask: MaskDrawn | null;
	// The shadow, then the outline, under the picture. Both are children of the
	// holder, so the mask filter cuts the three together (SDD-004 section 3.15).
	shadowSprite: Sprite | null;
	outlineSprite: Sprite | null;
	shadowPlace: PaintBox | null;
	outlinePlace: PaintBox | null;
	shadowKey: string;
	shadowSrcW: number;
	shadowSrcH: number;
}

interface PaintBox {
	x: number;
	y: number;
	width: number;
	height: number;
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
// says: the background, then each layer at its centre, size, rotation, opacity
// and colour matrix, with its shadow and outline, in plan order, and nothing
// else (rule T6; SDD-012 section 11.2).
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
	// False when the context cannot run (no audio device): the clock is then
	// the wall clock. Decided at each play(), so it never changes mid-playback.
	private audioClock = true;
	private readonly audioGraph = new AudioGraph();
	private clock: PlaybackClock | null = null;
	private windows: PlanWindows;
	private readonly videos: MediaPool;
	private readonly sounds: MediaPool;
	private readonly processedAudio: ProcessedAudio;
	// Decodes the proxies with WebCodecs in a worker; an asset it cannot decode
	// is drawn from a pooled <video> (SDD-013 section 5.3).
	private readonly decoder: DecodeClient<VideoFrame>;
	// The timestamp, in microseconds, of the last decoded frame drawn exactly.
	private decodedUs = -1;
	private readonly drawn = new Map<string, Drawn>();
	private readonly bitmaps = new Lru<string, Texture>(BITMAP_CACHE, (_k, t) =>
		t.destroy(true),
	);
	private readonly loading = new Set<string>();
	private readonly stills = new Map<string, Texture>();
	// The poster of a still asset, shared by every crop of that asset.
	private readonly stillImages = new Map<string, HTMLImageElement>();
	// Pixels of a layer, read once per source, to paint its shadow and outline.
	private shadowScratch: HTMLCanvasElement | null = null;
	private readonly shadowStamps = new WeakMap<object, number>();
	private shadowSerial = 1;
	// The parsed table of each lut asset; null once it cannot be had, so the
	// layer is drawn without it and the route is not asked again.
	private readonly luts = new Map<string, LutTable | null>();
	private readonly frameListeners = new Set<(frame: number) => void>();
	private readonly stateListeners = new Set<(state: PreviewState) => void>();

	constructor(
		private readonly client: ApiClient,
		timers?: PoolTimers,
		// Asset pixel size, the space the plan's crop is measured in. Absent
		// means the poster is the source (a still at most STILL_POSTER_WIDTH wide).
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
			indexUrl: (asset) => chunksUrl(this.info?.projectId ?? "", asset),
			proxyUrl: (asset) => proxyUrl(this.info?.projectId ?? "", asset),
			onChange: redraw,
			now: () => performance.now(),
		});
		this.videos = new MediaPool(
			"video",
			() => this.context,
			() => this.audioGraph.input,
			poolTimers,
			redraw,
		);
		this.sounds = new MediaPool(
			"audio",
			() => this.context,
			() => this.audioGraph.input,
			poolTimers,
			redraw,
		);
		this.processedAudio = new ProcessedAudio({
			projectId: () => this.info?.projectId ?? "",
			timers: poolTimers,
			now: () => performance.now(),
			onChange: redraw,
		});
	}

	get stats(): PreviewExecutor["stats"] {
		return {
			videoElements: this.videos.size,
			planWindows: this.windows.size,
			droppedFrames: this.videos.droppedFrames(),
			...this.decoder.stats,
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
				// Below 1 the canvas keeps an alpha channel. PixiJS cannot add one
				// later, so a transparent project can clear to alpha 0; an opaque
				// colour still sets alpha to 1 on each draw.
				backgroundAlpha: 0,
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
		this.processedAudio.dispose();
		setProcessedAudioStates({});
		this.decoder.dispose();
		this.bitmaps.clear();
		for (const texture of this.stills.values()) texture.destroy(true);
		this.stills.clear();
		this.luts.clear();
		for (const entry of this.drawn.values()) {
			this.dropMask(entry);
			this.dropShadowOutline(entry);
			this.sizeFx(entry, 0);
			entry.grade?.destroy();
		}
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
			() =>
				this.audioClock && this.context !== null
					? this.context.currentTime
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
		this.audioClock = await audioClockReady(this.context);
		if (this.disposed) return;
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

	readColour(x: number, y: number): string | null {
		const app = this.app;
		const info = this.info;
		if (app === null || info === null || this.buffer === null) return null;
		const ratio = this.buffer.width / info.width;
		const px = Math.min(
			this.buffer.width - 1,
			Math.max(0, Math.floor(x * ratio)),
		);
		const py = Math.min(
			this.buffer.height - 1,
			Math.max(0, Math.floor(y * ratio)),
		);
		const { pixels } = app.renderer.extract.pixels({
			target: this.stage,
			frame: new Rectangle(px, py, 1, 1),
			resolution: 1,
		});
		const hex = (n: number | undefined): string =>
			(n ?? 0).toString(16).padStart(2, "0");
		return `#${hex(pixels[0])}${hex(pixels[1])}${hex(pixels[2])}`.toUpperCase();
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
		const [red, green, blue, alpha] = uncoveredPixel(plan);
		app.renderer.background.color = [
			red / 255,
			green / 255,
			blue / 255,
			alpha / 255,
		];

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
				const holder = new Container();
				entry = {
					holder,
					sprite: new Sprite(),
					key: null,
					color: null,
					grade: null,
					lut: null,
					look: null,
					blur: null,
					fx: [],
					mask: null,
					shadowSprite: null,
					outlineSprite: null,
					shadowPlace: null,
					outlinePlace: null,
					shadowKey: "",
					shadowSrcW: 0,
					shadowSrcH: 0,
				};
				entry.sprite.anchor.set(0.5);
				holder.addChild(entry.sprite);
				this.drawn.set(layer.item, entry);
			}
			if (entry.sprite.texture !== texture) entry.sprite.texture = texture;
			this.place(entry, layer, frame, info);
			if (this.stage.children[index] !== entry.holder) {
				this.stage.addChildAt(
					entry.holder,
					Math.min(index, this.stage.children.length),
				);
			}
			index++;
		}

		for (const [item, entry] of this.drawn) {
			if (used.has(item)) continue;
			entry.holder.removeFromParent();
			this.dropMask(entry);
			this.dropShadowOutline(entry);
			this.sizeFx(entry, 0);
			entry.grade?.destroy();
			entry.lut?.destroy();
			entry.holder.destroy({ children: true });
			this.drawn.delete(item);
		}
		this.videos.idle(usedVideos);
		this.syncAudio(plan, frame, mode.playing);
		app.render();
		// A frame evicted from the decode cache is closed only after the render
		// that may have read it.
		this.decoder.sweep();
		this.publishDecode();
		this.markDrawn(frame);
	}

	// What the page shows about the decode path, for the end-to-end specs: the
	// open VideoFrames and their peak, the assets that fell back to <video> and
	// the timestamp of the last frame drawn exactly. Written on change only.
	private publishDecode(): void {
		const canvas = this.canvas;
		if (canvas === null) return;
		const { openFrames, peakOpenFrames, fallbackAssets } = this.decoder.stats;
		const values: Record<string, string> = {
			"data-open-frames": String(openFrames),
			"data-peak-frames": String(peakOpenFrames),
			"data-fallback-assets": String(fallbackAssets),
		};
		if (this.complete && this.decodedUs >= 0) {
			values["data-frame-us"] = String(this.decodedUs);
		}
		for (const [name, value] of Object.entries(values)) {
			if (canvas.getAttribute(name) !== value) canvas.setAttribute(name, value);
		}
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

	private place(
		entry: Drawn,
		layer: PlanLayer,
		frame: number,
		info: ProjectInfo,
	): void {
		const g = geometryOf(layer, frame);
		const { sprite } = entry;
		sprite.position.set(g.cx, g.cy);
		sprite.width = g.w;
		sprite.height = g.h;
		// The mirror turns the picture around the centre of the unrotated
		// rectangle; the rotation below is applied after it (SDD-004 section 3.10).
		// The mask and the chroma key keep the frame of the item, so they do not
		// take part.
		const mirror = mirrorSigns(layer);
		sprite.scale.x = mirror.x * Math.abs(sprite.scale.x);
		sprite.scale.y = mirror.y * Math.abs(sprite.scale.y);
		sprite.angle = g.rotation;
		entry.holder.alpha = valueAt(layer.opacity, frame, layer.first);
		entry.holder.blendMode = pixiBlendMode(layer.blend);
		this.applyFilters(entry, layer, g, frame, info);
		// Under the picture, on the holder, so the mask cuts all three together.
		this.applyShadowOutline(entry, layer, g);
	}

	private applyFilters(
		entry: Drawn,
		layer: PlanLayer,
		g: LayerGeometry,
		frame: number,
		info: ProjectInfo,
	): void {
		const media = layer.kind === "media" ? layer : null;
		if (media?.chroma_key) {
			entry.key ??= new ChromaKeyFilter();
			entry.key.set(media.chroma_key);
		} else {
			entry.key = null;
		}
		entry.color = this.matrixFilter(entry.color, colorMatrixAt(layer, frame));
		const gradeOn = this.gradeFilter(entry, media?.grade);
		const lutOn = this.lutFilter(entry, media?.lut, info);
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
		this.applyMask(entry, layer, g, frame);
		const passes = effectPasses(layer, frame, {
			scale: this.stage.scale.x,
			rotation: g.rotation,
			frameWidth: info.width,
			frameHeight: info.height,
		});
		const used = [...passes.before, ...passes.after];
		this.sizeFx(entry, used.length);
		const now = [
			entry.key,
			entry.color,
			gradeOn ? entry.grade : null,
			lutOn ? entry.lut : null,
			entry.look,
			entry.blur,
			...entry.fx.slice(0, passes.before.length),
			entry.mask?.filter ?? null,
			...entry.fx.slice(passes.before.length),
		].filter((f) => f !== null);
		const current = Array.isArray(entry.holder.filters)
			? entry.holder.filters
			: [];
		if (current.length !== now.length || now.some((f, i) => f !== current[i])) {
			entry.holder.filters = now.length > 0 ? now : null;
		}
		this.placeFx(entry, used, now, g);
	}

	private sizeFx(entry: Drawn, count: number): void {
		while (entry.fx.length < count) entry.fx.push(new EffectFilter());
		for (const extra of entry.fx.splice(count)) extra.destroy();
	}

	// The effects need to know where the layer is inside the texture they read,
	// which PixiJS does not say. So the area of the filters is set here, as the
	// box of the rotated layer grown by what the passes add, and the corner of
	// the texture is that area less the padding the other filters ask for (the
	// sum, cut to a whole number, as FilterSystem does).
	private placeFx(
		entry: Drawn,
		passes: EffectPass[],
		filters: readonly { padding: number }[],
		g: LayerGeometry,
	): void {
		const { holder } = entry;
		if (passes.length === 0) {
			holder.filterArea = undefined;
			return;
		}
		const scale = this.stage.scale.x;
		const grow = passes.reduce((sum, pass) => sum + pass.padding, 0);
		const halfW = (g.w * scale) / 2;
		const halfH = (g.h * scale) / 2;
		const turn = (g.rotation * Math.PI) / 180;
		const ex =
			Math.abs(Math.cos(turn)) * halfW + Math.abs(Math.sin(turn)) * halfH;
		const ey =
			Math.abs(Math.sin(turn)) * halfW + Math.abs(Math.cos(turn)) * halfH;
		const cx = g.cx * scale;
		const cy = g.cy * scale;
		const left = Math.floor(cx - ex - grow);
		const top = Math.floor(cy - ey - grow);
		// The area is in the local space of the holder, which is the stage's: the
		// project pixels, so the output pixels divided by the scale.
		holder.filterArea = new Rectangle(
			left / scale,
			top / scale,
			(Math.ceil(cx + ex + grow) - left) / scale,
			(Math.ceil(cy + ey + grow) - top) / scale,
		);
		const others = filters.reduce((sum, f) => sum + f.padding, 0) | 0;
		const geometry = {
			cx: cx - (left - others),
			cy: cy - (top - others),
			halfWidth: halfW,
			halfHeight: halfH,
			rotation: g.rotation,
		};
		passes.forEach((pass, i) => {
			entry.fx[i]?.set(pass, geometry);
		});
	}

	// The coverage texture covers the layer rectangle (the plan's W by H, in
	// output pixels, rounded so a keyframed scale does not rebuild it on every
	// frame) and its sprite takes the item's position, size and rotation, so the
	// mask is in the item's own frame.
	private applyMask(
		entry: Drawn,
		layer: PlanLayer,
		g: LayerGeometry,
		frame: number,
	): void {
		const mask = maskAt(layer, frame);
		if (mask === null) {
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
		entry.holder.filters = null;
		drawn.filter.destroy();
		drawn.sprite.destroy({ texture: true, textureSource: true });
	}

	// The shadow and the outline are constant on the item. The picture sprite
	// stays; these two sit under it in source pixels of the same texture.
	private applyShadowOutline(
		entry: Drawn,
		layer: PlanLayer,
		g: LayerGeometry,
	): void {
		if (layer.shadow === undefined && layer.outline === undefined) {
			this.dropShadowOutline(entry);
			return;
		}
		const scale = this.stage.scale.x;
		const key = this.shadowCacheKey(
			entry.sprite.texture,
			layer,
			scale,
			g.w,
			g.h,
		);
		if (key === null) {
			this.dropShadowOutline(entry);
			return;
		}
		if (entry.shadowKey !== key) {
			const read = this.readLayerPixels(entry.sprite.texture, layer);
			if (read === null) {
				this.dropShadowOutline(entry);
				return;
			}
			const painted = shadowOutline(
				read.pixels,
				read.width,
				read.height,
				layer,
				scale,
				g.w * scale,
				g.h * scale,
			);
			this.installPaint(entry, painted);
			entry.shadowSrcW = read.width;
			entry.shadowSrcH = read.height;
			entry.shadowKey = key;
		}
		this.placePaint(entry, g);
	}

	private shadowCacheKey(
		texture: Texture,
		layer: PlanLayer,
		scale: number,
		w: number,
		h: number,
	): string | null {
		const resource: unknown = texture.source?.resource;
		if (!isDrawable(resource)) return null;
		const frame = texture.frame;
		const moving =
			resource instanceof HTMLVideoElement ? resource.currentTime : "";
		const shadow = layer.shadow;
		const outline = layer.outline;
		return [
			texture.uid,
			this.objectStamp(resource),
			moving,
			frame.x,
			frame.y,
			frame.width,
			frame.height,
			scale,
			w,
			h,
			layer.flip_x === true ? 1 : 0,
			layer.flip_y === true ? 1 : 0,
			shadow?.color ?? "",
			shadow?.opacity ?? "",
			shadow?.dx ?? "",
			shadow?.dy ?? "",
			shadow?.blur ?? "",
			outline?.color ?? "",
			outline?.width ?? "",
		].join("|");
	}

	private objectStamp(resource: object): number {
		let id = this.shadowStamps.get(resource);
		if (id === undefined) {
			id = this.shadowSerial++;
			this.shadowStamps.set(resource, id);
		}
		return id;
	}

	private readLayerPixels(
		texture: Texture,
		layer: PlanLayer,
	): { pixels: Uint8Array; width: number; height: number } | null {
		const resource: unknown = texture.source?.resource;
		if (!isDrawable(resource) || typeof document === "undefined") return null;
		const frame = texture.frame;
		const width = Math.round(frame.width);
		const height = Math.round(frame.height);
		if (width < 1 || height < 1) return null;
		const canvas = this.shadowScratch ?? document.createElement("canvas");
		this.shadowScratch = canvas;
		if (canvas.width !== width || canvas.height !== height) {
			canvas.width = width;
			canvas.height = height;
		}
		const ctx = canvas.getContext("2d", { willReadFrequently: true });
		if (ctx === null) return null;
		ctx.imageSmoothingEnabled = false;
		ctx.clearRect(0, 0, width, height);
		try {
			ctx.drawImage(
				resource,
				frame.x,
				frame.y,
				frame.width,
				frame.height,
				0,
				0,
				width,
				height,
			);
			const view = ctx.getImageData(0, 0, width, height).data;
			return {
				pixels: mirrorPixels(new Uint8Array(view), width, height, layer),
				width,
				height,
			};
		} catch (err) {
			// A frame the decoder already closed, or a canvas the browser will not read.
			if (err instanceof DOMException) return null;
			throw err;
		}
	}

	private installPaint(entry: Drawn, painted: ShadowOutlineImage): void {
		entry.shadowSprite = this.replacePaint(
			entry.shadowSprite,
			painted.shadow,
			entry.holder,
		);
		entry.outlineSprite = this.replacePaint(
			entry.outlineSprite,
			painted.outline,
			entry.holder,
		);
		entry.shadowPlace = paintBox(painted.shadow);
		entry.outlinePlace = paintBox(painted.outline);
		this.orderPaint(entry);
	}

	private replacePaint(
		current: Sprite | null,
		painted: PlacedPixels | null,
		holder: Container,
	): Sprite | null {
		if (painted === null) {
			this.destroyPaint(current);
			return null;
		}
		const texture = new Texture({
			source: new BufferImageSource({
				resource: painted.pixels,
				width: painted.width,
				height: painted.height,
				format: "rgba8unorm",
				alphaMode: "premultiplied-alpha",
				scaleMode: "linear",
			}),
		});
		if (current === null) {
			const sprite = new Sprite(texture);
			sprite.anchor.set(0.5);
			holder.addChild(sprite);
			return sprite;
		}
		const old = current.texture;
		current.texture = texture;
		old.destroy(true);
		return current;
	}

	private placePaint(entry: Drawn, g: LayerGeometry): void {
		const srcW = entry.shadowSrcW;
		const srcH = entry.shadowSrcH;
		if (!(srcW > 0) || !(srcH > 0)) return;
		if (entry.shadowSprite !== null && entry.shadowPlace !== null) {
			placePainted(entry.shadowSprite, entry.shadowPlace, srcW, srcH, g);
		}
		if (entry.outlineSprite !== null && entry.outlinePlace !== null) {
			placePainted(entry.outlineSprite, entry.outlinePlace, srcW, srcH, g);
		}
		this.orderPaint(entry);
	}

	private orderPaint(entry: Drawn): void {
		const { holder, sprite } = entry;
		let at = 0;
		if (entry.shadowSprite !== null)
			holder.addChildAt(entry.shadowSprite, at++);
		if (entry.outlineSprite !== null)
			holder.addChildAt(entry.outlineSprite, at);
		holder.addChild(sprite);
	}

	private dropShadowOutline(entry: Drawn): void {
		if (
			entry.shadowSprite === null &&
			entry.outlineSprite === null &&
			entry.shadowKey === ""
		) {
			return;
		}
		this.destroyPaint(entry.shadowSprite);
		this.destroyPaint(entry.outlineSprite);
		entry.shadowSprite = null;
		entry.outlineSprite = null;
		entry.shadowPlace = null;
		entry.outlinePlace = null;
		entry.shadowKey = "";
		entry.shadowSrcW = 0;
		entry.shadowSrcH = 0;
	}

	private destroyPaint(sprite: Sprite | null): void {
		if (sprite === null) return;
		sprite.removeFromParent();
		sprite.destroy({ texture: true, textureSource: true });
	}

	// The advanced colour comes right after the colour matrix (SDD-004 section
	// 3.14); a grade that changes no pixel leaves the filter out.
	private gradeFilter(entry: Drawn, grade: PlanMediaLayer["grade"]): boolean {
		if (grade === undefined) {
			entry.grade?.destroy();
			entry.grade = null;
			return false;
		}
		entry.grade ??= new GradeFilter();
		return entry.grade.set(grade);
	}

	// The lookup table sits between the grade and the filter (SDD-004 section
	// 3.6). While its table is on the way the frame is not complete, so a
	// capture waits; a table that cannot be had is drawn without.
	private lutFilter(
		entry: Drawn,
		lut: PlanMediaLayer["lut"],
		info: ProjectInfo,
	): boolean {
		const table = lut === undefined ? null : this.lutTable(info, lut.asset);
		if (lut === undefined || table === null || table === undefined) {
			if (table === undefined) this.complete = false;
			entry.lut?.destroy();
			entry.lut = null;
			return false;
		}
		entry.lut ??= new LutFilter();
		return entry.lut.set(table, lut.intensity);
	}

	// The table of an asset: undefined while it loads, null when it is refused.
	private lutTable(
		info: ProjectInfo,
		asset: string,
	): LutTable | null | undefined {
		const key = `${info.projectId}:${asset}`;
		const cached = this.luts.get(key);
		if (cached !== undefined) return cached;
		const id = `lut:${key}`;
		if (this.loading.has(id)) return undefined;
		this.loading.add(id);
		fetchLut(info.projectId, asset).then((table) => {
			this.loading.delete(id);
			if (this.disposed) return;
			this.luts.set(key, table);
			this.redraw();
		});
		return undefined;
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
			return this.still(info.projectId, layer, frame);
		}
		const decoded = this.decodedTexture(layer, frame, mode, info);
		if (decoded !== "fallback") return decoded;
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
		return this.videoTexture(video, layer, frame);
	}

	// The layer drawn from a decoded frame: null while the first frame is not
	// there, "fallback" when this asset is drawn by <video> instead.
	private decodedTexture(
		layer: PlanMediaLayer,
		frame: number,
		mode: DrawMode,
		info: ProjectInfo,
	): Texture | null | "fallback" {
		if (!this.decoder.handles(layer.asset)) return "fallback";
		const at = Math.min(
			Math.max(frame - layer.first, 0),
			layer.source_seconds.length - 1,
		);
		const seconds = layer.source_seconds[at] ?? 0;
		// A reversed item reads backwards, which a decoder does not do forward:
		// every frame is a seek.
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
			return null;
		}
		if (result.exact) this.decodedUs = result.timestamp;
		else this.complete = false;
		return this.frameTexture(layer, result.frame, frame);
	}

	// One texture per asset and crop whose source is swapped for each new
	// frame; the frame stays open until the decode cache evicts it.
	private frameTexture(
		layer: PlanMediaLayer,
		bitmap: VideoFrame,
		frame: number,
	): Texture | null {
		const crop = cropAt(layer, frame);
		if (crop === null) return null;
		const key = `frame:${layer.asset}:${crop.join(",")}`;
		let texture = this.stills.get(key);
		if (texture === undefined) {
			const source = new ImageSource({ resource: bitmap });
			const [x, y, w, h] = crop;
			texture = new Texture({ source, frame: new Rectangle(x, y, w, h) });
			this.stills.set(key, texture);
		} else if (texture.source.resource !== bitmap) {
			texture.source.resource = bitmap;
			texture.source.update();
		}
		return texture;
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
				// A reversed item: one seek per frame, the element stays paused.
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

	private videoTexture(
		video: HTMLVideoElement,
		layer: PlanMediaLayer,
		frame: number,
	): Texture | null {
		// Without autoLoad the source never learns the size of the video, and a
		// frame rectangle on a 1x1 source samples one texel: a flat color. The
		// element redraws when its metadata arrives (MediaPool).
		if (video.videoWidth === 0) return null;
		const crop = cropAt(layer, frame);
		if (crop === null) return null;
		const key = `video:${layer.asset}:${crop.join(",")}`;
		let texture = this.stills.get(key);
		if (texture === undefined) {
			const source = new VideoSource({
				resource: video,
				autoPlay: false,
				autoLoad: false,
			});
			source.resize(video.videoWidth, video.videoHeight);
			const [x, y, w, h] = crop;
			texture = new Texture({ source, frame: new Rectangle(x, y, w, h) });
			this.stills.set(key, texture);
		}
		texture.source.update();
		return texture;
	}

	// The thumbnail still of an asset that has no decoding element (beyond
	// MAX_VIDEO_LAYERS) or that is an image. The frame's crop is the source
	// region, the same rectangle a decoded video uses, scaled onto the poster.
	private still(
		projectId: string,
		layer: PlanMediaLayer,
		frame: number,
	): Texture | null {
		const crop = cropAt(layer, frame);
		if (crop === null) return null;
		const key = `still:${layer.asset}:${crop.join(",")}`;
		const cached = this.stills.get(key);
		if (cached !== undefined) return cached;
		const image = this.stillImages.get(layer.asset);
		if (image !== undefined) {
			const texture = this.textureOfStill(image, layer.asset, crop);
			this.stills.set(key, texture);
			return texture;
		}
		const loadKey = `still:${layer.asset}`;
		if (this.loading.has(loadKey)) return null;
		this.loading.add(loadKey);
		const loading = new Image();
		loading.onload = () => {
			this.loading.delete(loadKey);
			if (this.disposed) return;
			this.stillImages.set(layer.asset, loading);
			this.redraw();
		};
		loading.onerror = () => this.loading.delete(loadKey);
		loading.src = thumbnailUrl(projectId, layer.asset, STILL_POSTER_WIDTH);
		return null;
	}

	private textureOfStill(
		image: HTMLImageElement,
		asset: string,
		crop: [number, number, number, number],
	): Texture {
		const source = this.sources(asset);
		const rect = stillFrameRect(
			crop,
			image.naturalWidth,
			image.naturalHeight,
			source?.width ?? image.naturalWidth,
			source?.height ?? image.naturalHeight,
		);
		return new Texture({
			source: new ImageSource({ resource: image }),
			frame: new Rectangle(rect.x, rect.y, rect.width, rect.height),
		});
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
			// An item with a processed sound plays its file once there is one for
			// the playhead, and the proxy until then (SDD-004 section 7.1).
			const processed = this.processedAudio.resolve(audio, frame);
			const entry = this.sounds.acquire(
				audio.item,
				processed?.url ?? proxyUrl(info.projectId, audio.asset),
			);
			if (entry.gain !== null) {
				entry.gain.gain.value = valueAt(audio.gain, frame, audio.first);
			}
			const element = entry.element;
			const drive =
				processed === null
					? audioDrive(audio, frame, info.fps)
					: processedDrive(frame, processed.zero, info.fps);
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
		setProcessedAudioStates(this.processedAudio.states());
	}
}

function isDrawable(resource: unknown): resource is CanvasImageSource {
	return (
		(typeof HTMLImageElement !== "undefined" &&
			resource instanceof HTMLImageElement) ||
		(typeof HTMLVideoElement !== "undefined" &&
			resource instanceof HTMLVideoElement) ||
		(typeof HTMLCanvasElement !== "undefined" &&
			resource instanceof HTMLCanvasElement) ||
		(typeof ImageBitmap !== "undefined" && resource instanceof ImageBitmap) ||
		(typeof VideoFrame !== "undefined" && resource instanceof VideoFrame) ||
		(typeof OffscreenCanvas !== "undefined" &&
			resource instanceof OffscreenCanvas)
	);
}

// The silhouette is the alpha after the flip (SDD-004 section 3.15). The picture
// sprite mirrors on its own; this copy is the source the shadow is built from.
function mirrorPixels(
	src: Uint8Array,
	width: number,
	height: number,
	layer: PlanLayer,
): Uint8Array {
	const flipX = layer.flip_x === true;
	const flipY = layer.flip_y === true;
	if (!flipX && !flipY) return src;
	const out = new Uint8Array(src.length);
	for (let y = 0; y < height; y++) {
		const sy = flipY ? height - 1 - y : y;
		for (let x = 0; x < width; x++) {
			const sx = flipX ? width - 1 - x : x;
			const di = (y * width + x) * 4;
			const si = (sy * width + sx) * 4;
			out[di] = src[si] ?? 0;
			out[di + 1] = src[si + 1] ?? 0;
			out[di + 2] = src[si + 2] ?? 0;
			out[di + 3] = src[si + 3] ?? 0;
		}
	}
	return out;
}

function paintBox(painted: PlacedPixels | null): PaintBox | null {
	if (painted === null) return null;
	return {
		x: painted.x,
		y: painted.y,
		width: painted.width,
		height: painted.height,
	};
}

// The sprite rotates around its own centre, so that centre is the source offset
// rotated around the picture centre. Pixi's positive angle is clockwise.
function placePainted(
	sprite: Sprite,
	place: PaintBox,
	srcW: number,
	srcH: number,
	g: LayerGeometry,
): void {
	sprite.width = (g.w * place.width) / srcW;
	sprite.height = (g.h * place.height) / srcH;
	sprite.scale.x = Math.abs(sprite.scale.x);
	sprite.scale.y = Math.abs(sprite.scale.y);
	const lx = ((place.x + place.width / 2 - srcW / 2) * g.w) / srcW;
	const ly = ((place.y + place.height / 2 - srcH / 2) * g.h) / srcH;
	const turn = (g.rotation * Math.PI) / 180;
	const cos = Math.cos(turn);
	const sin = Math.sin(turn);
	sprite.position.set(g.cx + lx * cos - ly * sin, g.cy + lx * sin + ly * cos);
	sprite.angle = g.rotation;
}

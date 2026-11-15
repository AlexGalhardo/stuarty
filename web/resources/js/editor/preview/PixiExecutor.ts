import "pixi.js/unsafe-eval";
import {
	Application,
	ColorMatrixFilter,
	Container,
	ImageSource,
	Rectangle,
	Sprite,
	Texture,
	VideoSource,
} from "pixi.js";

import { fetchBitmap } from "../api/bitmaps";
import type { ApiClient } from "../api/client";
import type { PlanLayer, PlanMediaLayer, PlanWindow } from "../api/types";
import { proxyUrl, thumbnailUrl } from "../api/urls";
import { Lru } from "./lru";
import { MediaPool, type PoolTimers } from "./mediaPool";
import {
	MAX_VIDEO_LAYERS,
	needsCorrection,
	PlaybackClock,
	playbackRateFor,
	rankVideoLayers,
} from "./mediaSync";
import { bitmapKeyAt, geometryOf, layersAt, valueAt } from "./planValue";
import { PlanWindows } from "./planWindows";
import type {
	LayerGeometry,
	PreviewExecutor,
	PreviewState,
	ProjectInfo,
	Unsubscribe,
} from "./types";

const BITMAP_CACHE = 64;

interface Drawn {
	sprite: Sprite;
	filter: ColorMatrixFilter | null;
}

interface DrawMode {
	playing: boolean;
	scrubbing: boolean;
}

const browserTimers: PoolTimers = {
	setTimeout: (fn, ms) => setTimeout(fn, ms),
	clearTimeout: (handle) => clearTimeout(handle as number),
};

// PixiJS v8 on a WebGL2 canvas drawing exactly what a plan window says: the
// background, then each layer at its centre, size, rotation, opacity and colour
// matrix, in plan order, and nothing else (rule T6; SDD-012 section 11.2).
export class PixiExecutor implements PreviewExecutor {
	private app: Application | null = null;
	private readonly stage = new Container();
	private info: ProjectInfo | null = null;
	private revision = 0;
	private frame = 0;
	private state: PreviewState = "idle";
	private playing = false;
	private raf = 0;
	private generation = 0;
	private disposed = false;
	private context: AudioContext | null = null;
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
		this.videos = new MediaPool("video", () => this.context, timers, redraw);
		this.sounds = new MediaPool("audio", () => this.context, timers, redraw);
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
		try {
			const app = new Application();
			await app.init({
				canvas,
				width: this.info?.width ?? 1080,
				height: this.info?.height ?? 1920,
				preference: "webgl",
				antialias: false,
				autoStart: false,
				autoDensity: false,
				resolution: 1,
			});
			if (this.disposed) {
				app.destroy(true);
				return;
			}
			app.stage.addChild(this.stage);
			this.app = app;
			this.context = new AudioContext();
			this.redraw();
		} catch {
			// No WebGL2: the preview shows only paused frames (truth on pause).
			canvas.remove();
			this.setState("unsupported");
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
		this.app?.destroy({ removeView: true }, { children: true });
		this.app = null;
		void this.context?.close();
		this.context = null;
		this.frameListeners.clear();
		this.stateListeners.clear();
	}

	setProject(info: ProjectInfo): void {
		this.info = info;
		this.clock = new PlaybackClock(
			() => this.context?.currentTime ?? 0,
			info.fps,
		);
		this.windows = new PlanWindows((from, to, rev) =>
			this.client.getPlan(info.projectId, from, to, rev),
		);
		this.windows.setRevision(this.revision);
		this.redraw();
	}

	setRevision(revision: number): void {
		if (revision === this.revision) return;
		this.revision = revision;
		this.windows.setRevision(revision);
		this.redraw();
	}

	resize(bufferWidth: number, bufferHeight: number): void {
		if (this.app === null || this.info === null) return;
		this.app.renderer.resize(bufferWidth, bufferHeight);
		this.stage.scale.set(bufferWidth / this.info.width);
		this.redraw();
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
			if (texture === null) continue;
			used.add(layer.item);
			let entry = this.drawn.get(layer.item);
			if (entry === undefined) {
				entry = { sprite: new Sprite(), filter: null };
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
			entry.sprite.destroy();
			this.drawn.delete(item);
		}
		this.videos.idle(usedVideos);
		this.syncAudio(plan, frame, mode.playing);
		app.render();
	}

	private place(entry: Drawn, layer: PlanLayer, frame: number): void {
		const g = geometryOf(layer, frame);
		const { sprite } = entry;
		sprite.position.set(g.cx, g.cy);
		sprite.width = g.w;
		sprite.height = g.h;
		sprite.angle = g.rotation;
		sprite.alpha = valueAt(layer.opacity, frame, layer.first);
		const matrix = layer.kind === "media" ? layer.color_matrix : null;
		if (matrix === null) {
			if (entry.filter !== null) {
				sprite.filters = [];
				entry.filter = null;
			}
			return;
		}
		if (entry.filter === null) {
			entry.filter = new ColorMatrixFilter();
			sprite.filters = [entry.filter];
		}
		entry.filter.matrix = matrix as ColorMatrixFilter["matrix"];
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
			if (
				needsCorrection(element.currentTime, audio.source_seconds, info.fps)
			) {
				element.currentTime = audio.source_seconds;
			}
			if (element.paused) void element.play().catch(() => undefined);
		}
		this.sounds.idle(inUse);
	}
}

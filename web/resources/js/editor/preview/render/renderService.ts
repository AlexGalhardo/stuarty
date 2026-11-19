import type {
	FromRenderWorker,
	PixelSource,
	Rect,
	RenderBudgets,
	RenderInput,
	RenderReason,
	ToRenderWorker,
} from "./protocol";
import {
	type CoreExports,
	instantiateCore,
	RenderCore,
	RenderError,
} from "./renderCore";

// Pixels read out of a source in straight alpha, as the browser converts a
// frame (sRGB) or decodes a bitmap.
export interface Pixels {
	width: number;
	height: number;
	rgba: Uint8Array;
}

// What the worker needs from the browser, so the logic can be tested with the
// real module and without one.
export interface ServiceDeps<S extends PixelSource, B> {
	fetchModule(url: string): Promise<ArrayBuffer>;
	sha256(bytes: ArrayBuffer): Promise<string>;
	// The pixels of a frame or bitmap, cut to `rect` when given.
	read(source: S, rect: Rect | undefined): Promise<Pixels>;
	toBitmap(
		rgba: Uint8ClampedArray<ArrayBuffer>,
		width: number,
		height: number,
	): Promise<B>;
	// Compiles and instantiates the checked bytes; instantiateCore when absent.
	instantiate?(bytes: ArrayBuffer): Promise<CoreExports>;
}

type Budgets = RenderBudgets | undefined;

export interface ServiceHost<B> {
	post(message: FromRenderWorker<B>, transfer: Transferable[]): void;
}

// A render input holds the frame it asks for in its output size.
function outputSize(input: RenderInput): { width: number; height: number } {
	return { width: input.output.width, height: input.output.height };
}

// The logic of the render worker (SDD-013 section 4.7): messages are handled
// one after the other, in order, so an upload always precedes the render that
// needs it. Every transferred source is closed after its pixels are copied,
// on the error paths too (rule 3).
export class RenderService<S extends PixelSource, B> {
	private core: RenderCore | null = null;
	// The checked module and its budgets, to start a new instance after a trap.
	private loaded: { bytes: ArrayBuffer; budgets: Budgets } | null = null;
	private open = 0;
	private chain: Promise<void> = Promise.resolve();
	private disposed = false;

	constructor(
		private readonly deps: ServiceDeps<S, B>,
		private readonly host: ServiceHost<B>,
	) {}

	// Sources the worker holds and has not closed: the leak check reads it.
	get openSources(): number {
		return this.open;
	}

	// A message arrives; its sources are counted at once, so a source queued
	// behind a slow one is counted too.
	receive(message: ToRenderWorker<S>): Promise<void> {
		if (message.type === "texture") this.open++;
		if (message.type === "bitmap") this.open++;
		this.chain = this.chain.then(() => this.handle(message));
		return this.chain;
	}

	private async handle(message: ToRenderWorker<S>): Promise<void> {
		try {
			await this.dispatch(message);
		} catch (err) {
			const error =
				err instanceof RenderError
					? err
					: new RenderError(
							"render_trap",
							err instanceof Error ? err.message : String(err),
						);
			// After a trap the instance is discarded and a new one created; what it
			// held is gone and the main thread uploads again (SDD-013 section 4.5).
			if (error.reason === "render_trap") await this.restart();
			this.fail(
				message.type === "render" ? message.seq : undefined,
				error.reason,
				error.detail,
			);
		}
	}

	private async dispatch(message: ToRenderWorker<S>): Promise<void> {
		if (this.disposed) {
			if (message.type === "texture") this.closeSource(message.frame);
			if (message.type === "bitmap") this.closeSource(message.bitmap);
			return;
		}
		switch (message.type) {
			case "init":
				await this.init(message);
				return;
			case "texture":
				await this.texture(message.id, message.frame, message.rect);
				return;
			case "bitmap":
				await this.bitmap(message);
				return;
			case "lut":
				this.need().putLut(message.id, message.size, message.data);
				return;
			case "release":
				for (const id of message.ids) this.need().release(id);
				return;
			case "render":
				await this.render(message.seq, message.input);
				return;
			case "dispose":
				this.disposed = true;
				this.core = null;
				return;
		}
	}

	private need(): RenderCore {
		if (this.core === null) {
			throw new RenderError(
				"invalid_argument",
				"the module is not initialised",
			);
		}
		return this.core;
	}

	private async init(
		message: Extract<ToRenderWorker<S>, { type: "init" }>,
	): Promise<void> {
		let bytes: ArrayBuffer;
		try {
			bytes = await this.deps.fetchModule(message.module);
		} catch (err) {
			throw new RenderError(
				"module_load",
				err instanceof Error ? err.message : String(err),
			);
		}
		// The module is pinned by digest where the browser loads it (rule 9 of
		// .agents/rules/rust.md): a file that is not the one built is refused.
		const digest = await this.deps.sha256(bytes);
		if (digest !== message.digest) {
			throw new RenderError("module_digest", digest);
		}
		const core = await this.start(bytes, message.budgets);
		this.core = core;
		this.loaded = { bytes, budgets: message.budgets };
		this.host.post(
			{
				type: "ready",
				apiVersion: core.apiVersion,
				path: "cpu",
				open: this.open,
			},
			[],
		);
	}

	private async start(
		bytes: ArrayBuffer,
		budgets: Budgets,
	): Promise<RenderCore> {
		const instantiate = this.deps.instantiate ?? instantiateCore;
		const exports: CoreExports = await instantiate(bytes);
		const core = new RenderCore(exports);
		core.init(budgets?.textureBytes, budgets?.bitmapBytes);
		return core;
	}

	// A new instance in place of one that trapped. If that fails too, the worker
	// holds no module and refuses what follows as not initialised.
	private async restart(): Promise<void> {
		const loaded = this.loaded;
		this.core = null;
		if (loaded === null || this.disposed) return;
		try {
			this.core = await this.start(loaded.bytes, loaded.budgets);
		} catch {
			this.core = null;
		}
	}

	private closeSource(source: S): void {
		source.close();
		this.open--;
	}

	private async texture(
		id: string,
		frame: S,
		rect: Rect | undefined,
	): Promise<void> {
		try {
			const pixels = await this.deps.read(frame, rect);
			premultiply(pixels.rgba);
			this.need().putTexture(
				id,
				pixels.width,
				pixels.height,
				pixels.rgba,
				isOpaque(pixels.rgba),
			);
		} finally {
			this.closeSource(frame);
		}
	}

	private async bitmap(
		message: Extract<ToRenderWorker<S>, { type: "bitmap" }>,
	): Promise<void> {
		try {
			const pixels = await this.deps.read(message.bitmap, undefined);
			this.need().putBitmap(
				message.key,
				pixels.width,
				pixels.height,
				message.natural,
				message.center,
				pixels.rgba,
			);
		} finally {
			this.closeSource(message.bitmap);
		}
	}

	private async render(seq: number, input: RenderInput): Promise<void> {
		const { width, height } = outputSize(input);
		const rgba = this.need().render(JSON.stringify(input), width, height);
		const bitmap = await this.deps.toBitmap(rgba, width, height);
		this.host.post({ type: "frame", seq, bitmap, open: this.open }, [
			bitmap as unknown as Transferable,
		]);
	}

	private fail(
		seq: number | undefined,
		reason: RenderReason,
		detail: string,
	): void {
		this.host.post({ type: "error", seq, reason, detail, open: this.open }, []);
	}
}

// Every alpha is 255.
export function isOpaque(rgba: Uint8Array): boolean {
	for (let i = 3; i < rgba.length; i += 4) if (rgba[i] !== 255) return false;
	return true;
}

// Straight alpha to the premultiplied form of textures (SDD-013 section 4.2).
export function premultiply(rgba: Uint8Array): void {
	for (let i = 0; i < rgba.length; i += 4) {
		const a = rgba[i + 3] as number;
		if (a === 255) continue;
		rgba[i] = Math.trunc(((rgba[i] as number) * a + 127) / 255);
		rgba[i + 1] = Math.trunc(((rgba[i + 1] as number) * a + 127) / 255);
		rgba[i + 2] = Math.trunc(((rgba[i + 2] as number) * a + 127) / 255);
	}
}

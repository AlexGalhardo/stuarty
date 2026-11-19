import type { RenderReason } from "./protocol";

// The API version this host knows (SDD-013 section 4.2): a module that answers
// another number is refused before any other call.
export const API_VERSION = 3;

// The numbers of SDD-013 section 4.5, in order: status -1 is the first.
const REASONS: readonly RenderReason[] = [
	"invalid_plan",
	"unsupported_version",
	"unsupported_layer",
	"missing_texture",
	"limit_exceeded",
	"out_of_memory",
	"invalid_argument",
	"invalid_spec",
	"font_unknown",
	"font_invalid",
	"font_needed",
	"text_failed",
];

export class RenderError extends Error {
	constructor(
		readonly reason: RenderReason,
		readonly detail: string,
	) {
		super(`${reason}: ${detail}`);
		this.name = "RenderError";
	}
}

// What the core module exports (SDD-013 section 4.2), the part this host uses.
export interface CoreExports {
	memory: WebAssembly.Memory;
	vr_api_version(): number;
	vr_alloc(len: number): number;
	vr_free(ptr: number, len: number): void;
	vr_init(maxTextureBytes: number, maxBitmapBytes: number): number;
	vr_put_texture(
		idPtr: number,
		idLen: number,
		width: number,
		height: number,
		rgbaPtr: number,
		opaque: number,
	): number;
	vr_put_bitmap(
		keyPtr: number,
		keyLen: number,
		width: number,
		height: number,
		naturalW: number,
		naturalH: number,
		centerDx: number,
		centerDy: number,
		rgbaPtr: number,
	): number;
	vr_put_lut(
		idPtr: number,
		idLen: number,
		size: number,
		dataPtr: number,
		dataLen: number,
	): number;
	vr_release(idPtr: number, idLen: number): number;
	vr_render(
		inPtr: number,
		inLen: number,
		outPtr: number,
		outLen: number,
	): number;
	vr_error(bufPtr: number, bufLen: number): number;
}

export const DEFAULT_TEXTURE_BYTES = 256 << 20;
export const DEFAULT_BITMAP_BYTES = 128 << 20;
const MAX_ID = 64;

const encoder = new TextEncoder();
const decoder = new TextDecoder();

// Instantiates the core module: it has no import (SDD-013 section 4.1), so no
// import object is given, and a module that asks for one does not load.
export async function instantiateCore(
	bytes: BufferSource,
): Promise<CoreExports> {
	let instance: WebAssembly.Instance;
	try {
		instance = (await WebAssembly.instantiate(bytes, {})).instance;
	} catch (err) {
		throw new RenderError(
			"module_load",
			err instanceof Error ? err.message : String(err),
		);
	}
	return instance.exports as unknown as CoreExports;
}

// A typed view over the module: every buffer is allocated in its memory for
// the length of one call and freed after, as SDD-013 section 4.3 lays out.
// The memory may move when a call allocates, so a view is read again after
// each call (section 4.4 rule 3).
export class RenderCore {
	private out: { ptr: number; len: number } | null = null;

	constructor(private readonly core: CoreExports) {
		const version = core.vr_api_version();
		if (version !== API_VERSION) {
			throw new RenderError(
				"unsupported_version",
				`the module is API ${version}, this host knows ${API_VERSION}`,
			);
		}
	}

	get apiVersion(): number {
		return this.core.vr_api_version();
	}

	init(
		textureBytes: number = DEFAULT_TEXTURE_BYTES,
		bitmapBytes: number = DEFAULT_BITMAP_BYTES,
	): void {
		this.dropOut();
		this.check(this.core.vr_init(textureBytes, bitmapBytes));
	}

	// `rgba` is premultiplied, width x height x 4 bytes; `opaque` when every
	// alpha is 255.
	putTexture(
		id: string,
		width: number,
		height: number,
		rgba: Uint8Array,
		opaque: boolean,
	): void {
		this.with(id, rgba, (idPtr, idLen, pixelsPtr) =>
			this.core.vr_put_texture(
				idPtr,
				idLen,
				width,
				height,
				pixelsPtr,
				opaque ? 1 : 0,
			),
		);
	}

	// `rgba` is straight alpha; the module premultiplies it once.
	putBitmap(
		key: string,
		width: number,
		height: number,
		natural: [number, number],
		center: [number, number],
		rgba: Uint8Array,
	): void {
		this.with(key, rgba, (keyPtr, keyLen, pixelsPtr) =>
			this.core.vr_put_bitmap(
				keyPtr,
				keyLen,
				width,
				height,
				natural[0],
				natural[1],
				center[0],
				center[1],
				pixelsPtr,
			),
		);
	}

	putLut(id: string, size: number, data: Uint8Array): void {
		this.with(id, data, (idPtr, idLen, dataPtr) =>
			this.core.vr_put_lut(idPtr, idLen, size, dataPtr, data.length),
		);
	}

	release(id: string): void {
		this.with(id, new Uint8Array(0), (idPtr, idLen) =>
			this.core.vr_release(idPtr, idLen),
		);
	}

	// Draws the render input and returns width x height x 4 bytes, a copy that
	// stays valid when the memory grows.
	render(
		input: string,
		width: number,
		height: number,
	): Uint8ClampedArray<ArrayBuffer> {
		const outLen = width * height * 4;
		const bytes = encoder.encode(input);
		const inPtr = this.alloc(bytes.length);
		try {
			this.view(inPtr, bytes.length).set(bytes);
			const outPtr = this.outBlock(outLen);
			this.check(
				this.guard(() =>
					this.core.vr_render(inPtr, bytes.length, outPtr, outLen),
				),
			);
			const copy = new Uint8ClampedArray(outLen);
			copy.set(this.view(outPtr, outLen));
			return copy;
		} finally {
			this.core.vr_free(inPtr, bytes.length);
		}
	}

	private outBlock(len: number): number {
		if (this.out !== null && this.out.len === len) return this.out.ptr;
		this.dropOut();
		this.out = { ptr: this.alloc(len), len };
		return this.out.ptr;
	}

	private dropOut(): void {
		if (this.out === null) return;
		this.core.vr_free(this.out.ptr, this.out.len);
		this.out = null;
	}

	private alloc(len: number): number {
		const ptr = this.guard(() => this.core.vr_alloc(len));
		if (ptr === 0) throw new RenderError("out_of_memory", "vr_alloc");
		return ptr;
	}

	private view(ptr: number, len: number): Uint8Array {
		return new Uint8Array(this.core.memory.buffer, ptr, len);
	}

	// An id (at most 64 ASCII bytes) and a block of bytes, both in the module
	// for the length of the call.
	private with(
		id: string,
		data: Uint8Array,
		call: (idPtr: number, idLen: number, dataPtr: number) => number,
	): void {
		const name = encoder.encode(id);
		if (name.length === 0 || name.length > MAX_ID) {
			throw new RenderError("invalid_argument", `id of ${name.length} bytes`);
		}
		const idPtr = this.alloc(name.length);
		let dataPtr = 0;
		try {
			dataPtr = this.alloc(data.length);
			this.view(idPtr, name.length).set(name);
			this.view(dataPtr, data.length).set(data);
			this.check(this.guard(() => call(idPtr, name.length, dataPtr)));
		} finally {
			this.core.vr_free(idPtr, name.length);
			if (dataPtr !== 0) this.core.vr_free(dataPtr, data.length);
		}
	}

	private guard<T>(call: () => T): T {
		try {
			return call();
		} catch (err) {
			if (err instanceof RenderError) throw err;
			throw new RenderError(
				"render_trap",
				err instanceof Error ? err.message : String(err),
			);
		}
	}

	private check(status: number): void {
		if (status === 0) return;
		const reason = REASONS[-status - 1] ?? "invalid_argument";
		throw new RenderError(reason, this.lastError());
	}

	private lastError(): string {
		const length = this.core.vr_error(0, 0);
		if (length === 0) return "";
		const ptr = this.alloc(length);
		try {
			this.core.vr_error(ptr, length);
			return decoder.decode(this.view(ptr, length).slice());
		} finally {
			this.core.vr_free(ptr, length);
		}
	}
}

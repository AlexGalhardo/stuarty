// The messages between the main thread and the render worker (SDD-013 section
// 4.7). The reasons are those of SDD-013 section 7.1.

export type RenderReason =
	| "invalid_plan"
	| "unsupported_version"
	| "unsupported_layer"
	| "missing_texture"
	| "limit_exceeded"
	| "out_of_memory"
	| "invalid_argument"
	| "invalid_spec"
	| "font_unknown"
	| "font_invalid"
	| "font_needed"
	| "text_failed"
	| "render_trap"
	| "device_lost"
	| "module_digest"
	| "module_load";

// What a texture is made from: a decoded frame or a bitmap. Both are closed by
// the worker once their pixels are copied (SDD-013 section 4.7 rule 3).
export interface PixelSource {
	readonly displayWidth?: number;
	readonly width?: number;
	readonly height?: number;
	close(): void;
}

export interface Rect {
	x: number;
	y: number;
	w: number;
	h: number;
}

export interface RenderBudgets {
	textureBytes: number;
	bitmapBytes: number;
}

export type ToRenderWorker<S = PixelSource> =
	| {
			type: "init";
			module: string;
			// The lower-case hex SHA-256 the module must have (renderer.wasm.sha256).
			digest: string;
			prefer: "gpu" | "cpu";
			budgets?: RenderBudgets;
	  }
	| {
			type: "texture";
			id: string;
			frame: S;
			// The part of the frame to keep, in its pixels; absent is all of it.
			rect?: Rect;
			// The size to deliver that part at: the drawn size, in output pixels, of a
			// layer that is not rotated, as the Go host delivers it (SDD-013 section
			// 5.1 item 4). Absent is the size of the part.
			size?: [number, number];
	  }
	| {
			type: "bitmap";
			key: string;
			bitmap: S;
			natural: [number, number];
			center: [number, number];
	  }
	| { type: "lut"; id: string; size: number; data: Uint8Array }
	| { type: "release"; ids: string[] }
	| {
			type: "render";
			seq: number;
			// The render input of SDD-013 section 2.1.
			input: RenderInput;
			// The groups the frame holds, drawn first (SDD-013 section 3.12): the
			// inner ones before the ones that hold them.
			surfaces?: RenderSurface[];
	  }
	| { type: "dispose" };

export interface RenderInput {
	version: 1;
	output: { width: number; height: number; scale: number };
	plan: unknown;
}

// The nested frame of a group, drawn by the module on a surface of its own and
// stored as the texture `id`; the frame then draws that texture as the media
// layer of the group (SDD-013 section 3.12). The surface is the output size
// of `input`.
export interface RenderSurface {
	id: string;
	input: RenderInput;
}

export type FromRenderWorker<B = ImageBitmap> =
	| { type: "ready"; apiVersion: number; path: "cpu" | "gpu"; open: number }
	| { type: "frame"; seq: number; bitmap: B; open: number }
	| {
			type: "error";
			// Absent for init and for an upload.
			seq?: number;
			reason: RenderReason;
			detail: string;
			open: number;
	  };

import type { DecodeMode } from "./decodeCore";

// Main thread to worker (SDD-013 section 5.3).
export type ToWorker =
	| { type: "open"; asset: string; indexUrl: string; proxyUrl: string }
	| {
			type: "frame";
			id: number;
			asset: string;
			// The source time the plan asks for, in microseconds.
			micros: number;
			mode: DecodeMode;
	  }
	| { type: "drop"; asset: string };

// Worker to main thread. `open` is the number of frames the worker still holds,
// sent with every message so `stats.openFrames` adds both sides.
export type FromWorker<F = VideoFrame> =
	| { type: "opened"; asset: string; open: number }
	| { type: "fallback"; asset: string; reason: string; open: number }
	| { type: "skipped"; id: number; asset: string; open: number }
	| {
			type: "frame";
			id: number;
			asset: string;
			sample: number;
			timestamp: number;
			lo: number;
			hi: number;
			frame: F;
			open: number;
	  };

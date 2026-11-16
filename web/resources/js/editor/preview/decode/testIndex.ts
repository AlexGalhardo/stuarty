import type { ChunkIndex } from "./chunkIndex";

// 30 fps, a key frame every 15 samples, 40 samples.
export function makeIndex(n = 40, gop = 15, fps = 30): ChunkIndex {
	const offsets: number[] = [];
	const sizes: number[] = [];
	const timestamps: number[] = [];
	let at = 48;
	for (let i = 0; i < n; i++) {
		offsets.push(at);
		sizes.push(100 + i);
		at += 100 + i;
		timestamps.push(Math.round((i * 1_000_000) / fps));
	}
	const keyframes: number[] = [];
	for (let i = 0; i < n; i += gop) keyframes.push(i);
	return {
		version: 1,
		codec: "vp09.00.10.08.01.01.01.01.00",
		coded_width: 320,
		coded_height: 568,
		duration_us: Math.round((n * 1_000_000) / fps),
		offsets,
		sizes,
		timestamps_us: timestamps,
		keyframes,
	};
}

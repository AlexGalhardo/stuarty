// The proxy chunk index of SDD-013 section 6: what the WebCodecs worker needs
// to read the video of a proxy without a demuxer. The checks are hand written,
// not a schema library (SDD-013 section 6.3 item 2).
export interface ChunkIndex {
	version: 1;
	codec: string;
	coded_width: number;
	coded_height: number;
	duration_us: number;
	offsets: number[];
	sizes: number[];
	timestamps_us: number[];
	keyframes: number[];
}

export type IndexCheck =
	| { ok: true; index: ChunkIndex }
	| { ok: false; reason: string };

const CODEC = /^vp09(\.[0-9]{2}){8}$/;
const MAX_CODED = 4096;

const isInt = (v: unknown, min: number): v is number =>
	typeof v === "number" && Number.isInteger(v) && v >= min;

function intArray(v: unknown, min: number): v is number[] {
	return Array.isArray(v) && v.every((x) => isInt(x, min));
}

const KEYS = [
	"version",
	"codec",
	"coded_width",
	"coded_height",
	"duration_us",
	"offsets",
	"sizes",
	"timestamps_us",
	"keyframes",
];

// Validates the shape of section 6.1 and its invariants. An index that fails
// is not used: the asset is decoded by `<video>`.
export function parseChunkIndex(raw: unknown): IndexCheck {
	if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
		return { ok: false, reason: "not an object" };
	}
	const o = raw as Record<string, unknown>;
	if (o.version !== 1) return { ok: false, reason: "unknown version" };
	for (const key of Object.keys(o)) {
		if (!KEYS.includes(key)) return { ok: false, reason: `extra field ${key}` };
	}
	if (typeof o.codec !== "string" || !CODEC.test(o.codec)) {
		return { ok: false, reason: "codec is not VP9" };
	}
	if (!isInt(o.coded_width, 1) || o.coded_width > MAX_CODED) {
		return { ok: false, reason: "bad coded_width" };
	}
	if (!isInt(o.coded_height, 1) || o.coded_height > MAX_CODED) {
		return { ok: false, reason: "bad coded_height" };
	}
	if (!isInt(o.duration_us, 0)) return { ok: false, reason: "bad duration_us" };
	if (!intArray(o.offsets, 1)) return { ok: false, reason: "bad offsets" };
	if (!intArray(o.sizes, 1)) return { ok: false, reason: "bad sizes" };
	if (!intArray(o.timestamps_us, 0)) {
		return { ok: false, reason: "bad timestamps_us" };
	}
	if (!intArray(o.keyframes, 0)) return { ok: false, reason: "bad keyframes" };
	const n = o.offsets.length;
	if (n === 0) return { ok: false, reason: "no samples" };
	if (o.sizes.length !== n || o.timestamps_us.length !== n) {
		return { ok: false, reason: "array lengths differ" };
	}
	const key = o.keyframes;
	if (key.length === 0 || key[0] !== 0) {
		return { ok: false, reason: "the first sample is not a key frame" };
	}
	for (let i = 0; i < key.length; i++) {
		const k = key[i] as number;
		if (k >= n || (i > 0 && k <= (key[i - 1] as number))) {
			return { ok: false, reason: "keyframes are not increasing in range" };
		}
	}
	const ts = o.timestamps_us;
	for (let i = 1; i < n; i++) {
		if ((ts[i] as number) < (ts[i - 1] as number)) {
			return { ok: false, reason: "timestamps_us decrease" };
		}
	}
	return {
		ok: true,
		index: {
			version: 1,
			codec: o.codec,
			coded_width: o.coded_width,
			coded_height: o.coded_height,
			duration_us: o.duration_us,
			offsets: o.offsets,
			sizes: o.sizes,
			timestamps_us: ts,
			keyframes: key,
		},
	};
}

// The end of the last byte any sample reads, to compare with the size of the
// proxy the server answers (SDD-013 section 6.3 item 2).
export function indexEnd(index: ChunkIndex): number {
	let end = 0;
	for (let i = 0; i < index.offsets.length; i++) {
		end = Math.max(
			end,
			(index.offsets[i] as number) + (index.sizes[i] as number),
		);
	}
	return end;
}

// Half a sample duration in microseconds (0.5 frame of the asset's rate), the
// mean spacing of the timestamps.
function halfSample(index: ChunkIndex): number {
	const n = index.timestamps_us.length;
	if (n < 2) return 0;
	const span =
		(index.timestamps_us[n - 1] as number) - (index.timestamps_us[0] as number);
	return span / (n - 1) / 2;
}

// The largest `i` with timestamps_us[i] <= target, in a sorted array; -1 when
// every timestamp is above it.
function lastAtOrBefore(values: readonly number[], target: number): number {
	let lo = 0;
	let hi = values.length - 1;
	let found = -1;
	while (lo <= hi) {
		const mid = (lo + hi) >> 1;
		if ((values[mid] as number) <= target) {
			found = mid;
			lo = mid + 1;
		} else {
			hi = mid - 1;
		}
	}
	return found;
}

// The sample a plan frame shows: the largest timestamp not above the source
// time plus half a sample (SDD-013 section 5.3 item 2).
export function sampleAt(index: ChunkIndex, sourceSeconds: number): number {
	const target = Math.round(sourceSeconds * 1_000_000) + halfSample(index);
	return Math.max(0, lastAtOrBefore(index.timestamps_us, target));
}

// The source times (in microseconds) for which `sample` is the sample shown:
// [lo, hi). Open at both ends of the media, so a time past the end keeps the
// last frame.
export function sampleWindow(
	index: ChunkIndex,
	sample: number,
): { lo: number; hi: number } {
	const n = index.timestamps_us.length;
	const half = halfSample(index);
	const lo =
		sample === 0 ? -Infinity : (index.timestamps_us[sample] as number) - half;
	const hi =
		sample >= n - 1
			? Infinity
			: (index.timestamps_us[sample + 1] as number) - half;
	return { lo, hi };
}

// The last key frame at or before `sample`.
export function keyframeBefore(index: ChunkIndex, sample: number): number {
	return index.keyframes[lastAtOrBefore(index.keyframes, sample)] ?? 0;
}

// The span of bytes that holds samples `from` to `to`, `end` exclusive. The
// samples need not be in order in the file, so it is the min and the max.
export function byteSpan(
	index: ChunkIndex,
	from: number,
	to: number,
): { start: number; end: number } {
	let start = Infinity;
	let end = 0;
	for (let i = from; i <= to; i++) {
		const o = index.offsets[i] as number;
		start = Math.min(start, o);
		end = Math.max(end, o + (index.sizes[i] as number));
	}
	return { start, end };
}

// The waveform of an audio item (SDD-012 sections 7.4 and 1.3): the peaks file
// of the asset (SDD-011 section 4.6) read once, cached by asset, and turned
// into one min and max per pixel column for the visible part of the item. A
// decoration: it reads `source_in`, `speed`, `start` and `duration`, is never
// shown as a time and is never sent to the core.
import { fetchPeaks, PeaksPendingError } from "../../api/peaks";
import { Lru } from "../../preview/lru";

export interface PeakLevel {
	samplesPerPeak: number;
	peakCount: number;
	// Offset of the first peak in the file, in bytes.
	offset: number;
}

export interface Peaks {
	sampleRate: number;
	sampleCount: number;
	levels: readonly PeakLevel[];
	view: DataView;
}

const MAGIC = 0x4b50_4b56; // "VKPK", little-endian
const HEADER = 24;
const LEVEL_ENTRY = 16;

// null for a file that is not a peaks file of version 1: the item then draws
// no waveform, which is all a decoration may do when it cannot read its input.
export function parsePeaks(buffer: ArrayBuffer): Peaks | null {
	if (buffer.byteLength < HEADER) return null;
	const view = new DataView(buffer);
	if (view.getUint32(0, true) !== MAGIC || view.getUint16(4, true) !== 1) {
		return null;
	}
	const count = view.getUint16(6, true);
	if (buffer.byteLength < HEADER + count * LEVEL_ENTRY) return null;
	const levels: PeakLevel[] = [];
	for (let i = 0; i < count; i++) {
		const at = HEADER + i * LEVEL_ENTRY;
		const level = {
			samplesPerPeak: view.getUint32(at, true),
			peakCount: view.getUint32(at + 4, true),
			offset: Number(view.getBigUint64(at + 8, true)),
		};
		if (
			level.samplesPerPeak < 1 ||
			level.offset + level.peakCount * 4 > buffer.byteLength
		) {
			return null;
		}
		levels.push(level);
	}
	return {
		sampleRate: view.getUint32(8, true),
		sampleCount: Number(view.getBigUint64(16, true)),
		levels,
		view,
	};
}

export interface WaveformParams {
	// Project frames of `source_in`, the speed as num/den, project rate.
	sourceIn: number;
	speed: { num: number; den: number };
	fps: { num: number; den: number };
	pxPerFrame: number;
	// The columns wanted, counted from the left edge of the item.
	firstColumn: number;
	columns: number;
}

// One min and one max per column, each in [-1, 1]; a column outside the file
// is 0 and 0. Level choice: the coarsest level whose peak spans at most one
// column, so a peak is never wider than a pixel.
export function columnPeaks(peaks: Peaks, p: WaveformParams): Float32Array {
	const out = new Float32Array(p.columns * 2);
	if (p.pxPerFrame <= 0 || peaks.levels.length === 0) return out;
	const samplesPerFrame = (peaks.sampleRate * p.fps.den) / p.fps.num;
	const speed = p.speed.num / p.speed.den;
	const samplesPerColumn = (samplesPerFrame * speed) / p.pxPerFrame;
	let level = peaks.levels[0] as PeakLevel;
	for (const candidate of peaks.levels) {
		if (candidate.samplesPerPeak <= samplesPerColumn) level = candidate;
	}
	const at = (column: number): number =>
		(p.sourceIn + (column / p.pxPerFrame) * speed) * samplesPerFrame;
	for (let c = 0; c < p.columns; c++) {
		const from = Math.max(0, at(p.firstColumn + c));
		const to = Math.max(from, at(p.firstColumn + c + 1));
		if (from >= peaks.sampleCount) continue;
		const first = Math.floor(from / level.samplesPerPeak);
		const last = Math.min(
			level.peakCount - 1,
			Math.max(first, Math.ceil(to / level.samplesPerPeak) - 1),
		);
		let lo = 0;
		let hi = 0;
		for (let i = first; i <= last; i++) {
			const base = level.offset + i * 4;
			lo = Math.min(lo, peaks.view.getInt16(base, true));
			hi = Math.max(hi, peaks.view.getInt16(base + 2, true));
		}
		out[c * 2] = lo / 32768;
		out[c * 2 + 1] = hi / 32768;
	}
	return out;
}

// Per asset, by LRU of 50 assets (SDD-012 section 19.2 item 6). The promise is
// cached, so every item of one asset shares a single request; a failure leaves
// the cache so the next draw may try again.
const cache = new Lru<string, Promise<Peaks | null>>(50);

// The assets whose last read was answered "being computed": the page asks again for them.
const pendingKeys = new Set<string>();

export function isPeaksPending(projectId: string, assetId: string): boolean {
	return pendingKeys.has(`${projectId}/${assetId}`);
}

export function loadPeaks(
	projectId: string,
	assetId: string,
): Promise<Peaks | null> {
	const key = `${projectId}/${assetId}`;
	const cached = cache.get(key);
	if (cached !== undefined) return cached;
	const pending = fetchPeaks(projectId, assetId)
		.then((raw) => {
			pendingKeys.delete(key);
			return parsePeaks(raw);
		})
		.catch((error: unknown) => {
			cache.delete(key);
			if (error instanceof PeaksPendingError) pendingKeys.add(key);
			else pendingKeys.delete(key);
			return null;
		});
	cache.set(key, pending);
	return pending;
}

// Reads the peaks and, while the server says they are being computed, reads them again every retryMs. onResult gets
// the peaks, or null for a final failure, once. The function it returns stops asking.
export function watchPeaks(
	projectId: string,
	assetId: string,
	retryMs: number,
	onResult: (peaks: Peaks | null) => void,
): () => void {
	let live = true;
	let timer: ReturnType<typeof setTimeout> | undefined;
	const load = (): void => {
		void loadPeaks(projectId, assetId).then((peaks) => {
			if (!live) return;
			if (peaks === null && isPeaksPending(projectId, assetId)) {
				timer = setTimeout(load, retryMs);
				return;
			}
			onResult(peaks);
		});
	};
	load();
	return () => {
		live = false;
		clearTimeout(timer);
	};
}

export function clearPeaksCache(): void {
	cache.clear();
	pendingKeys.clear();
}

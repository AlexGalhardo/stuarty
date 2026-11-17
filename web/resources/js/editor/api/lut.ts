import { lutUrl } from "./urls";

// The lookup table the server parsed (SDD-004 section 3.6): the magic `VLUT`,
// the version (1), the size N (u16 LE), then N^3 entries of red, green and blue
// as u16 LE, the red index varying fastest. The browser parses no .cube text.
export interface LutTable {
	size: number;
	// 3 * N^3 values in 0..65535, entry (r, g, b) at 3 * (r + N * (g + N * b)).
	entries: Uint16Array;
}

const HEADER = 7;
const MIN_SIZE = 2;
const MAX_SIZE = 65;

// Returns null for anything but a well formed table; it never throws.
export function parseLut(bytes: ArrayBuffer): LutTable | null {
	if (bytes.byteLength < HEADER) return null;
	const view = new DataView(bytes);
	if (
		view.getUint8(0) !== 0x56 ||
		view.getUint8(1) !== 0x4c ||
		view.getUint8(2) !== 0x55 ||
		view.getUint8(3) !== 0x54 ||
		view.getUint8(4) !== 1
	) {
		return null;
	}
	const size = view.getUint16(5, true);
	if (size < MIN_SIZE || size > MAX_SIZE) return null;
	const count = 3 * size ** 3;
	if (bytes.byteLength !== HEADER + 2 * count) return null;
	const entries = new Uint16Array(count);
	for (let i = 0; i < count; i++) {
		entries[i] = view.getUint16(HEADER + 2 * i, true);
	}
	return { size, entries };
}

// Fetches and validates the table of a lut asset; null when it cannot be had.
export async function fetchLut(
	projectId: string,
	assetId: string,
	signal?: AbortSignal,
): Promise<LutTable | null> {
	try {
		const response = await fetch(lutUrl(projectId, assetId), {
			credentials: "same-origin",
			signal,
		});
		if (!response.ok) return null;
		return parseLut(await response.arrayBuffer());
	} catch {
		return null;
	}
}

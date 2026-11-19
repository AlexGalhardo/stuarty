// The units of a numeric field of the inspector (SDD-012 section 24.6): a whole number of pixels, written bare or
// with "px", or a percent of the canvas written with "%". The field resolves the unit to the integer it sends;
// the layer rules (limits, locks, the crop inside the source) are the server's.

export interface Parsed {
	value: number;
}

// `axis` is the size of the canvas along the axis the field measures, for a percent; 0 when a percent has no
// meaning there (an opacity, a rotation), and then it is refused.
export function parseLength(text: string, axis: number): Parsed | null {
	const match = /^\s*(-?\d+(?:[.,]\d+)?)\s*(px|%)?\s*$/i.exec(text);
	if (match === null) return null;
	const n = Number((match[1] ?? "").replace(",", "."));
	if (!Number.isFinite(n)) return null;
	if (match[2] === "%") {
		if (axis <= 0) return null;
		return { value: Math.round((n / 100) * axis) };
	}
	return { value: Math.round(n) };
}

// A random layer id for a command whose later operations name the layer it makes (SDD-012 Appendix A item 3):
// "lyr_" and ten characters of the alphabet of SDD-000 section 2.
const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

export function newLayerId(): string {
	const bytes = new Uint8Array(10);
	crypto.getRandomValues(bytes);
	let id = "lyr_";
	for (const b of bytes) id += ALPHABET[b % ALPHABET.length];
	return id;
}

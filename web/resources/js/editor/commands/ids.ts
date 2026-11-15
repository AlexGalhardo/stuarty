// Ids the UI supplies when one batch refers to an object it creates (a track
// added and a clip placed on it, SDD-012 section 8.2): prefix, underscore and
// ten characters of the Crockford-like alphabet of SDD-000.
const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

export function newId(prefix: "trk" | "itm" | "ast"): string {
	const bytes = crypto.getRandomValues(new Uint8Array(10));
	let tail = "";
	for (const byte of bytes) tail += ALPHABET[byte % 32];
	return `${prefix}_${tail}`;
}

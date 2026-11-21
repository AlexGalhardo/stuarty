import { inflateRawSync } from "node:zlib";

// Reads one entry of a zip from its bytes, through the central directory (the
// sizes there are exact even when the local header defers them to a data
// descriptor). Enough for the manifest of a .vvbundle; not a general reader.
export function zipEntry(zip: Buffer, name: string): Buffer {
	let end = zip.length - 22;
	while (end >= 0 && zip.readUInt32LE(end) !== 0x06054b50) end--;
	if (end < 0) throw new Error("not a zip: no end of central directory");
	const count = zip.readUInt16LE(end + 10);
	let at = zip.readUInt32LE(end + 16);
	for (let i = 0; i < count; i++) {
		if (zip.readUInt32LE(at) !== 0x02014b50) throw new Error("bad directory");
		const method = zip.readUInt16LE(at + 10);
		const size = zip.readUInt32LE(at + 20);
		const nameLength = zip.readUInt16LE(at + 28);
		const extraLength = zip.readUInt16LE(at + 30);
		const commentLength = zip.readUInt16LE(at + 32);
		const local = zip.readUInt32LE(at + 42);
		const entry = zip.toString("utf8", at + 46, at + 46 + nameLength);
		if (entry === name) {
			const start =
				local +
				30 +
				zip.readUInt16LE(local + 26) +
				zip.readUInt16LE(local + 28);
			const data = zip.subarray(start, start + size);
			if (method === 0) return data;
			if (method === 8) return inflateRawSync(data);
			throw new Error(`entry ${name} uses method ${method}`);
		}
		at += 46 + nameLength + extraLength + commentLength;
	}
	throw new Error(`no entry ${name} in the zip`);
}

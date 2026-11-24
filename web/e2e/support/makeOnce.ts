import { randomBytes } from "node:crypto";
import {
	closeSync,
	existsSync,
	fstatSync,
	openSync,
	readSync,
	renameSync,
	rmSync,
} from "node:fs";

const PNG_SIGNATURE = Buffer.from([
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
]);
// The last chunk of every PNG: length 0, "IEND", and its CRC.
const PNG_END = Buffer.from([
	0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
]);

// isCompletePng is a cheap check that a file starts with the PNG signature and ends with the IEND chunk, so a
// picture a killed run left half written is not trusted (VV-405).
export function isCompletePng(path: string): boolean {
	let fd: number | undefined;
	try {
		fd = openSync(path, "r");
		const size = fstatSync(fd).size;
		if (size < PNG_SIGNATURE.length + PNG_END.length) return false;
		const head = Buffer.alloc(PNG_SIGNATURE.length);
		const tail = Buffer.alloc(PNG_END.length);
		readSync(fd, head, 0, head.length, 0);
		readSync(fd, tail, 0, tail.length, size - tail.length);
		return head.equals(PNG_SIGNATURE) && tail.equals(PNG_END);
	} catch {
		return false;
	} finally {
		if (fd !== undefined) closeSync(fd);
	}
}

// makeOnce makes a media file at `out` unless a complete one is there. Several Playwright workers ask for the
// same file at the same time (VV-332): each writes its own temporary file next to `out` and renames it, so
// `out` is never seen half written, and the worker that loses the rename drops its copy. `complete` tells a
// finished file from the rest of a killed run: a file it refuses is made again (VV-405).
export function makeOnce(
	out: string,
	make: (target: string) => void,
	complete: (path: string) => boolean = () => true,
): void {
	if (existsSync(out) && complete(out)) return;
	const dot = out.lastIndexOf(".");
	const target = `${out.slice(0, dot)}.tmp-${process.pid}-${randomBytes(4).toString("hex")}${out.slice(dot)}`;
	try {
		make(target);
		try {
			renameSync(target, out);
		} catch (err) {
			// Another worker renamed first and holds the file open: its copy is as good as ours.
			if (!existsSync(out) || !complete(out)) throw err;
		}
	} finally {
		rmSync(target, { force: true });
	}
}

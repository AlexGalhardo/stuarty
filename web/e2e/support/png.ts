import { crc32, deflateSync } from "node:zlib";
import type { Pixels } from "./ssim";

// A PNG encoder for the pictures a failing comparison attaches: 8-bit RGBA, no
// filter, one IDAT. The tests need no library for it.
function chunk(type: string, data: Uint8Array): Buffer {
	const out = Buffer.alloc(12 + data.length);
	out.writeUInt32BE(data.length, 0);
	out.write(type, 4, "latin1");
	out.set(data, 8);
	out.writeUInt32BE(crc32(out.subarray(4, 8 + data.length)), 8 + data.length);
	return out;
}

export function encodePng({ width, height, data }: Pixels): Buffer {
	const head = Buffer.alloc(13);
	head.writeUInt32BE(width, 0);
	head.writeUInt32BE(height, 4);
	head[8] = 8; // bit depth
	head[9] = 6; // RGBA
	const stride = width * 4;
	const raw = Buffer.alloc((stride + 1) * height);
	for (let row = 0; row < height; row++) {
		raw[row * (stride + 1)] = 0;
		raw.set(
			data.subarray(row * stride, (row + 1) * stride),
			row * (stride + 1) + 1,
		);
	}
	return Buffer.concat([
		Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
		chunk("IHDR", head),
		chunk("IDAT", deflateSync(raw)),
		chunk("IEND", new Uint8Array(0)),
	]);
}

// The absolute difference of two pictures, times `gain` and opaque: what a
// reviewer looks at to see where the preview and the export part.
export function differencePicture(a: Pixels, b: Pixels, gain = 4): Pixels {
	const data = new Uint8Array(a.width * a.height * 4);
	for (let i = 0; i < data.length; i += 4) {
		for (let c = 0; c < 3; c++) {
			data[i + c] = Math.min(
				255,
				Math.abs((a.data[i + c] ?? 0) - (b.data[i + c] ?? 0)) * gain,
			);
		}
		data[i + 3] = 255;
	}
	return { width: a.width, height: a.height, data };
}

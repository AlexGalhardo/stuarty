import { describe, expect, test, vi } from "vitest";

import type { ApiClient } from "../api/client";
import { ApiError } from "../api/errors";
import type { Recording, RecordingUploadChunkResult } from "../api/types";
import {
	ChunkUploader,
	RETRY_WAITS_MS,
	type UploaderState,
} from "./chunkUploader";

const blob = (n: number): Blob => new Blob([new Uint8Array(n)]);

const ok = (index: number): RecordingUploadChunkResult => ({
	recording_id: "rec_0123456789",
	track: "screen",
	index,
	next_index: index + 1,
	duplicate: false,
	bytes: 0,
	text: "",
});

function api(
	code: string,
	details?: Record<string, string | number>,
): ApiError {
	return new ApiError(code === "E_NETWORK" ? 0 : 409, {
		code,
		message: code,
		...(details === undefined ? {} : { details }),
	});
}

interface Harness {
	uploader: ChunkUploader;
	upload: ReturnType<typeof vi.fn>;
	get: ReturnType<typeof vi.fn>;
	sleeps: number[];
	states: UploaderState[];
}

function make(
	upload: (...args: unknown[]) => Promise<RecordingUploadChunkResult>,
	get: () => Promise<Partial<Recording>> = async () => ({}),
): Harness {
	const uploadFn = vi.fn(upload);
	const getFn = vi.fn(get);
	const sleeps: number[] = [];
	const states: UploaderState[] = [];
	const client = {
		uploadRecordingChunk: uploadFn,
		getRecording: getFn,
	} as unknown as ApiClient;
	const uploader = new ChunkUploader(
		client,
		"rec_0123456789",
		(s) => states.push(s),
		{
			sleep: async (ms) => {
				sleeps.push(ms);
			},
			digest: async () => "d".repeat(64),
		},
	);
	return { uploader, upload: uploadFn, get: getFn, sleeps, states };
}

const indexOf = (call: unknown[]): number => call[2] as number;
const trackOf = (call: unknown[]): string => call[1] as string;
const counted = (index: unknown): RecordingUploadChunkResult =>
	ok(index as number);

describe("the chunks are sent in order, each track numbered from 0", () => {
	test("every chunk goes up once, in the order it was made, and the count of the server is the one to declare", async () => {
		const h = make(async (_id, _kind, index) => counted(index));
		h.uploader.enqueue("screen", blob(10), 100);
		h.uploader.enqueue("camera", blob(5), 100);
		h.uploader.enqueue("screen", blob(10), 200);
		h.uploader.enqueue("screen", blob(10), 300);

		const done = await h.uploader.drain();

		expect(
			h.upload.mock.calls.map((c) => `${trackOf(c)}:${indexOf(c)}`),
		).toEqual(["screen:0", "camera:0", "screen:1", "screen:2"]);
		expect(done.phase).toBe("idle");
		expect(done.pending).toBe(0);
		expect(done.bytes).toBe(35);
		expect(h.uploader.acknowledged()).toEqual([
			{ kind: "screen", chunks: 3 },
			{ kind: "camera", chunks: 1 },
		]);
	});

	test("an empty chunk is not sent and takes no number", async () => {
		const h = make(async (_id, _kind, index) => counted(index));
		h.uploader.enqueue("screen", blob(0), 0);
		h.uploader.enqueue("screen", blob(3), 0);
		await h.uploader.drain();
		expect(h.upload).toHaveBeenCalledTimes(1);
		expect(indexOf(h.upload.mock.calls[0] ?? [])).toBe(0);
	});

	test("the digest and the clock travel with the chunk", async () => {
		const h = make(async (_id, _kind, index) => counted(index));
		h.uploader.enqueue("screen", blob(4), 1234);
		await h.uploader.drain();
		const call = h.upload.mock.calls[0] ?? [];
		expect(call[4]).toBe("d".repeat(64));
		expect(call[5]).toBe(1234);
	});
});

describe("a dropped connection", () => {
	test("a failed request is sent again after a wait, with the same index", async () => {
		let n = 0;
		const h = make(async (_id, _kind, index) => {
			n++;
			if (n < 3) throw api("E_NETWORK");
			return counted(index);
		});
		h.uploader.enqueue("screen", blob(4), 0);
		const done = await h.uploader.drain();
		expect(h.upload.mock.calls.map(indexOf)).toEqual([0, 0, 0]);
		expect(h.sleeps).toEqual([RETRY_WAITS_MS[0], RETRY_WAITS_MS[1]]);
		expect(done.end).toBeNull();
		expect(done.chunks.screen).toBe(1);
		expect(h.states.some((s) => s.phase === "retrying")).toBe(true);
	});

	test("when the answer was lost the server is asked where it is, and a stored chunk is not sent again", async () => {
		let n = 0;
		const h = make(
			async (_id, _kind, index) => {
				n++;
				if (n === 1) throw api("E_NETWORK");
				return counted(index);
			},
			async () => ({
				upload: {
					container: "webm",
					max_chunk_bytes: 1,
					tracks: [{ kind: "screen", next_index: 1, bytes: 4 }],
				},
			}),
		);
		h.uploader.enqueue("screen", blob(4), 0);
		const done = await h.uploader.drain();
		expect(h.upload).toHaveBeenCalledTimes(1);
		expect(h.get).toHaveBeenCalledTimes(1);
		expect(done.chunks.screen).toBe(1);
	});

	test("a chunk the server says it already has past this index counts as sent", async () => {
		const h = make(async () => {
			throw api("E_INVALID_STATE", { reason: "out_of_order", expected: 5 });
		});
		h.uploader.enqueue("screen", blob(4), 0);
		const done = await h.uploader.drain();
		expect(done.end).toBeNull();
		expect(done.chunks.screen).toBe(1);
	});

	test("after the last wait the upload is given up and says why", async () => {
		const h = make(async () => {
			throw api("E_NETWORK");
		});
		h.uploader.enqueue("screen", blob(4), 0);
		h.uploader.enqueue("screen", blob(4), 0);
		const done = await h.uploader.drain();
		expect(h.upload).toHaveBeenCalledTimes(RETRY_WAITS_MS.length + 1);
		expect(done.end?.kind).toBe("failed");
		expect(done.phase).toBe("ended");
		expect(done.pending).toBe(0);
		// nothing more is taken once it ended
		h.uploader.enqueue("screen", blob(4), 0);
		expect(h.uploader.snapshot().pending).toBe(0);
	});
});

describe("what the server refuses", () => {
	test("a limit ends the upload with its reason, and what was sent stays acknowledged", async () => {
		let n = 0;
		const h = make(async (_id, _kind, index) => {
			n++;
			if (n === 2) throw api("E_LIMIT_EXCEEDED", { reason: "size" });
			return counted(index);
		});
		h.uploader.enqueue("screen", blob(4), 0);
		h.uploader.enqueue("screen", blob(4), 0);
		h.uploader.enqueue("screen", blob(4), 0);
		const done = await h.uploader.drain();
		expect(done.end).toEqual({ kind: "limit", reason: "size" });
		expect(h.uploader.acknowledged()).toEqual([{ kind: "screen", chunks: 1 }]);
		expect(h.sleeps).toEqual([]);
	});

	test("a session that is not a person, a recording that is gone or one that no longer receives are not tried again", async () => {
		for (const [code, details] of [
			["E_PRIVILEGED_SESSION", undefined],
			["E_RECORDING_NOT_FOUND", undefined],
			["E_INVALID_STATE", { reason: "not_receiving" }],
			["E_INVALID_STATE", { reason: "chunk_conflict" }],
			["E_PAYLOAD_TOO_LARGE", undefined],
		] as const) {
			const h = make(async () => {
				throw api(code, details);
			});
			h.uploader.enqueue("screen", blob(4), 0);
			const done = await h.uploader.drain();
			expect(done.end?.kind, code).toBe("failed");
			expect(h.upload, code).toHaveBeenCalledTimes(1);
			expect(h.sleeps, code).toEqual([]);
		}
	});

	test("a digest the server did not accept is sent again: the bytes may have been damaged on the way", async () => {
		let n = 0;
		const h = make(async (_id, _kind, index) => {
			n++;
			if (n === 1) throw api("E_VALIDATION", { reason: "checksum_mismatch" });
			return counted(index);
		});
		h.uploader.enqueue("screen", blob(4), 0);
		const done = await h.uploader.drain();
		expect(done.end).toBeNull();
		expect(h.upload).toHaveBeenCalledTimes(2);
	});
});

test("drain resolves at once when nothing was queued", async () => {
	const h = make(async (_id, _kind, index) => counted(index));
	await expect(h.uploader.drain()).resolves.toMatchObject({
		phase: "idle",
		pending: 0,
	});
});

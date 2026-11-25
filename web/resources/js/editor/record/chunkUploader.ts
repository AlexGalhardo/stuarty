import type { ApiClient } from "../api/client";
import { ApiError } from "../api/errors";
import type { RecordingUploadKind } from "../api/types";

// The sending side of a recording a browser captures (SDD-025 sections 17.3 and 17.4). Chunks are sent in order,
// one at a time, each with its SHA-256; the server accepts a chunk only at the index the track waits for, and a
// chunk sent twice only once, so a request that failed is simply sent again. This file keeps the chunks that the
// server has not acknowledged and finds out, after a failure, where the server is. It decides no rule: the order,
// the checksum and the limits are the server's.

export interface UploaderDeps {
	sleep(ms: number): Promise<void>;
	digest(chunk: Blob): Promise<string>;
}

// How long to wait before each new attempt at the same chunk. After the last one the upload is given up, and the
// recording is stopped so that the server keeps what it has.
export const RETRY_WAITS_MS = [
	1000, 2000, 4000, 8000, 15_000, 15_000, 15_000, 15_000,
] as const;

export type UploadPhase = "idle" | "sending" | "retrying" | "ended";

// Why the upload ended before the person stopped it: a limit the server reported, or a failure that no new
// attempt can mend (the session is not a person, the recording is gone, a chunk was lost).
export type UploadEnd =
	| { kind: "limit"; reason: string }
	| { kind: "failed"; error: Error };

export interface UploaderState {
	phase: UploadPhase;
	// chunks queued or in flight
	pending: number;
	// what the server has acknowledged, by track
	chunks: Record<RecordingUploadKind, number>;
	bytes: number;
	end: UploadEnd | null;
}

interface Item {
	kind: RecordingUploadKind;
	index: number;
	blob: Blob;
	elapsedMs: number;
}

const FATAL_CODES = new Set([
	"E_PRIVILEGED_SESSION",
	"E_UNAUTHORIZED",
	"E_FORBIDDEN",
	"E_RECORDING_NOT_FOUND",
	"E_LOCKED",
	"E_NOT_FOUND",
]);

export async function sha256Hex(chunk: Blob): Promise<string> {
	const digest = await crypto.subtle.digest(
		"SHA-256",
		await chunk.arrayBuffer(),
	);
	return Array.from(new Uint8Array(digest), (b) =>
		b.toString(16).padStart(2, "0"),
	).join("");
}

const realDeps: UploaderDeps = {
	sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
	digest: sha256Hex,
};

export class ChunkUploader {
	private readonly queue: Item[] = [];
	private readonly next: Record<RecordingUploadKind, number> = {
		screen: 0,
		camera: 0,
	};
	private state: UploaderState = {
		phase: "idle",
		pending: 0,
		chunks: { screen: 0, camera: 0 },
		bytes: 0,
		end: null,
	};
	private running = false;
	private idle: Array<() => void> = [];

	constructor(
		private readonly client: ApiClient,
		private readonly recordingId: string,
		private readonly onChange: (state: UploaderState) => void,
		private readonly deps: UploaderDeps = realDeps,
	) {}

	snapshot(): UploaderState {
		return this.state;
	}

	// The chunk is numbered here, in the order the recorder produced it, and sent when the ones before it were.
	enqueue(kind: RecordingUploadKind, blob: Blob, elapsedMs: number): void {
		if (this.state.end !== null || blob.size === 0) return;
		const index = this.next[kind];
		this.next[kind] = index + 1;
		this.queue.push({ kind, index, blob, elapsedMs });
		this.set({ pending: this.queue.length });
		if (!this.running) void this.run();
	}

	// The number of chunks the server holds, by track: what the finish declares.
	acknowledged(): { kind: RecordingUploadKind; chunks: number }[] {
		return (["screen", "camera"] as const)
			.filter((kind) => this.next[kind] > 0)
			.map((kind) => ({ kind, chunks: this.state.chunks[kind] }));
	}

	// Resolves when every chunk queued so far was acknowledged or the upload ended; the state says which.
	drain(): Promise<UploaderState> {
		if (!this.running && this.queue.length === 0) {
			return Promise.resolve(this.state);
		}
		return new Promise((resolve) => {
			this.idle.push(() => resolve(this.state));
		});
	}

	private set(change: Partial<UploaderState>): void {
		this.state = { ...this.state, ...change };
		this.onChange(this.state);
	}

	private async run(): Promise<void> {
		this.running = true;
		try {
			while (this.queue.length > 0 && this.state.end === null) {
				const item = this.queue[0];
				if (item === undefined) break;
				const outcome = await this.send(item);
				if (outcome === "sent") {
					this.queue.shift();
					this.set({
						pending: this.queue.length,
						phase: "sending",
						chunks: {
							...this.state.chunks,
							[item.kind]: item.index + 1,
						},
						bytes: this.state.bytes + item.blob.size,
					});
				}
			}
		} finally {
			if (this.state.end !== null) {
				this.queue.length = 0;
				this.set({ phase: "ended", pending: 0 });
			} else {
				this.set({ phase: "idle" });
			}
			this.running = false;
			const waiting = this.idle;
			this.idle = [];
			for (const resolve of waiting) resolve();
		}
	}

	// One chunk, with the attempts of RETRY_WAITS_MS. It returns "sent" or leaves `end` set.
	private async send(item: Item): Promise<"sent" | "ended"> {
		const sha256 = await this.deps.digest(item.blob);
		for (let attempt = 0; ; attempt++) {
			try {
				await this.client.uploadRecordingChunk(
					this.recordingId,
					item.kind,
					item.index,
					item.blob,
					sha256,
					item.elapsedMs,
				);
				return "sent";
			} catch (error) {
				const verdict = this.judge(error, item);
				if (verdict === "ended") return "ended";
				if (verdict === "sent") return "sent";
				const wait = RETRY_WAITS_MS[attempt];
				if (wait === undefined) {
					this.set({ end: { kind: "failed", error: asError(error) } });
					return "ended";
				}
				this.set({ phase: "retrying" });
				await this.deps.sleep(wait);
				// After a failure the server may have the chunk (the answer was lost): ask where it is.
				if (await this.alreadyStored(item)) return "sent";
			}
		}
	}

	// What an error of the server means for the chunk: the upload ended, the chunk is there after all, or it is sent
	// again.
	private judge(error: unknown, item: Item): "retry" | "sent" | "ended" {
		if (!(error instanceof ApiError)) return "retry";
		if (error.code === "E_LIMIT_EXCEEDED") {
			const reason = error.details?.reason;
			this.set({
				end: {
					kind: "limit",
					reason: typeof reason === "string" ? reason : "size",
				},
			});
			return "ended";
		}
		if (FATAL_CODES.has(error.code)) {
			this.set({ end: { kind: "failed", error } });
			return "ended";
		}
		if (error.code === "E_INVALID_STATE") {
			const reason = error.details?.reason;
			const expected = error.details?.expected;
			if (
				reason === "out_of_order" &&
				typeof expected === "number" &&
				expected > item.index
			) {
				return "sent";
			}
			// not receiving, a chunk that conflicts, a gap: nothing sent again can mend it
			this.set({ end: { kind: "failed", error } });
			return "ended";
		}
		// E_PAYLOAD_TOO_LARGE and E_VALIDATION do not change by trying again, except a digest that did not match: the
		// bytes may have been damaged on the way
		if (
			error.code === "E_PAYLOAD_TOO_LARGE" ||
			(error.code === "E_VALIDATION" &&
				error.details?.reason !== "checksum_mismatch")
		) {
			this.set({ end: { kind: "failed", error } });
			return "ended";
		}
		return "retry";
	}

	private async alreadyStored(item: Item): Promise<boolean> {
		try {
			const rec = await this.client.getRecording(this.recordingId);
			const track = rec.upload?.tracks.find((t) => t.kind === item.kind);
			return track !== undefined && track.next_index > item.index;
		} catch {
			return false;
		}
	}
}

function asError(error: unknown): Error {
	return error instanceof Error ? error : new Error(String(error));
}

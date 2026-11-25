import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";

import type { ApiClient } from "../api/client";
import { ApiError } from "../api/errors";
import type { Recording } from "../api/types";
import {
	type CaptureChoice,
	endKey,
	type RecorderState,
	startBrowserRecording,
} from "./browserRecorder";

// The browser is replaced by fakes: the recorder is tested for what it asks of the browser and of the server, not
// for what a real MediaRecorder writes (the end to end test runs one).

class FakeTrack {
	readonly stop = vi.fn();
	private readonly listeners = new Map<string, Array<() => void>>();
	constructor(readonly kind: "audio" | "video") {}
	addEventListener(type: string, fn: () => void): void {
		this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn]);
	}
	end(): void {
		for (const fn of this.listeners.get("ended") ?? []) fn();
	}
}

class FakeStream {
	constructor(readonly tracks: FakeTrack[] = []) {}
	getTracks(): FakeTrack[] {
		return this.tracks;
	}
	getVideoTracks(): FakeTrack[] {
		return this.tracks.filter((t) => t.kind === "video");
	}
	getAudioTracks(): FakeTrack[] {
		return this.tracks.filter((t) => t.kind === "audio");
	}
}

const recorders: FakeRecorder[] = [];

class FakeRecorder {
	state: "inactive" | "recording" = "inactive";
	ondataavailable: ((e: { data: Blob }) => void) | null = null;
	started: number | null = null;
	private stopListeners: Array<() => void> = [];
	constructor(
		readonly stream: FakeStream,
		readonly options: { mimeType: string },
	) {
		recorders.push(this);
	}
	start(timeslice: number): void {
		this.state = "recording";
		this.started = timeslice;
	}
	emit(bytes: number): void {
		this.ondataavailable?.({ data: new Blob([new Uint8Array(bytes)]) });
	}
	addEventListener(_type: string, fn: () => void): void {
		this.stopListeners.push(fn);
	}
	stop(): void {
		this.state = "inactive";
		this.emit(3);
		for (const fn of this.stopListeners) fn();
	}
}

const rec = (over: Partial<Recording> = {}): Recording => ({
	recording_id: "rec_0123456789",
	project_id: "prj_0123456789",
	state: "recording",
	source: { kind: "browser" },
	fps: 0,
	duration_ms: 0,
	frames: 0,
	dropped: 0,
	bytes: 0,
	actor: "local:ui",
	created_at: "2026-11-24T00:00:00Z",
	text: "",
	...over,
});

interface World {
	display: FakeStream;
	mic: FakeStream;
	camera: FakeStream;
	client: {
		beginRecordingUpload: ReturnType<typeof vi.fn>;
		uploadRecordingChunk: ReturnType<typeof vi.fn>;
		finishRecordingUpload: ReturnType<typeof vi.fn>;
		stopRecording: ReturnType<typeof vi.fn>;
		getRecording: ReturnType<typeof vi.fn>;
	};
	getDisplayMedia: ReturnType<typeof vi.fn>;
	getUserMedia: ReturnType<typeof vi.fn>;
	states: RecorderState[];
}

function world(
	over: { displayAudio?: boolean; micDenied?: boolean } = {},
): World {
	const display = new FakeStream([
		new FakeTrack("video"),
		...(over.displayAudio === true ? [new FakeTrack("audio")] : []),
	]);
	const mic = new FakeStream([new FakeTrack("audio")]);
	const camera = new FakeStream([new FakeTrack("video")]);
	const getDisplayMedia = vi.fn(async () => display);
	const getUserMedia = vi.fn(
		async (c: { audio?: boolean; video?: boolean }) => {
			if (c.audio === true) {
				if (over.micDenied === true) throw new Error("NotAllowedError");
				return mic;
			}
			return camera;
		},
	);
	vi.stubGlobal("navigator", {
		mediaDevices: { getDisplayMedia, getUserMedia },
	});
	vi.stubGlobal("MediaStream", FakeStream);
	vi.stubGlobal("MediaRecorder", FakeRecorder);
	vi.stubGlobal(
		"AudioContext",
		class {
			createMediaStreamDestination(): { stream: FakeStream } {
				return { stream: new FakeStream([new FakeTrack("audio")]) };
			}
			createMediaStreamSource(): { connect: () => void } {
				return { connect: () => {} };
			}
			close(): Promise<void> {
				return Promise.resolve();
			}
		},
	);
	return {
		display,
		mic,
		camera,
		getDisplayMedia,
		getUserMedia,
		states: [],
		client: {
			beginRecordingUpload: vi.fn(async () => rec()),
			uploadRecordingChunk: vi.fn(
				async (_id: string, _kind: string, index: number) => ({
					recording_id: "rec_0123456789",
					track: "screen",
					index,
					next_index: index + 1,
					duplicate: false,
					bytes: 0,
					text: "",
				}),
			),
			finishRecordingUpload: vi.fn(async () => rec({ state: "finishing" })),
			stopRecording: vi.fn(async () => rec({ state: "finishing" })),
			getRecording: vi.fn(async () => rec()),
		},
	};
}

const choice = (over: Partial<CaptureChoice> = {}): CaptureChoice => ({
	microphone: true,
	camera: false,
	displayAudio: false,
	...over,
});

function setup(w: World, c: CaptureChoice) {
	return {
		client: w.client as unknown as ApiClient,
		projectId: "prj_0123456789",
		mimeType: "video/webm;codecs=vp9,opus",
		container: "webm" as const,
		choice: c,
		onChange: (s: RecorderState) => w.states.push(s),
		tickMs: 10,
	};
}

beforeEach(() => {
	recorders.length = 0;
});
afterEach(() => {
	vi.unstubAllGlobals();
});

describe("a recording the browser makes", () => {
	test("asks the browser for what the person chose, begins on the server and slices by the second", async () => {
		const w = world();
		const r = await startBrowserRecording(setup(w, choice()));

		expect(w.getDisplayMedia).toHaveBeenCalledWith({
			video: true,
			audio: false,
		});
		expect(w.getUserMedia).toHaveBeenCalledWith({ audio: true });
		expect(w.client.beginRecordingUpload).toHaveBeenCalledWith({
			project_id: "prj_0123456789",
			container: "webm",
			label: "Browser",
		});
		expect(recorders).toHaveLength(1);
		expect(recorders[0]?.options.mimeType).toBe("video/webm;codecs=vp9,opus");
		expect(recorders[0]?.started).toBe(1000);
		expect(r.state().shared).toEqual({
			screen: true,
			microphone: true,
			displayAudio: false,
			camera: false,
		});
		// the recorder holds the picture and the voice in one stream
		expect(recorders[0]?.stream.getTracks().map((t) => t.kind)).toEqual([
			"video",
			"audio",
		]);
		await r.stop();
	});

	test("the chunks go up in order, then the server is told how many there were", async () => {
		const w = world();
		const r = await startBrowserRecording(setup(w, choice()));

		recorders[0]?.emit(100);
		recorders[0]?.emit(200);
		const end = await r.stop();

		// two chunks while recording and the last one the recorder flushes when it stops
		expect(
			w.client.uploadRecordingChunk.mock.calls.map((c) => [c[1], c[2]]),
		).toEqual([
			["screen", 0],
			["screen", 1],
			["screen", 2],
		]);
		expect(w.client.finishRecordingUpload).toHaveBeenCalledWith(
			"rec_0123456789",
			{
				tracks: [{ kind: "screen", chunks: 3 }],
				elapsed_ms: expect.any(Number),
			},
		);
		expect(end.phase).toBe("ended");
		expect(end.stoppedBy).toBe("person");
		expect(end.recording?.state).toBe("finishing");
		expect(end.error).toBeNull();
		// the indicators of the browser go off
		for (const s of [w.display, w.mic]) {
			for (const t of s.tracks) expect(t.stop).toHaveBeenCalled();
		}
	});

	test("a microphone the person declines does not stop the recording", async () => {
		const w = world({ micDenied: true });
		const r = await startBrowserRecording(setup(w, choice()));
		expect(r.state().shared.microphone).toBe(false);
		expect(recorders[0]?.stream.getAudioTracks()).toHaveLength(0);
		await r.stop();
		expect(w.client.finishRecordingUpload).toHaveBeenCalledTimes(1);
	});

	test("the sound of the page and the microphone are mixed into one track", async () => {
		const w = world({ displayAudio: true });
		const r = await startBrowserRecording(
			setup(w, choice({ displayAudio: true })),
		);
		expect(w.getDisplayMedia).toHaveBeenCalledWith({
			video: true,
			audio: true,
		});
		expect(r.state().shared.displayAudio).toBe(true);
		expect(recorders[0]?.stream.getAudioTracks()).toHaveLength(1);
		await r.stop();
	});

	test("the camera is a second recorder on its own track", async () => {
		const w = world();
		const r = await startBrowserRecording(setup(w, choice({ camera: true })));
		expect(w.client.beginRecordingUpload).toHaveBeenCalledWith(
			expect.objectContaining({ camera: true }),
		);
		expect(recorders).toHaveLength(2);
		recorders[1]?.emit(50);
		await r.stop();
		const kinds = w.client.uploadRecordingChunk.mock.calls.map((c) => c[1]);
		expect(kinds).toContain("camera");
		const sent = w.client.finishRecordingUpload.mock.calls[0]?.[1] as {
			tracks: { kind: string; chunks: number }[];
		};
		expect(sent.tracks.map((t) => t.kind).sort()).toEqual(["camera", "screen"]);
	});

	test("a screen the person declines starts nothing on the server", async () => {
		const w = world();
		w.getDisplayMedia.mockRejectedValue(new Error("NotAllowedError"));
		await expect(startBrowserRecording(setup(w, choice()))).rejects.toThrow(
			"NotAllowedError",
		);
		expect(w.client.beginRecordingUpload).not.toHaveBeenCalled();
		expect(recorders).toHaveLength(0);
	});

	test("a server that refuses the begin leaves no capture running", async () => {
		const w = world();
		w.client.beginRecordingUpload.mockRejectedValue(
			new ApiError(403, { code: "E_PRIVILEGED_SESSION", message: "no" }),
		);
		await expect(
			startBrowserRecording(setup(w, choice())),
		).rejects.toMatchObject({
			code: "E_PRIVILEGED_SESSION",
		});
		for (const t of [...w.display.tracks, ...w.mic.tracks]) {
			expect(t.stop).toHaveBeenCalled();
		}
	});

	test("Stop sharing in the browser ends the recording as Stop does", async () => {
		const w = world();
		const r = await startBrowserRecording(
			setup(w, choice({ microphone: false })),
		);
		w.display.getVideoTracks()[0]?.end();
		const end = await r.stop();
		expect(end.stoppedBy).toBe("browser");
		expect(w.client.finishRecordingUpload).toHaveBeenCalledTimes(1);
	});

	test("a limit of the server stops the capture and keeps what was sent", async () => {
		const w = world();
		let n = 0;
		w.client.uploadRecordingChunk.mockImplementation(
			async (_id: string, _kind: string, index: number) => {
				n++;
				if (n === 2) {
					throw new ApiError(422, {
						code: "E_LIMIT_EXCEEDED",
						message: "size",
						details: { reason: "size" },
					});
				}
				return { index, next_index: index + 1 };
			},
		);
		const r = await startBrowserRecording(
			setup(w, choice({ microphone: false })),
		);
		recorders[0]?.emit(10);
		recorders[0]?.emit(10);
		// the refusal comes while the person is still recording: the capture is stopped for them
		await vi.waitFor(() => expect(r.state().stoppedBy).toBe("limit"));
		const end = await r.stop();
		expect(end.stoppedBy).toBe("limit");
		expect(end.upload.end).toEqual({ kind: "limit", reason: "size" });
		expect(endKey(end.upload.end)).toBe("ui.editor.record.browser.limit.size");
		expect(w.client.finishRecordingUpload).toHaveBeenCalledWith(
			"rec_0123456789",
			expect.objectContaining({ tracks: [{ kind: "screen", chunks: 1 }] }),
		);
	});

	test("an upload that cannot go on stops the recording on the server, which keeps what it has", async () => {
		const w = world();
		w.client.uploadRecordingChunk.mockRejectedValue(
			new ApiError(403, { code: "E_PRIVILEGED_SESSION", message: "no" }),
		);
		const r = await startBrowserRecording(
			setup(w, choice({ microphone: false })),
		);
		recorders[0]?.emit(10);
		const end = await r.stop();
		expect(end.upload.end?.kind).toBe("failed");
		expect(w.client.stopRecording).toHaveBeenCalledWith("rec_0123456789");
		expect(w.client.finishRecordingUpload).not.toHaveBeenCalled();
		expect(end.error).not.toBeNull();
		expect(endKey(end.upload.end)).toBe(
			"ui.editor.record.browser.upload_failed",
		);
	});

	test("the clock moves while recording and stops at the stop", async () => {
		const w = world();
		const r = await startBrowserRecording(
			setup(w, choice({ microphone: false })),
		);
		await new Promise((resolve) => setTimeout(resolve, 60));
		expect(r.state().elapsedMs).toBeGreaterThan(0);
		const end = await r.stop();
		const at = end.elapsedMs;
		await new Promise((resolve) => setTimeout(resolve, 40));
		expect(r.state().elapsedMs).toBe(at);
	});

	test("stopping twice is one stop", async () => {
		const w = world();
		const r = await startBrowserRecording(
			setup(w, choice({ microphone: false })),
		);
		await Promise.all([r.stop(), r.stop()]);
		expect(w.client.finishRecordingUpload).toHaveBeenCalledTimes(1);
	});
});

import type { ApiClient } from "../api/client";
import type {
	Recording,
	RecordingContainer,
	RecordingUploadKind,
} from "../api/types";
import {
	ChunkUploader,
	type UploadEnd,
	type UploaderState,
} from "./chunkUploader";

// The recorder of a recording a browser captures (SDD-025 section 18). It asks the browser for the screen, the
// microphone and the camera on a gesture of the person (the permission prompts are the browser's and are never
// answered here), slices what MediaRecorder writes into chunks of about a second, and hands them to the uploader.
// It computes no edit and no limit: what the person reads after a stop is what the server answered.

// What the person chose to share besides the screen.
export interface CaptureChoice {
	microphone: boolean;
	camera: boolean;
	// ask the browser for the sound of the page; the picker of the browser decides whether it is offered
	displayAudio: boolean;
}

// What the browser actually handed over: a track the person declined, or the browser did not offer, is false.
export interface SharedTracks {
	screen: true;
	microphone: boolean;
	displayAudio: boolean;
	camera: boolean;
}

export type RecorderPhase =
	| "starting"
	| "recording"
	| "paused"
	// the person stopped: the last chunks go up and the server is told
	| "sending"
	| "finishing"
	| "ended";

export interface RecorderState {
	phase: RecorderPhase;
	recordingId: string;
	// the clock of the recorder, for the eye; the service says the duration of the file
	elapsedMs: number;
	shared: SharedTracks;
	upload: UploaderState;
	// the server was told to keep what it has and the browser stopped before the person did
	stoppedBy: "person" | "browser" | "limit" | "failure" | null;
	// the answer to the last call of the server, once the recording finished
	recording: Recording | null;
	error: Error | null;
}

export interface RecorderSetup {
	client: ApiClient;
	projectId: string | undefined;
	mimeType: string;
	container: RecordingContainer;
	choice: CaptureChoice;
	onChange(state: RecorderState): void;
	// how often the clock is read, in milliseconds
	tickMs?: number;
}

// The slice of the recorder: the time between two chunks.
const TIMESLICE_MS = 1000;
const TICK_MS = 250;

export interface BrowserRecording {
	readonly recordingId: string;
	stop(): Promise<RecorderState>;
	togglePause(): void;
	cancel(): Promise<void>;
	state(): RecorderState;
}

// A stream the browser gave: its tracks are stopped when the recording ends, which turns off the indicators of
// the browser (the camera light, the sharing banner).
function stopAll(...streams: (MediaStream | null)[]): void {
	for (const s of streams) {
		for (const t of s?.getTracks() ?? []) t.stop();
	}
}

// One audio track out of up to two: the sound of the page and the microphone are mixed by the browser, because a
// recorder writes one stream. With one source it is used as it is.
function mixAudio(streams: MediaStream[]): {
	track: MediaStreamTrack | null;
	close: () => void;
} {
	const tracks = streams.flatMap((s) => s.getAudioTracks());
	const [only] = tracks;
	if (only === undefined) return { track: null, close: () => undefined };
	if (tracks.length === 1) return { track: only, close: () => undefined };
	const context = new AudioContext();
	const out = context.createMediaStreamDestination();
	for (const s of streams) {
		if (s.getAudioTracks().length > 0) {
			context.createMediaStreamSource(s).connect(out);
		}
	}
	return {
		track: out.stream.getAudioTracks()[0] ?? null,
		close: () => void context.close(),
	};
}

// Starts a recording. It must be called from the click of the person (the browser refuses a screen capture
// otherwise). It throws what the browser throws when the person declines the screen (`NotAllowedError`), before
// anything exists on the server.
export async function startBrowserRecording(
	setup: RecorderSetup,
): Promise<BrowserRecording> {
	const { client, choice } = setup;

	// The browser asks the person for each source, one prompt each.
	const display = await navigator.mediaDevices.getDisplayMedia({
		video: true,
		audio: choice.displayAudio,
	});
	let micStream: MediaStream | null = null;
	let cameraStream: MediaStream | null = null;
	try {
		if (choice.microphone) {
			micStream = await navigator.mediaDevices
				.getUserMedia({ audio: true })
				.catch(() => null);
		}
		if (choice.camera) {
			cameraStream = await navigator.mediaDevices
				.getUserMedia({ video: true })
				.catch(() => null);
		}
	} catch (error) {
		stopAll(display, micStream, cameraStream);
		throw error;
	}

	const shared: SharedTracks = {
		screen: true,
		microphone: micStream !== null && micStream.getAudioTracks().length > 0,
		displayAudio: display.getAudioTracks().length > 0,
		camera: cameraStream !== null && cameraStream.getVideoTracks().length > 0,
	};

	let begun: Recording;
	try {
		begun = await client.beginRecordingUpload({
			...(setup.projectId ? { project_id: setup.projectId } : {}),
			container: setup.container,
			...(shared.camera ? { camera: true } : {}),
			label: "Browser",
		});
	} catch (error) {
		stopAll(display, micStream, cameraStream);
		throw error;
	}

	let mix: { track: MediaStreamTrack | null; close(): void } | null = null;
	const recorders: { kind: RecordingUploadKind; recorder: MediaRecorder }[] =
		[];
	let timer: ReturnType<typeof setInterval> | null = null;

	try {
		mix = mixAudio(
			[display, micStream].filter((s): s is MediaStream => s !== null),
		);
		const screenTracks: MediaStreamTrack[] = [...display.getVideoTracks()];
		if (mix.track !== null) screenTracks.push(mix.track);
		const screenStream = new MediaStream(screenTracks);

		const startedAt = performance.now();
		let pausedAt = 0;
		let pausedTotal = 0;
		const elapsed = () =>
			Math.round((pausedAt || performance.now()) - startedAt - pausedTotal);
		let elapsedMs = 0;
		let stoppedBy: RecorderState["stoppedBy"] = null;
		let phase: RecorderPhase = "recording";
		let recording: Recording | null = null;
		let failure: Error | null = null;
		let current: RecorderState;

		const emit = (upload: UploaderState): void => {
			current = {
				phase,
				recordingId: begun.recording_id,
				elapsedMs,
				shared,
				upload,
				stoppedBy,
				recording,
				error: failure,
			};
			setup.onChange(current);
		};

		const uploader = new ChunkUploader(client, begun.recording_id, (u) => {
			emit(u);
			// the server said enough (a limit) or the upload cannot go on: stop what is being captured
			if (u.end !== null && (phase === "recording" || phase === "paused")) {
				stoppedBy = u.end.kind === "limit" ? "limit" : "failure";
				void halt();
			}
		});
		emit(uploader.snapshot());

		const make = (kind: RecordingUploadKind, stream: MediaStream): void => {
			const recorder = new MediaRecorder(stream, { mimeType: setup.mimeType });
			recorder.ondataavailable = (event: BlobEvent) => {
				if (event.data.size > 0) {
					uploader.enqueue(kind, event.data, elapsed());
				}
			};
			recorders.push({ kind, recorder });
		};
		make("screen", screenStream);
		if (shared.camera && cameraStream !== null) make("camera", cameraStream);

		timer = setInterval(() => {
			if (phase !== "recording") return;
			elapsedMs = elapsed();
			emit(uploader.snapshot());
		}, setup.tickMs ?? TICK_MS);

		const stopped = (recorder: MediaRecorder): Promise<void> =>
			new Promise((resolve) => {
				if (recorder.state === "inactive") {
					resolve();
					return;
				}
				recorder.addEventListener("stop", () => resolve(), { once: true });
				recorder.stop();
			});

		let ending: Promise<RecorderState> | null = null;
		let discarding = false;
		const audioMix = mix;
		const tickTimer = timer;

		// Stops the capture and ends the recording on the server: the last chunks are flushed by the recorders'
		// own stop, sent, and then the server is told how many chunks there were.
		async function halt(): Promise<RecorderState> {
			if (ending !== null) return ending;
			ending = (async () => {
				elapsedMs = elapsed();
				phase = "sending";
				emit(uploader.snapshot());
				await Promise.all(recorders.map((r) => stopped(r.recorder)));
				clearInterval(tickTimer);
				stopAll(display, micStream, cameraStream);
				audioMix.close();
				const finalState = await uploader.drain();
				phase = "finishing";
				emit(finalState);
				try {
					if (discarding) {
						recording = await client.cancelRecording(begun.recording_id);
					} else {
						recording =
							finalState.end?.kind === "failed"
								? await client.stopRecording(begun.recording_id)
								: await client.finishRecordingUpload(begun.recording_id, {
										tracks: uploader.acknowledged(),
										elapsed_ms: elapsedMs,
									});
					}
				} catch (error) {
					failure = error instanceof Error ? error : new Error(String(error));
				}
				if (finalState.end?.kind === "failed") {
					failure ??= finalState.end.error;
				}
				phase = "ended";
				emit(finalState);
				return current;
			})();
			return ending;
		}

		// The person pressed "Stop sharing" in the browser: that ends the recording as Stop does.
		display.getVideoTracks()[0]?.addEventListener("ended", () => {
			if (phase === "recording" || phase === "paused") {
				stoppedBy = "browser";
				void halt();
			}
		});

		for (const r of recorders) r.recorder.start(TIMESLICE_MS);

		return {
			recordingId: begun.recording_id,
			state: () => current,
			togglePause: () => {
				if (phase === "recording") {
					pausedAt = performance.now();
					elapsedMs = elapsed();
					phase = "paused";
					for (const { recorder } of recorders) recorder.pause();
				} else if (phase === "paused") {
					pausedTotal += performance.now() - pausedAt;
					pausedAt = 0;
					phase = "recording";
					for (const { recorder } of recorders) recorder.resume();
				}
				emit(uploader.snapshot());
			},
			cancel: async () => {
				discarding = true;
				const result = await halt();
				if (result.error !== null) throw result.error;
			},
			stop: () => {
				if (phase === "recording") stoppedBy = "person";
				return halt();
			},
		};
	} catch (error) {
		if (timer !== null) clearInterval(timer);
		for (const r of recorders) {
			if (r.recorder.state !== "inactive") {
				try {
					r.recorder.stop();
				} catch {
					// ignore recorder stop errors during rollback
				}
			}
		}
		stopAll(display, micStream, cameraStream);
		mix?.close();
		void client.cancelRecording(begun.recording_id).catch(() => undefined);
		throw error;
	}
}

// The sentence key for why the upload ended before the person stopped it.
export function endKey(end: UploadEnd | null): string | null {
	if (end === null) return null;
	return end.kind === "limit"
		? `ui.editor.record.browser.limit.${end.reason}`
		: "ui.editor.record.browser.upload_failed";
}

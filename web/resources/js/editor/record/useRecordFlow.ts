import { useCallback, useEffect, useRef, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import type { ApiClient } from "../api/client";
import { currentCapability } from "./browserCapture";
import {
	type BrowserRecording,
	type CaptureChoice,
	type RecorderState,
	startBrowserRecording,
} from "./browserRecorder";
import {
	browserOffered,
	browserViewOf,
	isKept,
	isLive,
	LIVE_POLL_MS,
	type RecordView,
	type StartOptions,
	unavailableReason,
	viewOf,
} from "./recordFlow";

export interface RecordFlow {
	view: RecordView;
	failure: Failure | null;
	busy: boolean;
	// the recording was placed on the timeline by this dialog
	placed: boolean;
	start(
		kind: "display" | "window",
		id: string,
		name: string,
		fps: number,
		extras: StartOptions,
	): Promise<void>;
	stop(): Promise<void>;
	// a recording made in the browser (SDD-025 section 17): the browser captures and uploads, the server decides
	startBrowser(choice: CaptureChoice): Promise<void>;
	stopBrowser(): Promise<void>;
	// pausing is for anyone; resuming restarts the capture of the screen, so the service asks for a person
	pause(): Promise<void>;
	resume(): Promise<void>;
	// confirm and remove act on the recording on screen, or on a kept one named by its id
	confirm(id?: string): Promise<void>;
	dismiss(): Promise<void>;
	remove(id?: string): Promise<void>;
	place(): Promise<void>;
	// back to the picker, to record again
	reset(): void;
}

// One recording at a time (SDD-025 section 9.1). On opening, the dialog asks the service for the sources and for
// the recordings of the project: one still running, or one finished and waiting for a person, is picked up again,
// so closing the dialog never loses a take. While a recording runs it asks again every second; the clock it shows
// is the one the service reports.
export function useRecordFlow(
	client: ApiClient,
	projectId: string,
	open: boolean,
): RecordFlow {
	const [view, setView] = useState<RecordView>({ step: "loading" });
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	const [placed, setPlaced] = useState(false);
	const alive = useRef(true);
	// the recording this tab is making in the browser, if any: it outlives the dialog being closed
	const browser = useRef<BrowserRecording | null>(null);

	useEffect(() => {
		alive.current = true;
		return () => {
			alive.current = false;
		};
	}, []);

	const load = useCallback(async (): Promise<void> => {
		setView({ step: "loading" });
		setFailure(null);
		try {
			const list = await client.listRecordings(projectId);
			const waiting = list.recordings.find(
				(r) => isLive(r) || (r.state === "ready" && r.review === "pending"),
			);
			if (waiting !== undefined) {
				const own = browser.current;
				if (own !== null && own.recordingId === waiting.recording_id) {
					if (alive.current)
						setView({ step: "browser_live", state: own.state() });
				} else if (alive.current) setView(viewOf(waiting));
				return;
			}
			const sources = await client.recordingSources();
			if (alive.current)
				setView({
					step: "pick",
					sources,
					kept: list.recordings.filter(isKept),
				});
		} catch (error) {
			const reason = unavailableReason(error);
			if (!alive.current) return;
			if (reason !== null && browserOffered(reason)) {
				// no native capture here: the browser records and uploads instead
				try {
					const list = await client.listRecordings(projectId);
					if (alive.current)
						setView({
							step: "browser",
							capability: currentCapability(),
							kept: list.recordings.filter(isKept),
						});
				} catch (inner) {
					if (alive.current) {
						setView({ step: "loading" });
						setFailure(failureOf(inner));
					}
				}
			} else if (reason !== null) setView({ step: "unavailable", reason });
			else {
				setView({ step: "loading" });
				setFailure(failureOf(error));
			}
		}
	}, [client, projectId]);

	useEffect(() => {
		if (open) void load();
	}, [open, load]);

	const recordingId =
		view.step === "recording" ||
		view.step === "interrupted" ||
		view.step === "review" ||
		view.step === "added" ||
		view.step === "failed"
			? view.recording.recording_id
			: null;
	const polling = view.step === "recording";

	useEffect(() => {
		if (!open || !polling || recordingId === null) return;
		let active = true;
		const timer = setInterval(() => {
			client
				.getRecording(recordingId)
				.then((rec) => {
					if (active) setView(viewOf(rec));
				})
				.catch((error: unknown) => {
					if (active) setFailure(failureOf(error));
				});
		}, LIVE_POLL_MS);
		return () => {
			active = false;
			clearInterval(timer);
		};
	}, [client, open, polling, recordingId]);

	// One call of the person: the answer decides the next screen.
	async function run(call: () => Promise<RecordView | null>): Promise<void> {
		setFailure(null);
		setBusy(true);
		try {
			const next = await call();
			if (next !== null && alive.current) setView(next);
		} catch (error) {
			if (alive.current) setFailure(failureOf(error));
		} finally {
			if (alive.current) setBusy(false);
		}
	}

	const current = (): string => recordingId ?? "";

	// What the recorder of this tab reports. A recorder that ended, by the Stop of the person or on its own (Stop
	// sharing in the browser, a limit, a failed upload), is no longer this tab's: the dialog follows the server's
	// answer, and a dialog opened again finds the take like any other.
	const showBrowser = (state: RecorderState): void => {
		if (state.phase === "ended") browser.current = null;
		if (!alive.current) return;
		setView(browserViewOf(state));
		if (
			state.phase === "ended" &&
			state.recording === null &&
			state.error !== null
		) {
			setFailure(failureOf(state.error));
		}
	};

	return {
		view,
		failure,
		busy,
		placed,
		start: (kind, id, name, fps, extras) =>
			run(async () =>
				viewOf(
					await client.startRecording({
						project_id: projectId,
						source: name === "" ? { kind, id } : { kind, id, name },
						fps,
						...(extras.microphone === ""
							? {}
							: { microphone: extras.microphone }),
						...(extras.systemAudio ? { system_audio: true } : {}),
						...(extras.camera === "" ? {} : { camera: extras.camera }),
						...(extras.events ? { events: true } : {}),
					}),
				),
			),
		stop: () => run(async () => viewOf(await client.stopRecording(current()))),
		// The browser asks the person for the screen on this click; a refusal is the browser's answer, not a failure of
		// the product, and leaves the dialog as it was.
		startBrowser: (choice) =>
			run(async () => {
				const capability = currentCapability();
				if (!capability.ok) return null;
				let handle: BrowserRecording;
				try {
					handle = await startBrowserRecording({
						client,
						projectId,
						mimeType: capability.mimeType,
						container: capability.container,
						choice,
						onChange: (state: RecorderState) => {
							if (browser.current !== null) showBrowser(state);
						},
					});
				} catch (error) {
					if (
						error instanceof DOMException &&
						error.name === "NotAllowedError"
					) {
						return null;
					}
					throw error;
				}
				browser.current = handle;
				return { step: "browser_live", state: handle.state() };
			}),
		stopBrowser: async () => {
			const own = browser.current;
			if (own === null) return;
			setBusy(true);
			try {
				showBrowser(await own.stop());
			} finally {
				if (alive.current) setBusy(false);
			}
		},
		pause: () =>
			run(async () => viewOf(await client.pauseRecording(current()))),
		resume: () =>
			run(async () => viewOf(await client.resumeRecording(current()))),
		confirm: (id) =>
			run(async () => viewOf(await client.confirmRecording(id ?? current()))),
		// "Not now": the recording stays out of the project; the picker lists it the next time, to add or delete.
		dismiss: () =>
			run(async () => {
				await client.dismissRecording(current());
				return { step: "kept" };
			}),
		remove: (id) =>
			run(async () => viewOf(await client.cancelRecording(id ?? current()))),
		place: async () => {
			await run(async () => {
				await client.placeRecording(current());
				if (alive.current) setPlaced(true);
				return null;
			});
		},
		reset: () => {
			setPlaced(false);
			void load();
		},
	};
}

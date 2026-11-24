import { useCallback, useEffect, useRef, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import type { ApiClient } from "../api/client";
import {
	isLive,
	POLL_MS,
	type RecordView,
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
	): Promise<void>;
	stop(): Promise<void>;
	confirm(): Promise<void>;
	dismiss(): Promise<void>;
	remove(): Promise<void>;
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
				if (alive.current) setView(viewOf(waiting));
				return;
			}
			const sources = await client.recordingSources();
			if (alive.current) setView({ step: "pick", sources });
		} catch (error) {
			const reason = unavailableReason(error);
			if (!alive.current) return;
			if (reason !== null) setView({ step: "unavailable", reason });
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
		}, POLL_MS);
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

	return {
		view,
		failure,
		busy,
		placed,
		start: (kind, id, name, fps) =>
			run(async () =>
				viewOf(
					await client.startRecording({
						project_id: projectId,
						source: name === "" ? { kind, id } : { kind, id, name },
						fps,
					}),
				),
			),
		stop: () => run(async () => viewOf(await client.stopRecording(current()))),
		confirm: () =>
			run(async () => viewOf(await client.confirmRecording(current()))),
		// "Not now": the recording stays out of the project, so the dialog has nothing more to show of it.
		dismiss: () =>
			run(async () => {
				await client.dismissRecording(current());
				return { step: "gone" };
			}),
		remove: () =>
			run(async () => viewOf(await client.cancelRecording(current()))),
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

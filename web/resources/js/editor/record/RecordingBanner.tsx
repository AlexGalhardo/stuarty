import { type ReactNode, useEffect, useState } from "react";

import { useT } from "@/lib/i18n";
import type { ApiClient } from "../api/client";
import type { Recording } from "../api/types";
import { useOptionalRuntime } from "../state/EditorContext";
import { capturing, formatClock } from "./recordFlow";

// How often the banner asks whether a capture is on.
export const BANNER_POLL_MS = 2000;

// The recording of the project that is capturing now, asked of the service every couple of seconds. A refusal or a
// missing answer shows nothing new: the banner says what the service last said.
export function useCapturing(
	client: ApiClient | undefined,
	projectId: string | undefined,
): Recording | null {
	const [live, setLive] = useState<Recording | null>(null);
	useEffect(() => {
		if (client === undefined || projectId === undefined) return;
		if (typeof client.listRecordings !== "function") return;
		let active = true;
		const ask = (): void => {
			client
				.listRecordings(projectId)
				.then((list) => {
					if (active) setLive(capturing(list.recordings));
				})
				.catch(() => {});
		};
		ask();
		const timer = setInterval(ask, BANNER_POLL_MS);
		return () => {
			active = false;
			clearInterval(timer);
		};
	}, [client, projectId]);
	return live;
}

// A capture that is on is visible on every page of the editor, not only in the dialog that started it
// (ADR-0051 item 4, SDD-025 section 19.4). The desktop shell adds the system tray; the browser adds its own mark.
export default function RecordingBanner(): ReactNode {
	const t = useT();
	const runtime = useOptionalRuntime();
	const live = useCapturing(runtime?.client, runtime?.projectId);
	if (live === null) return null;
	return (
		<div
			role="status"
			data-recording-banner={live.state}
			className="flex items-center gap-2 border-b border-destructive bg-ed-panel px-3 py-1.5 text-sm font-medium text-destructive"
		>
			<span
				aria-hidden="true"
				className="size-2.5 rounded-full bg-destructive"
			/>
			{live.state === "paused"
				? t("ui.editor.record.banner_paused")
				: t("ui.editor.record.banner")}
			<span className="tabular-nums">{formatClock(live.duration_ms)}</span>
		</div>
	);
}

import { type ReactNode, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { apiTransport, callJson } from "@/lib/api";
import {
	canOpenTeleprompter,
	type DesktopBridge,
	getDesktopBridge,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";
import type { WritingClient } from "../../pages/Write/client";
import Teleprompter from "../teleprompter/Teleprompter";

const TELEPROMPTER_TIMEOUT_MS = 15_000;

function teleprompterClient(): WritingClient {
	return {
		list: (filter) => {
			const query = new URLSearchParams();
			for (const [key, value] of Object.entries(filter)) {
				if (value !== undefined && value !== "") query.set(key, value);
			}
			const text = query.toString();
			return callJson(
				apiTransport,
				{
					method: "GET",
					path: `/texts${text === "" ? "" : `?${text}`}`,
					timeoutMs: TELEPROMPTER_TIMEOUT_MS,
				},
				{ retry: false },
			);
		},
		get: (projectId, itemId, revision) =>
			callJson(
				apiTransport,
				{
					method: "GET",
					path: `/projects/${encodeURIComponent(projectId)}/texts/${encodeURIComponent(itemId)}${
						revision === undefined ? "" : `?revision=${revision}`
					}`,
					timeoutMs: TELEPROMPTER_TIMEOUT_MS,
				},
				{ retry: false },
			),
	} as WritingClient;
}

// The teleprompter of the record dialog (SDD-025 section 20.3): in the desktop shell a button opens it in a window
// of its own, which the system keeps out of a screen capture where it can; on the web it is a panel of this page,
// which a capture of this tab or screen records, and the dialog says so. It reads the script of the project and
// writes nothing.
export default function RecordTeleprompter({
	projectId,
}: {
	projectId: string;
}): ReactNode {
	const t = useT();
	const bridge = getDesktopBridge();
	const client = useMemo(() => teleprompterClient(), []);
	return (
		<TeleprompterControls
			bridge={bridge}
			projectId={projectId}
			panel={(show) =>
				show ? (
					<Teleprompter
						client={client}
						projectId={projectId}
						className="max-h-72"
					/>
				) : null
			}
			label={t("ui.editor.teleprompter.title")}
		/>
	);
}

// The controls and the notes, with the panel drawn by the caller; it runs the bridge only on a click.
export function TeleprompterControls({
	bridge,
	projectId,
	panel,
	label,
}: {
	bridge: DesktopBridge | null;
	projectId: string;
	panel: (show: boolean) => ReactNode;
	label: string;
}): ReactNode {
	const t = useT();
	const [shown, setShown] = useState(false);
	const [failed, setFailed] = useState(false);
	const windowed = canOpenTeleprompter(bridge);

	const toggleWindow = async (): Promise<void> => {
		setFailed(false);
		try {
			if (shown) await bridge?.closeTeleprompter?.();
			else await bridge?.openTeleprompter?.(projectId);
			setShown(!shown);
		} catch {
			setFailed(true);
		}
	};

	return (
		<fieldset
			className="flex flex-col gap-2 rounded-md border p-3"
			data-record-teleprompter=""
		>
			<legend className="px-1 text-sm font-medium">{label}</legend>
			{windowed ? (
				<Button
					type="button"
					variant="outline"
					size="sm"
					className="self-start"
					onClick={toggleWindow}
				>
					{shown
						? t("ui.editor.teleprompter.close_window")
						: t("ui.editor.teleprompter.open_window")}
				</Button>
			) : (
				<label className="flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						checked={shown}
						onChange={(event) => setShown(event.target.checked)}
					/>
					{t("ui.editor.teleprompter.show_here")}
				</label>
			)}
			<p className="text-xs text-muted-foreground">
				{windowed
					? t("ui.editor.teleprompter.hidden_windows")
					: t("ui.editor.teleprompter.captured_web")}
			</p>
			{failed && (
				<p role="alert" className="text-xs text-destructive">
					{t("ui.editor.teleprompter.window_failed")}
				</p>
			)}
			{!windowed && panel(shown)}
		</fieldset>
	);
}

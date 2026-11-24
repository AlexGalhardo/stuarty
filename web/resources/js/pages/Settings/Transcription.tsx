import { Copy } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useState } from "react";

import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { ApiError } from "../../editor/api/errors";
import { HttpTransport } from "../../editor/api/http";
import type { ToolsStatusResult } from "../../editor/api/types";
import {
	DEFAULT_WHISPER_MODEL,
	WHISPER_MODELS,
} from "../../editor/inspector/panels/transcriptModel";
import { installCommand, transcriptionState } from "./installState";
import { SELECT_CLASS, SettingsSection } from "./SettingsSection";

const POLL_EVERY_MS = 1000;

type Phase =
	| { name: "idle" }
	| { name: "command_only" }
	| { name: "failed"; message: string };

// The transcription helper on the settings page (VV-077): whether the helper and a model are
// installed, and a button for a person to install them. The server refuses the install to an agent
// surface; a session it refuses shows the command to run in a terminal instead.
export default function Transcription(): ReactNode {
	const t = useT();
	const [status, setStatus] = useState<ToolsStatusResult | null>(null);
	const [unavailable, setUnavailable] = useState(false);
	const [model, setModel] = useState(DEFAULT_WHISPER_MODEL);
	const [phase, setPhase] = useState<Phase>({ name: "idle" });

	const refresh = useCallback(async (): Promise<void> => {
		try {
			setStatus(await createClient(new HttpTransport("/api/v1")).getTools());
			setUnavailable(false);
		} catch {
			setUnavailable(true);
		}
	}, []);

	useEffect(() => {
		void refresh();
	}, [refresh]);

	const running = status?.state === "running";
	useEffect(() => {
		if (!running) return;
		const timer = setInterval(() => void refresh(), POLL_EVERY_MS);
		return () => clearInterval(timer);
	}, [running, refresh]);

	async function install(): Promise<void> {
		setPhase({ name: "idle" });
		try {
			await createClient(new HttpTransport("/api/v1")).installTool({
				name: "whisper",
				model,
			});
			await refresh();
		} catch (error) {
			if (error instanceof ApiError && error.code === "E_FORBIDDEN") {
				setPhase({ name: "command_only" });
				return;
			}
			setPhase({
				name: "failed",
				message: error instanceof ApiError ? error.message : String(error),
			});
		}
	}

	const state = transcriptionState(status?.installed ?? []);
	const ready = state.helper && state.models.length > 0;
	const percent =
		status !== null && status.total_bytes > 0
			? Math.min(
					100,
					Math.round((status.done_bytes / status.total_bytes) * 100),
				)
			: 0;
	const command = installCommand(model);

	return (
		<SettingsSection
			id="settings-transcription"
			title={t("ui.settings.transcription.title")}
			description={t("ui.settings.transcription.intro")}
		>
			{unavailable && (
				<p role="alert" className="text-sm text-danger-text">
					{t("ui.settings.unavailable")}
				</p>
			)}
			{status !== null && (
				<p data-testid="transcription-state">
					<StatusBadge status={ready ? "success" : "neutral"}>
						{ready
							? t("ui.settings.transcription.ready", {
									models: state.models.join(", "),
								})
							: t("ui.settings.transcription.missing")}
					</StatusBadge>
				</p>
			)}
			{status?.state === "failed" && status.error !== undefined && (
				<p role="alert" className="text-sm text-danger-text">
					<UntrustedText>{status.error.message}</UntrustedText>
				</p>
			)}
			{running && (
				<div>
					<Progress
						value={percent}
						aria-label={t("ui.settings.transcription.progress")}
						className="h-2"
					/>
					<p className="mt-1 text-xs [font-variant-numeric:tabular-nums]">
						{percent}%
					</p>
				</div>
			)}
			{status !== null && !running && (
				<div className="flex items-end gap-2">
					<div className="flex flex-col gap-1">
						<Label htmlFor="settings-whisper-model">
							{t("ui.settings.transcription.model")}
						</Label>
						<select
							id="settings-whisper-model"
							value={model}
							className={SELECT_CLASS}
							onChange={(event) => setModel(event.target.value)}
						>
							{WHISPER_MODELS.map((name) => (
								<option key={name} value={name}>
									{name}
								</option>
							))}
						</select>
					</div>
					<Button
						type="button"
						variant="outline"
						disabled={state.helper && state.models.includes(model)}
						onClick={() => void install()}
					>
						{t("ui.settings.transcription.install")}
					</Button>
				</div>
			)}
			{phase.name === "failed" && (
				<p role="alert" className="text-sm text-danger-text">
					<UntrustedText>{phase.message}</UntrustedText>
				</p>
			)}
			{phase.name === "command_only" && (
				<div className="flex flex-col gap-1">
					<p role="status" className="text-sm text-text-muted">
						{t("ui.settings.transcription.forbidden")}
					</p>
					<p className="flex items-center gap-1">
						<input
							readOnly
							value={command}
							aria-label={t("ui.settings.transcription.command")}
							className="h-8 min-w-0 flex-1 rounded-lg border border-border-strong bg-surface-sunken px-2 font-mono text-sm text-text max-md:h-11"
							onFocus={(event) => event.currentTarget.select()}
						/>
						<Button
							type="button"
							variant="ghost"
							size="icon-sm"
							aria-label={t("ui.settings.transcription.copy_command")}
							onClick={() => void navigator.clipboard.writeText(command)}
						>
							<Copy aria-hidden="true" />
						</Button>
					</p>
				</div>
			)}
		</SettingsSection>
	);
}

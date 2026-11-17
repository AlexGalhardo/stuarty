import { Copy } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useState } from "react";

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
		<section
			aria-labelledby="settings-transcription"
			className="flex flex-col gap-3"
		>
			<h2 id="settings-transcription" className="text-lg font-semibold">
				{t("ui.settings.transcription.title")}
			</h2>
			<p className="text-sm text-muted-foreground">
				{t("ui.settings.transcription.intro")}
			</p>
			{unavailable && (
				<p role="alert" className="text-sm text-destructive">
					{t("ui.settings.unavailable")}
				</p>
			)}
			{status !== null && (
				<p data-testid="transcription-state" className="text-sm">
					{ready
						? t("ui.settings.transcription.ready", {
								models: state.models.join(", "),
							})
						: t("ui.settings.transcription.missing")}
				</p>
			)}
			{status?.state === "failed" && status.error !== undefined && (
				<p role="alert" className="text-sm text-destructive">
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
							className="h-8 rounded-lg border border-input bg-transparent px-2.5 text-sm dark:bg-input/30"
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
				<p role="alert" className="text-sm text-destructive">
					<UntrustedText>{phase.message}</UntrustedText>
				</p>
			)}
			{phase.name === "command_only" && (
				<div className="flex flex-col gap-1">
					<p role="status" className="text-sm text-muted-foreground">
						{t("ui.settings.transcription.forbidden")}
					</p>
					<p className="flex items-center gap-1">
						<input
							readOnly
							value={command}
							aria-label={t("ui.settings.transcription.command")}
							className="h-8 min-w-0 flex-1 rounded-md border bg-transparent px-2 font-mono text-xs"
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
		</section>
	);
}

import { type ReactNode, useState } from "react";

import { type Failure, FailureNote } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { RecordingSourceInfo } from "../api/types";
import {
	DEFAULT_FPS,
	FPS_CHOICES,
	formatClock,
	type RecordView,
	unavailableKey,
} from "./recordFlow";
import type { RecordFlow } from "./useRecordFlow";

// The screens of the record dialog (SDD-023 section 12). It draws what the service said and sends the commands of
// the person; the state of a recording, the elapsed time and what a capture may do are the service's.
export default function RecordPanel({
	flow,
	onClose,
}: {
	flow: RecordFlow;
	onClose: () => void;
}): ReactNode {
	return (
		<div className="flex flex-col gap-3" data-record-step={flow.view.step}>
			<Step view={flow.view} flow={flow} onClose={onClose} />
			<FailureNote failure={flow.failure as Failure | null} />
		</div>
	);
}

function Step({
	view,
	flow,
	onClose,
}: {
	view: RecordView;
	flow: RecordFlow;
	onClose: () => void;
}): ReactNode {
	const t = useT();
	switch (view.step) {
		case "loading":
			return (
				<p role="status" aria-busy="true" className="text-sm">
					{t("ui.editor.record.loading")}
				</p>
			);
		case "unavailable":
			return (
				<>
					<p role="status" className="text-sm" data-record-unavailable="">
						{t(unavailableKey(view.reason))}
					</p>
					<Button type="button" variant="outline" onClick={onClose}>
						{t("ui.editor.record.close")}
					</Button>
				</>
			);
		case "pick":
			return (
				<Picker
					displays={view.sources.displays}
					windows={view.sources.windows}
					busy={flow.busy}
					onStart={(kind, id, name, fps) =>
						void flow.start(kind, id, name, fps)
					}
				/>
			);
		case "recording":
			return (
				<>
					<p
						className="flex items-center gap-2 text-sm font-medium"
						role="status"
					>
						<span
							aria-hidden="true"
							className="size-2.5 rounded-full bg-destructive"
						/>
						{view.recording.state === "finishing"
							? t("ui.editor.record.finishing")
							: t("ui.editor.record.recording")}
						<span data-record-clock="" className="tabular-nums">
							{t("ui.editor.record.elapsed", {
								time: formatClock(view.recording.duration_ms),
							})}
						</span>
					</p>
					<Button
						type="button"
						variant="destructive"
						disabled={flow.busy || view.recording.state !== "recording"}
						data-record-stop=""
						onClick={() => void flow.stop()}
					>
						{t("ui.editor.record.stop")}
					</Button>
				</>
			);
		case "review":
			return (
				<>
					<h3 className="text-sm font-medium">
						{t("ui.editor.record.review_title")}
					</h3>
					<p className="text-sm tabular-nums">
						{t("ui.editor.record.review_length", {
							time: formatClock(view.recording.duration_ms),
						})}
					</p>
					{view.recording.dropped > 0 && (
						<p className="text-xs text-muted-foreground">
							{t("ui.editor.record.review_dropped", {
								count: view.recording.dropped,
							})}
						</p>
					)}
					<p className="text-sm" data-record-notice="">
						{t("ui.editor.record.review_notice")}
					</p>
					<div className="flex flex-wrap gap-2">
						<Button
							type="button"
							disabled={flow.busy}
							data-record-add=""
							onClick={() => void flow.confirm()}
						>
							{t("ui.editor.record.add")}
						</Button>
						<Button
							type="button"
							variant="outline"
							disabled={flow.busy}
							data-record-not-now=""
							onClick={() => void flow.dismiss()}
						>
							{t("ui.editor.record.not_now")}
						</Button>
						<Button
							type="button"
							variant="ghost"
							disabled={flow.busy}
							data-record-delete=""
							onClick={() => void flow.remove()}
						>
							{t("ui.editor.record.delete")}
						</Button>
					</div>
				</>
			);
		case "added":
			return (
				<>
					<p role="status" className="text-sm">
						{flow.placed
							? t("ui.editor.record.placed")
							: t("ui.editor.record.added")}
					</p>
					<div className="flex flex-wrap gap-2">
						{!flow.placed && (
							<Button
								type="button"
								disabled={flow.busy}
								data-record-place=""
								onClick={() => void flow.place()}
							>
								{t("ui.editor.record.place")}
							</Button>
						)}
						<Button type="button" variant="outline" onClick={onClose}>
							{t("ui.editor.record.close")}
						</Button>
					</div>
				</>
			);
		case "failed":
			return (
				<>
					<p role="alert" className="text-sm font-medium text-destructive">
						{t("ui.editor.record.failed")}
					</p>
					{view.recording.error !== undefined && (
						<p lang="en" className="text-xs">
							<UntrustedText>{view.recording.error.message}</UntrustedText>
						</p>
					)}
					<div className="flex gap-2">
						<Button type="button" onClick={flow.reset}>
							{t("ui.editor.record.again")}
						</Button>
						<Button type="button" variant="outline" onClick={onClose}>
							{t("ui.editor.record.close")}
						</Button>
					</div>
				</>
			);
		case "gone":
			return (
				<>
					<p role="status" className="text-sm">
						{t("ui.editor.record.gone")}
					</p>
					<div className="flex gap-2">
						<Button type="button" onClick={flow.reset}>
							{t("ui.editor.record.again")}
						</Button>
						<Button type="button" variant="outline" onClick={onClose}>
							{t("ui.editor.record.close")}
						</Button>
					</div>
				</>
			);
	}
}

interface Choice {
	kind: "display" | "window";
	source: RecordingSourceInfo;
}

function Picker({
	displays,
	windows,
	busy,
	onStart,
}: {
	displays: RecordingSourceInfo[];
	windows: RecordingSourceInfo[];
	busy: boolean;
	onStart: (
		kind: "display" | "window",
		id: string,
		name: string,
		fps: number,
	) => void;
}): ReactNode {
	const t = useT();
	const choices: Choice[] = [
		...displays.map((source): Choice => ({ kind: "display", source })),
		...windows.map((source): Choice => ({ kind: "window", source })),
	];
	const [picked, setPicked] = useState<string>(
		choices.length > 0 ? `${choices[0]?.kind}:${choices[0]?.source.id}` : "",
	);
	const [fps, setFps] = useState<number>(DEFAULT_FPS);

	if (choices.length === 0) {
		return <p className="text-sm">{t("ui.editor.record.none")}</p>;
	}

	const chosen = choices.find((c) => `${c.kind}:${c.source.id}` === picked);

	return (
		<form
			className="flex flex-col gap-3"
			onSubmit={(event) => {
				event.preventDefault();
				if (chosen === undefined) return;
				onStart(
					chosen.kind,
					chosen.source.id,
					chosen.kind === "window" ? chosen.source.name : "",
					fps,
				);
			}}
		>
			{(
				[
					["display", displays, "ui.editor.record.displays"],
					["window", windows, "ui.editor.record.windows"],
				] as const
			).map(([kind, list, label]) =>
				list.length === 0 ? null : (
					<fieldset key={kind} className="flex flex-col gap-1">
						<legend className="text-xs font-medium text-muted-foreground uppercase">
							{t(label)}
						</legend>
						{list.map((source) => {
							const value = `${kind}:${source.id}`;
							return (
								<label key={value} className="flex items-center gap-2 text-sm">
									<input
										type="radio"
										name="record-source"
										value={value}
										checked={picked === value}
										onChange={() => setPicked(value)}
									/>
									<span className="min-w-0 truncate">
										<UntrustedText>{source.name}</UntrustedText>
									</span>
									<span className="text-xs text-muted-foreground tabular-nums">
										{`${source.width}×${source.height}`}
									</span>
								</label>
							);
						})}
					</fieldset>
				),
			)}
			<div className="flex items-center gap-2">
				<Label htmlFor="record-fps">{t("ui.editor.record.fps")}</Label>
				<select
					id="record-fps"
					className="h-8 rounded-md border bg-background px-2 text-sm"
					value={fps}
					onChange={(event) => setFps(Number(event.target.value))}
				>
					{FPS_CHOICES.map((n) => (
						<option key={n} value={n}>
							{n}
						</option>
					))}
				</select>
			</div>
			<Button
				type="submit"
				disabled={busy || chosen === undefined}
				data-record-start=""
			>
				{busy ? t("ui.editor.record.starting") : t("ui.editor.record.start")}
			</Button>
		</form>
	);
}

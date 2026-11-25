import { FileText, Scissors, X } from "lucide-react";
import { type ReactNode, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { useT } from "@/lib/i18n";
import { formatTime, fpsOf } from "../commands/units";
import { isSelected } from "../inspector/panels/transcriptPanelModel";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { selectFps } from "../state/selectors";
import { transcriptLines, wordKinds } from "./transcriptLines";
import { useTranscriptState } from "./useTranscript";

const KIND_CLASS = {
	filler: "border-warning bg-warning-soft text-warning-text",
	repetition: "border-danger-text bg-danger-soft text-danger-text",
} as const;

// The transcript of the edit by text screen (final design 26): the words of the clip's asset, a line for each
// sentence with its time, in the wide region at the left of the preview. A word is a button; the filler words and
// repetitions the service found are boxed, and the chips choose which kinds are boxed. Struck-through words are
// already cut. Deleting sends `transcript.delete_range` with word indices; the core finds the media to cut.
export default function TranscriptStage(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const state = useTranscriptState();
	const fps = fpsOf(useDocumentState(selectFps));
	const name = useDocumentState((s) => {
		const asset =
			state === null ? undefined : s.document?.assets[state.assetId];
		return asset?.name;
	});
	const [kinds, setKinds] = useState({ filler: true, repetition: true });
	if (state === null) return null;
	const marks = wordKinds(state.fillers?.suggestions ?? []);
	const lines = transcriptLines(state.view?.words ?? []);
	return (
		<section
			aria-label={t("ui.editor.panels.captions")}
			data-stage="transcript"
			className="flex min-h-0 min-w-0 flex-1 flex-col bg-ed-panel"
		>
			<div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-3 text-sm font-semibold">
				<FileText aria-hidden="true" className="size-4 text-muted-foreground" />
				<span className="min-w-0 flex-1 truncate">
					{t("ui.editor.transcript_stage.title")}
					{name !== undefined && (
						<>
							: <UntrustedText>{name}</UntrustedText>
						</>
					)}
				</span>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					onClick={() => runtime.uiWriter.showStage(null)}
				>
					<X aria-hidden="true" />
					{t("ui.editor.stage.close")}
				</Button>
			</div>
			<div className="flex shrink-0 items-center gap-2 border-b border-border px-3 py-2">
				{(["filler", "repetition"] as const).map((kind) => (
					<button
						key={kind}
						type="button"
						aria-pressed={kinds[kind]}
						data-kind-chip={kind}
						className={`rounded-md border px-2 py-0.5 text-xs ${KIND_CLASS[kind]} ${kinds[kind] ? "" : "opacity-50"}`}
						onClick={() => setKinds({ ...kinds, [kind]: !kinds[kind] })}
					>
						{kind === "filler"
							? t("ui.editor.transcript.kind_filler")
							: t("ui.editor.transcript.kind_repetition")}
					</button>
				))}
				<span className="ml-auto text-xs text-muted-foreground">
					{t("ui.editor.transcript_stage.select_hint")}
				</span>
			</div>
			<div className="min-h-0 flex-1 overflow-auto px-3 py-3">
				{state.problem !== null && (
					<p role="alert" className="text-xs text-destructive">
						{state.problem}
					</p>
				)}
				{state.view !== null && (
					<>
						<p className="sr-only">{t("ui.editor.transcript.hint")}</p>
						<fieldset
							dir="auto"
							className="m-0 flex min-w-0 flex-col gap-3 border-0 p-0 text-sm"
						>
							<legend className="sr-only">
								{t("ui.editor.transcript.words")}
							</legend>
							{lines.map((line) => (
								<div key={line.start} className="flex gap-4">
									<span className="w-12 shrink-0 pt-1 font-mono text-xs text-muted-foreground tabular-nums">
										{formatTime(line.start, fps, "timecode").slice(0, 5)}
									</span>
									<div className="flex min-w-0 flex-wrap gap-x-1 gap-y-1.5 leading-7">
										{line.words.map((word) => {
											const cut = word.ranges.length === 0;
											const on = isSelected(state.selection, word.index);
											const mark = marks.get(word.index);
											const boxed = mark !== undefined && kinds[mark];
											return (
												<button
													key={word.index}
													type="button"
													data-word-index={word.index}
													data-cut={cut ? "true" : undefined}
													data-mark={boxed ? mark : undefined}
													aria-pressed={on}
													className={`rounded-md border px-1 hover:bg-surface-hover ${boxed ? KIND_CLASS[mark] : "border-transparent"} ${on ? "bg-surface-selected text-foreground" : ""} ${cut ? "text-muted-foreground line-through" : ""}`}
													onClick={(event) =>
														state.selectWord(word.index, event.shiftKey)
													}
												>
													<UntrustedText>{word.text}</UntrustedText>
												</button>
											);
										})}
									</div>
								</div>
							))}
						</fieldset>
					</>
				)}
			</div>
			<div className="flex shrink-0 flex-col gap-2 border-t border-border px-3 py-2">
				<div className="flex flex-wrap items-center gap-2">
					<Button
						type="button"
						size="sm"
						disabled={!state.canEdit || state.busy || state.selectedCount === 0}
						onClick={() => void state.deleteSelected()}
					>
						<Scissors aria-hidden="true" />
						{t("ui.editor.transcript.delete_selected")}
					</Button>
					<span className="text-xs text-muted-foreground" aria-live="polite">
						{t("ui.editor.transcript.selected", {
							count: state.selectedCount,
						})}
					</span>
					<span className="ml-auto flex items-center gap-2 text-xs">
						<Checkbox
							id={state.snapId}
							checked={state.snap}
							onCheckedChange={(on) => state.setSnap(on === true)}
						/>
						<label htmlFor={state.snapId}>
							{t("ui.editor.transcript.snap")}
						</label>
					</span>
				</div>
				{state.snapped !== null &&
					state.snapped.revision === state.revision && (
						<p
							className="text-xs text-muted-foreground"
							data-snap-result
							aria-live="polite"
						>
							{state.snapped.summary.missing
								? t("ui.editor.transcript.snap_missing")
								: t("ui.editor.transcript.snap_result", {
										moved: state.snapped.summary.moved,
										inside: state.snapped.summary.inside,
										none: state.snapped.summary.none,
									})}
						</p>
					)}
			</div>
		</section>
	);
}

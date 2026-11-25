import { ListChecks, Scissors, ShieldCheck, Undo2 } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { useT } from "@/lib/i18n";
import { formatSeconds } from "../commands/units";
import UndoRedo from "../history/UndoRedo";
import { ToolLine } from "../inspector/Section";
import { useTranscriptState } from "./useTranscript";

export const TRANSCRIPT_REVIEW_ICON = ListChecks;

// The review at the right of the edit by text screen (final design 26): the filler words and repetitions the
// service suggests, each with a check, how many are checked, and the button that removes the checked ones.
// Nothing is removed until the person confirms (SDD-023 section 6): the suggestions change no timeline.
export default function TranscriptReview(): ReactNode {
	const t = useT();
	const state = useTranscriptState();
	if (state === null) return null;
	const { fillers } = state;
	return (
		<div className="flex flex-col gap-3" data-review="transcript">
			<ToolLine name="TranscriptDetectFillers" />
			<p className="flex items-start gap-2 text-xs text-muted-foreground">
				<ShieldCheck aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
				{t("ui.editor.transcript_review.notice")}
			</p>
			{state.problem !== null && (
				<p role="alert" className="text-xs text-destructive">
					{state.problem}
				</p>
			)}
			<div className="flex flex-col gap-2 border-b border-border pb-3">
				<p
					className="flex items-center justify-between text-sm"
					aria-live="polite"
				>
					<span className="text-muted-foreground">
						{t("ui.editor.transcript_review.selected")}
					</span>
					<span className="font-medium tabular-nums">
						{t("ui.editor.transcript.selected", {
							count: state.selectedCount,
						})}
					</span>
				</p>
				<Button
					type="button"
					variant="outline"
					disabled={!state.canEdit || state.busy || state.selectedCount === 0}
					onClick={() => void state.deleteSelected()}
				>
					<Scissors aria-hidden="true" />
					{t("ui.editor.transcript.delete_selected")}
				</Button>
				<span className="flex items-center gap-2 text-xs">
					<Checkbox
						id={state.snapId}
						checked={state.snap}
						onCheckedChange={(on) => state.setSnap(on === true)}
					/>
					<label htmlFor={state.snapId}>{t("ui.editor.transcript.snap")}</label>
				</span>
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
			{fillers === null ? (
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={state.busy}
					onClick={() => void state.find()}
				>
					{t("ui.editor.transcript.find")}
				</Button>
			) : (
				<div className="flex flex-col gap-3" data-fillers>
					{fillers.suggestions.length === 0 ? (
						<p className="text-xs text-muted-foreground">
							{fillers.filler_rules
								? t("ui.editor.transcript.none_found", {
										language: fillers.language,
									})
								: t("ui.editor.transcript.none_found_repetitions", {
										language: fillers.language,
									})}
						</p>
					) : (
						<>
							<ul className="flex flex-col">
								{fillers.suggestions.map((suggestion, i) => (
									<li
										key={`${suggestion.from_word}-${suggestion.to_word}`}
										className="grid min-h-11 grid-cols-[20px_1fr_auto_24px] items-center gap-2 border-b border-border text-[15px] last:border-b-0"
									>
										<Checkbox
											checked={state.checked.has(i)}
											onCheckedChange={() => state.toggle(i)}
											aria-label={suggestion.text}
										/>
										<span className="min-w-0 flex-1 truncate">
											{suggestion.kind === "filler"
												? t("ui.editor.transcript.kind_filler")
												: t("ui.editor.transcript_review.repetition_at", {
														time: formatSeconds(
															suggestion.start_ms / 1000,
															"timecode",
														).slice(0, 5),
													})}
											{suggestion.strength === "weak"
												? ` (${t("ui.editor.transcript.weak")})`
												: ""}
										</span>
										<span
											className={`max-w-28 truncate rounded-md border px-2 text-xs ${suggestion.kind === "filler" ? "border-warning bg-warning-soft text-warning-text" : "border-danger-text bg-danger-soft text-danger-text"}`}
										>
											<UntrustedText>{suggestion.text}</UntrustedText>
										</span>
										<span className="text-end font-mono text-xs text-muted-foreground">
											{suggestion.to_word - suggestion.from_word + 1}
										</span>
									</li>
								))}
							</ul>
							<p className="flex items-center justify-between border-t border-border pt-3 text-sm">
								<span className="text-muted-foreground">
									{t("ui.editor.transcript_review.selected")}
								</span>
								<span className="font-medium tabular-nums">
									{t("ui.editor.transcript_review.of", {
										count: state.checked.size,
										total: fillers.suggestions.length,
									})}
								</span>
							</p>
							<Button
								type="button"
								className="min-h-10"
								disabled={
									!state.canEdit || state.busy || state.checked.size === 0
								}
								onClick={() => void state.removeChecked()}
							>
								<Scissors aria-hidden="true" />
								{t("ui.editor.transcript.remove_checked", {
									count: state.checked.size,
								})}
							</Button>
							<Button type="button" variant="ghost" onClick={state.keepAll}>
								{t("ui.editor.transcript_review.keep_all")}
							</Button>
						</>
					)}
				</div>
			)}
			<div className="flex gap-1">
				<UndoRedo />
			</div>
			<p className="flex items-start gap-2 text-xs text-muted-foreground">
				<Undo2 aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
				{t("ui.editor.transcript_review.undo_note")}
			</p>
		</div>
	);
}

import { type ReactNode, useEffect, useRef, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { useT } from "@/lib/i18n";
import { ApiError } from "../../api/errors";
import type {
	TranscriptFillersResult,
	TranscriptViewResult,
} from "../../api/types";
import { transcriptDeleteOperations } from "../../commands/builders";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import { errorHeadline } from "../../ui/errorHeadline";
import Section from "../Section";
import {
	checkedRanges,
	extendSelection,
	initialChecked,
	isSelected,
	selectedRange,
	toggled,
	type WordSelection,
} from "./transcriptPanelModel";

// The Transcript section of SDD-023 section 6: the words of the clip's asset, struck through where the timeline
// no longer shows them. Deleting the selected words sends `transcript.delete_range`; "Find fillers" asks the service
// for suggestions and removes nothing until the person checks some and confirms. The panel draws what the service
// says and sends word indices: the core finds the media to cut.
export default function TranscriptPanel({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const actions = useActions();
	const canEdit = useCanEdit();
	const assetId = useDocumentState((s) => s.document?.items[itemId]?.asset);
	const hasAudio = useDocumentState((s) => {
		const asset =
			assetId === undefined ? undefined : s.document?.assets[assetId];
		return asset?.kind === "video" && asset.has_audio !== false;
	});
	const revision = useDocumentState((s) => s.revision);
	const [view, setView] = useState<TranscriptViewResult | null>(null);
	const [problem, setProblem] = useState<string | null>(null);
	const [selection, setSelection] = useState<WordSelection | null>(null);
	const [fillers, setFillers] = useState<TranscriptFillersResult | null>(null);
	const [checked, setChecked] = useState<ReadonlySet<number>>(new Set());
	const [busy, setBusy] = useState(false);
	// `t` is a new function on every render: the effect below must not depend on it, or it would read the
	// transcript again on every render and clear the selection.
	const translate = useRef(t);
	translate.current = t;

	// The view is recomputed by the service after every edit, so it is read again when the revision moves.
	// biome-ignore lint/correctness/useExhaustiveDependencies: revision is the trigger
	useEffect(() => {
		if (!hasAudio || assetId === undefined) return;
		let current = true;
		runtime.client
			.getTranscript(runtime.projectId, assetId)
			.then((result) => {
				if (!current) return;
				setView(result);
				setProblem(null);
			})
			.catch((error: unknown) => {
				if (!current) return;
				setView(null);
				setProblem(
					error instanceof ApiError
						? `${errorHeadline(translate.current, error.code)} ${error.hint ?? ""}`.trim()
						: translate.current("ui.editor.transcript.failed"),
				);
			});
		setSelection(null);
		setFillers(null);
		return () => {
			current = false;
		};
	}, [runtime, assetId, hasAudio, revision]);

	if (!hasAudio || assetId === undefined) return null;

	const range = selectedRange(selection);
	const count = range === null ? 0 : range.to - range.from + 1;

	async function deleteSelected(): Promise<void> {
		if (range === null || assetId === undefined) return;
		setBusy(true);
		try {
			await actions.runOperations(transcriptDeleteOperations(assetId, [range]));
		} finally {
			setBusy(false);
		}
	}

	async function find(): Promise<void> {
		if (assetId === undefined) return;
		setBusy(true);
		try {
			const found = await runtime.client.getTranscriptFillers(
				runtime.projectId,
				assetId,
			);
			setFillers(found);
			setChecked(initialChecked(found.suggestions));
			setProblem(null);
		} catch (error) {
			setProblem(
				error instanceof ApiError
					? `${errorHeadline(t, error.code)} ${error.hint ?? ""}`.trim()
					: t("ui.editor.transcript.failed"),
			);
		} finally {
			setBusy(false);
		}
	}

	async function removeChecked(): Promise<void> {
		if (fillers === null || assetId === undefined) return;
		const ranges = checkedRanges(fillers.suggestions, checked);
		if (ranges.length === 0) return;
		setBusy(true);
		try {
			await actions.runOperations(transcriptDeleteOperations(assetId, ranges));
		} finally {
			setBusy(false);
		}
	}

	return (
		<Section title={t("ui.editor.transcript.title")} name="transcript">
			{problem !== null && (
				<p role="alert" className="text-xs text-destructive">
					{problem}
				</p>
			)}
			{view !== null && (
				<>
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.transcript.hint")}
					</p>
					<fieldset
						dir="auto"
						className="m-0 flex max-h-64 min-w-0 flex-wrap gap-x-1 gap-y-0.5 overflow-y-auto border-0 p-0 text-sm"
					>
						<legend className="sr-only">
							{t("ui.editor.transcript.words")}
						</legend>
						{view.words.map((word) => {
							const cut = word.ranges.length === 0;
							const on = isSelected(selection, word.index);
							return (
								<button
									key={word.index}
									type="button"
									data-word-index={word.index}
									data-cut={cut ? "true" : undefined}
									aria-pressed={on}
									className={`rounded-sm px-0.5 ${on ? "bg-accent text-accent-foreground" : ""} ${cut ? "text-muted-foreground line-through" : ""}`}
									onClick={(event) =>
										setSelection((was) =>
											extendSelection(was, word.index, event.shiftKey),
										)
									}
								>
									<UntrustedText>{word.text}</UntrustedText>
								</button>
							);
						})}
					</fieldset>
					<div className="flex flex-wrap items-center gap-2">
						<Button
							type="button"
							variant="outline"
							size="sm"
							disabled={!canEdit || busy || range === null}
							onClick={() => void deleteSelected()}
						>
							{t("ui.editor.transcript.delete_selected")}
						</Button>
						<span className="text-xs text-muted-foreground" aria-live="polite">
							{t("ui.editor.transcript.selected", { count })}
						</span>
					</div>
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={busy}
						onClick={() => void find()}
					>
						{t("ui.editor.transcript.find")}
					</Button>
				</>
			)}
			{fillers !== null && (
				<div className="flex flex-col gap-1" data-fillers>
					{fillers.suggestions.length === 0 ? (
						<p className="text-xs text-muted-foreground">
							{t("ui.editor.transcript.none_found", { rule: fillers.rule })}
						</p>
					) : (
						<>
							<p className="text-xs text-muted-foreground">
								{t("ui.editor.transcript.review")}
							</p>
							<ul className="flex flex-col gap-1">
								{fillers.suggestions.map((suggestion, i) => (
									<li
										key={`${suggestion.from_word}-${suggestion.to_word}`}
										className="flex items-center gap-2 text-sm"
									>
										<Checkbox
											checked={checked.has(i)}
											onCheckedChange={() =>
												setChecked((was) => toggled(was, i))
											}
											aria-label={suggestion.text}
										/>
										<UntrustedText>{suggestion.text}</UntrustedText>
										<span className="text-xs text-muted-foreground">
											{suggestion.kind === "filler"
												? t("ui.editor.transcript.kind_filler")
												: t("ui.editor.transcript.kind_repetition")}
											{suggestion.strength === "weak"
												? ` (${t("ui.editor.transcript.weak")})`
												: ""}
										</span>
									</li>
								))}
							</ul>
							<Button
								type="button"
								variant="outline"
								size="sm"
								disabled={!canEdit || busy || checked.size === 0}
								onClick={() => void removeChecked()}
							>
								{t("ui.editor.transcript.remove_checked", {
									count: checked.size,
								})}
							</Button>
						</>
					)}
				</div>
			)}
		</Section>
	);
}

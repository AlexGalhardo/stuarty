import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useLocale, useT } from "@/lib/i18n";
import {
	bulkEditWords,
	captionBreaksOperations,
	captionEditWordOperations,
	captionWordsOperations,
	toggleBreak,
	withWordField,
} from "../../commands/builders";
import {
	formatTime,
	fpsOf,
	localizeDecimalInput,
	normalizeTimeInput,
	type TimeDisplay,
} from "../../commands/units";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import { readTimeUnit } from "../../state/prefs";
import type { CommitResult } from "../fields/commitField";
import { FieldMessage } from "../fields/FieldFrame";
import TextAreaField from "../fields/TextAreaField";
import { useCommitField } from "../fields/useCommitField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const DISPLAY: TimeDisplay = "timecode";

// One cell of the word table: Enter or blur commits, Esc discards, like every
// field of the inspector (SDD-012 section 12.3).
function Cell({
	label,
	stored,
	commit,
	disabled,
	mono = false,
}: {
	label: string;
	stored: string;
	commit(draft: string): Promise<CommitResult>;
	disabled: boolean;
	mono?: boolean;
}): ReactNode {
	const binding = useCommitField(stored, commit);
	return (
		<div className="flex min-w-0 flex-col gap-0.5">
			<Input
				{...binding.input}
				aria-label={label}
				disabled={disabled}
				autoComplete="off"
				spellCheck={false}
				className={mono ? "h-7 px-1 font-mono text-xs" : "h-7 px-1 text-xs"}
			/>
			<FieldMessage id={`${label}-message`} state={binding} />
		</div>
	);
}

function Breaks({
	label,
	checked,
	disabled,
	onChange,
}: {
	label: string;
	checked: boolean;
	disabled: boolean;
	onChange(): void;
}): ReactNode {
	return (
		<input
			type="checkbox"
			aria-label={label}
			checked={checked}
			disabled={disabled}
			className="size-4 accent-primary"
			onChange={onChange}
		/>
	);
}

// The Caption words section of SDD-012 section 12.1: index, text, start,
// duration and the line break of each word, a split before a word, and the bulk
// edit. Every change is one command; the list is the read model's.
export default function CaptionWords({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const actions = useActions();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const rate = useDocumentState((s) => s.document?.settings.fps);
	if (item === undefined) return null;
	const fps = fpsOf(rate);
	const words = item.words ?? [];
	const breaks = item.breaks ?? [];

	const timeCell =
		(index: number, field: "start" | "duration") =>
		async (draft: string): Promise<CommitResult> => {
			const time = normalizeTimeInput(
				localizeDecimalInput(draft, locale),
				readTimeUnit(),
			);
			if (!time.ok) {
				return { ok: false, error: { code: "E_INVALID_TIME", message: "" } };
			}
			return send(
				captionWordsOperations(
					itemId,
					withWordField(words, index, field, time.value),
				),
			);
		};

	return (
		<Section title={t("ui.editor.caption_words.title")}>
			<TextAreaField
				label={t("ui.editor.caption_words.bulk")}
				stored={words.map((word) => word.text).join(" ")}
				disabled={!canEdit}
				commit={async (text) => {
					const next = bulkEditWords(words, text);
					if (next === null) {
						return {
							ok: false,
							error: { code: "E_VALIDATION", message: "" },
						};
					}
					return send(captionWordsOperations(itemId, next));
				}}
			/>
			<div className="overflow-x-auto">
				<table className="w-full border-separate border-spacing-y-1 text-xs">
					<thead>
						<tr className="text-left text-muted-foreground">
							<th scope="col">{t("ui.editor.caption_words.index")}</th>
							<th scope="col">{t("ui.editor.caption_words.text")}</th>
							<th scope="col">{t("ui.editor.caption_words.start")}</th>
							<th scope="col">{t("ui.editor.caption_words.duration")}</th>
							<th scope="col">{t("ui.editor.caption_words.break")}</th>
							<th scope="col">
								<span className="sr-only">
									{t("ui.editor.caption_words.actions")}
								</span>
							</th>
						</tr>
					</thead>
					<tbody>
						{words.map((word, index) => {
							const n = index + 1;
							return (
								<tr
									// biome-ignore lint/suspicious/noArrayIndexKey: a word has no id; its index is what the commands address.
									key={index}
									data-word-row={index}
									className="align-top"
								>
									<th scope="row" className="pr-1 pt-1.5 font-normal">
										{n}
									</th>
									<td className="pr-1">
										<Cell
											label={t("ui.editor.caption_words.text_of", { n })}
											stored={word.text}
											disabled={!canEdit}
											commit={(draft) =>
												send(
													captionEditWordOperations(
														itemId,
														index,
														draft.trim(),
													),
												)
											}
										/>
									</td>
									<td className="pr-1">
										<Cell
											mono
											label={t("ui.editor.caption_words.start_of", { n })}
											stored={formatTime(word.start, fps, DISPLAY)}
											disabled={!canEdit}
											commit={timeCell(index, "start")}
										/>
									</td>
									<td className="pr-1">
										<Cell
											mono
											label={t("ui.editor.caption_words.duration_of", { n })}
											stored={formatTime(word.duration, fps, DISPLAY)}
											disabled={!canEdit}
											commit={timeCell(index, "duration")}
										/>
									</td>
									<td className="pr-1 pt-1.5">
										<Breaks
											label={t("ui.editor.caption_words.break_of", { n })}
											checked={breaks.includes(index)}
											disabled={!canEdit || index === 0}
											onChange={() =>
												void send(
													captionBreaksOperations(
														itemId,
														toggleBreak(breaks, index),
													),
												)
											}
										/>
									</td>
									<td>
										<Button
											type="button"
											variant="ghost"
											size="xs"
											disabled={!canEdit || index === 0}
											aria-label={t("ui.editor.caption_words.split_before", {
												n,
											})}
											onClick={() => void actions.splitCaption(itemId, index)}
										>
											{t("ui.editor.caption_words.split")}
										</Button>
									</td>
								</tr>
							);
						})}
					</tbody>
				</table>
			</div>
		</Section>
	);
}

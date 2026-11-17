import { type ReactNode, useRef } from "react";

import UntrustedText from "@/components/UntrustedText";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { captionWordSegments } from "../../commands/units";
import { useActions } from "../../commands/useActions";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../../state/EditorContext";
import { beginWordBoundary } from "../gestures/wordBoundary";

// The inline editor of a word: Enter or leaving sends `caption.edit_word` (an
// empty text removes the word), Esc leaves it alone.
function WordEditor({
	itemId,
	index,
	initial,
}: {
	itemId: string;
	index: number;
	initial: string;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const actions = useActions();
	const finished = useRef(false);
	const finish = (value: string | null): void => {
		if (finished.current) return;
		finished.current = true;
		runtime.uiWriter.startWordEdit(null);
		if (value !== null && value !== initial) {
			void actions.editWord(itemId, index, value.trim());
		}
		document
			.querySelector<HTMLElement>(`[data-item-id="${CSS.escape(itemId)}"]`)
			?.focus();
	};
	return (
		<input
			// biome-ignore lint/a11y/noAutofocus: the editor opens on a double click and the person types at once.
			autoFocus
			aria-label={t("ui.editor.caption_words.edit_label", { n: index + 1 })}
			defaultValue={initial}
			maxLength={80}
			className="h-full w-full min-w-0 bg-background px-1 text-xs text-foreground outline-none"
			onFocus={(event) => event.currentTarget.select()}
			onPointerDown={(event) => event.stopPropagation()}
			onClick={(event) => event.stopPropagation()}
			onDoubleClick={(event) => event.stopPropagation()}
			onKeyDown={(event) => {
				event.stopPropagation();
				if (event.key === "Enter") finish(event.currentTarget.value);
				else if (event.key === "Escape") finish(null);
			}}
			onBlur={(event) => finish(event.currentTarget.value)}
		/>
	);
}

// The inside of a caption item (SDD-012 section 7.4): one segment per word when
// the item is wide enough and the zoom allows, else one block with the word
// count. A click on a word selects it, a double click edits it, and the bar in
// front of a word drags the boundary with the word before it.
export default function CaptionItem({ id }: { id: string }): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const actions = useActions();
	const item = useDocumentState((s) => s.document?.items[id]);
	const ppf = useUiState((s) => s.timeline.pxPerFrame);
	const selected = useUiState((s) => s.selection.items.includes(id));
	const word = useUiState((s) => s.selectedWord);
	const editing = useUiState((s) => s.editingWord);
	if (item === undefined) return null;
	const segments = captionWordSegments(item, ppf);
	if (segments.length === 0) {
		return (
			<UntrustedText className="truncate">
				{t("ui.editor.timeline.caption_words", {
					count: item.words?.length ?? 0,
				})}
			</UntrustedText>
		);
	}
	return (
		<div data-caption-words="" className="absolute inset-0">
			{segments.map((segment) => {
				const isSelected =
					selected && word?.item === id && word.index === segment.index;
				const isEditing =
					editing?.item === id && editing.index === segment.index;
				const className = cn(
					"absolute top-1 bottom-1 flex items-center overflow-hidden rounded-sm border border-ed-track-line bg-ed-item-caption px-1 text-left text-xs",
					isSelected && "outline-2 outline-ed-selection",
				);
				const style = { left: segment.left, width: segment.width };
				// An input does not live inside a button, so the word being edited is a plain box.
				if (isEditing) {
					return (
						<div
							key={segment.index}
							data-word-index={segment.index}
							data-selected={isSelected ? "" : undefined}
							className={className}
							style={style}
						>
							<WordEditor
								itemId={id}
								index={segment.index}
								initial={segment.text}
							/>
						</div>
					);
				}
				return (
					<button
						type="button"
						key={segment.index}
						tabIndex={-1}
						aria-label={t("ui.editor.timeline.word_label", {
							n: segment.index + 1,
							text: segment.text,
						})}
						data-word-index={segment.index}
						data-selected={isSelected ? "" : undefined}
						className={className}
						style={style}
						onPointerDown={() =>
							runtime.uiWriter.selectWord({ item: id, index: segment.index })
						}
						onKeyDown={(event) => {
							// Enter or F2 edits the word, as a double click does (VV-095).
							if (event.key !== "Enter" && event.key !== "F2") return;
							event.preventDefault();
							event.stopPropagation();
							runtime.uiWriter.select([id], id);
							runtime.uiWriter.selectWord({ item: id, index: segment.index });
							runtime.uiWriter.startWordEdit({
								item: id,
								index: segment.index,
							});
						}}
					>
						<UntrustedText className="truncate">{segment.text}</UntrustedText>
					</button>
				);
			})}
			{segments.slice(1).map((segment) => (
				<span
					key={`boundary-${segment.index}`}
					aria-hidden="true"
					data-word-boundary={segment.index}
					className="absolute inset-y-0 z-[5] w-2 -translate-x-1/2 cursor-col-resize hover:bg-ed-handle/60"
					style={{ left: segment.left }}
					onPointerDown={(event) => {
						if (event.button !== 0) return;
						event.stopPropagation();
						beginWordBoundary({
							runtime,
							actions,
							event,
							itemId: id,
							index: segment.index,
						});
					}}
				/>
			))}
		</div>
	);
}

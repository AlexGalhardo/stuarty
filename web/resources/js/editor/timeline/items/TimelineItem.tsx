import { memo, type ReactNode, useRef } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { useActions } from "../../commands/useActions";
import { useStore } from "../../state/createStore";
import {
	useActivityState,
	useDocumentState,
	useRuntime,
	useUiState,
} from "../../state/EditorContext";
import { countRender } from "../../state/renderCount";
import { selectAsset, selectItem } from "../../state/selectors";
import { frameInItem } from "../geometry";
import { consumeDragClick } from "../gestures/lifecycle";
import { beginMove } from "../gestures/move";
import { clickSelection, gridLayout } from "../keyboard";
import AudioItem from "./AudioItem";
import Badges from "./Badges";
import CaptionItem from "./CaptionItem";
import ClipItem from "./ClipItem";
import KeyframeDiamonds from "./KeyframeDiamonds";
import ShapeItem from "./ShapeItem";
import TransitionMarks from "./TransitionMarks";
import TrimHandle from "./TrimHandle";

const TYPE_CLASS: Record<string, string> = {
	clip: "bg-ed-item-clip text-ed-item-clip-fg",
	audio: "bg-ed-item-audio text-ed-item-audio-fg",
	text: "bg-ed-item-text text-ed-item-text-fg",
	caption: "bg-ed-item-caption text-ed-item-caption-fg",
	shape: "bg-ed-item-shape text-ed-item-shape-fg",
};

// The inline editor of F2 and the menu entry "Rename": Enter or leaving the
// field sends `item.rename` (an empty name clears it), Esc leaves it alone.
function RenameField({
	itemId,
	initial,
}: {
	itemId: string;
	initial: string;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const actions = useActions();
	const finished = useRef(false);
	const finish = (value: string | null): void => {
		if (finished.current) return;
		finished.current = true;
		runtime.uiWriter.startRename(null);
		if (value !== null && value !== initial)
			void actions.renameItem(itemId, value);
		document
			.querySelector<HTMLElement>(`[data-item-id="${CSS.escape(itemId)}"]`)
			?.focus();
	};
	return (
		<Input
			autoFocus
			aria-label={t("ui.editor.edit.rename_label")}
			defaultValue={initial}
			maxLength={120}
			className="h-6 min-w-0 flex-1 px-1 text-xs text-foreground"
			onFocus={(event) => event.currentTarget.select()}
			onPointerDown={(event) => event.stopPropagation()}
			onClick={(event) => event.stopPropagation()}
			onKeyDown={(event) => {
				event.stopPropagation();
				if (event.key === "Enter") finish(event.currentTarget.value);
				else if (event.key === "Escape") finish(null);
			}}
			onBlur={(event) => finish(event.currentTarget.value)}
		/>
	);
}

// The label: the item name, else the asset name, else the text, else the
// localized type (SDD-012 section 7.4).
export const TimelineItem = memo(function TimelineItem({
	id,
	listed = true,
}: {
	id: string;
	// False when the row lists only a few clips to assistive technology: the
	// cell stays on the screen and in the keyboard order (VV-191).
	listed?: boolean;
}): ReactNode {
	countRender("TimelineItem");
	const t = useT();
	const runtime = useRuntime();
	const actions = useActions();
	const item = useDocumentState((s) => selectItem(s, id));
	const asset = useDocumentState((s) =>
		item?.asset === undefined ? undefined : selectAsset(s, item.asset),
	);
	const selected = useUiState((s) => s.selection.items.includes(id));
	const tabStop = useUiState((s) => s.focusedItem === id);
	// Outlined for a moment when it blocked a move (SDD-012 section 8.4).
	const conflicting = useStore(runtime.ghosts, (s) => s.errorItem === id);
	const remote = useActivityState((s) => s.highlighted.includes(id));
	const renaming = useUiState((s) => s.renamingItem === id);
	const blade = useUiState((s) => s.tool === "blade");
	if (item === undefined) return null;

	const typeName = t(`ui.editor.item.${item.type}`);
	const label =
		item.name ?? asset?.name ?? item.text?.split("\n")[0] ?? typeName;

	return (
		// biome-ignore lint/a11y/useSemanticElements: the cell sits in a CSS-positioned grid (SDD-012 section 7.1).
		<div
			role="gridcell"
			aria-hidden={listed ? undefined : true}
			tabIndex={tabStop ? 0 : -1}
			aria-selected={selected}
			aria-label={t("ui.editor.timeline.item_summary", {
				type: typeName,
				name: label,
			})}
			data-item-id={id}
			className={cn(
				"ed-motion group absolute top-1 bottom-1 flex min-w-px items-center overflow-hidden rounded-md border border-ed-track-line px-2 text-xs whitespace-nowrap",
				TYPE_CLASS[item.type],
				blade ? "cursor-crosshair" : "cursor-pointer",
				item.disabled === true && "opacity-40",
				selected && "ring-2 ring-ed-selection",
				remote && "ed-highlight outline-2 outline-agent",
				conflicting && "ed-highlight outline-2 outline-destructive",
			)}
			style={{
				// The item is placed by two custom properties and the zoom variable of
				// #timeline, so a zoom change relayouts without re-rendering any item.
				["--f" as string]: item.start,
				["--d" as string]: item.duration,
				left: "calc(var(--f) * var(--ppf) * 1px)",
				width: "calc(var(--d) * var(--ppf) * 1px)",
			}}
			data-conflicting={conflicting ? "" : undefined}
			data-disabled={item.disabled === true ? "" : undefined}
			onPointerDown={(event) => {
				if (event.button === 0 && runtime.ui.getState().tool === "blade") {
					event.preventDefault();
					void actions.splitItemAt(
						id,
						frameInItem(
							item.start,
							event.clientX,
							event.currentTarget.getBoundingClientRect().left,
							runtime.ui.getState().timeline.pxPerFrame,
						),
					);
					return;
				}
				if (
					event.button !== 0 ||
					event.shiftKey ||
					event.ctrlKey ||
					event.metaKey
				) {
					return;
				}
				beginMove({ runtime, actions, event, itemId: id });
			}}
			onDoubleClick={(event) => {
				if (runtime.documents.getState().state === "trashed") return;
				if (item.type === "caption") {
					// The pointer is captured by the item while it is pressed, so the
					// event no longer names the word under it.
					const word = document
						.elementsFromPoint(event.clientX, event.clientY)
						.find((el) => el instanceof HTMLElement && el.dataset.wordIndex)
						?.getAttribute("data-word-index");
					if (word === null || word === undefined) return;
					runtime.uiWriter.select([id], id);
					runtime.uiWriter.selectWord({ item: id, index: Number(word) });
					runtime.uiWriter.startWordEdit({ item: id, index: Number(word) });
					return;
				}
				if (item.type !== "text") return;
				runtime.uiWriter.select([id], id);
				runtime.uiWriter.startTextEdit(id);
			}}
			onClick={(event) => {
				event.stopPropagation();
				if (consumeDragClick()) return;
				const { selection } = runtime.ui.getState();
				const next = clickSelection(
					gridLayout(runtime.documents.getState()),
					selection.items,
					selection.anchor,
					id,
					{ shift: event.shiftKey, toggle: event.ctrlKey || event.metaKey },
				);
				runtime.uiWriter.select(next.items, next.anchor);
			}}
		>
			{item.type === "clip" && <ClipItem id={id} />}
			{item.type === "audio" && <AudioItem id={id} />}
			<TrimHandle itemId={id} edge="start" />
			{item.type === "shape" && <ShapeItem id={id} />}
			{renaming ? (
				<RenameField itemId={id} initial={item.name ?? ""} />
			) : item.type === "caption" ? (
				<CaptionItem id={id} />
			) : (
				<UntrustedText
					// The cell already carries the name; the text would be read twice.
					aria-hidden="true"
					className={cn(
						"relative z-10 truncate",
						(item.type === "clip" || item.type === "audio") &&
							"rounded bg-background/70 px-1",
					)}
				>
					{label}
				</UntrustedText>
			)}
			<Badges id={id} />
			<TransitionMarks id={id} />
			<KeyframeDiamonds id={id} />
			<TrimHandle itemId={id} edge="end" />
		</div>
	);
});

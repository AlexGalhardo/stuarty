import { AudioLines, Video } from "lucide-react";
import { memo, type ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { useActions } from "../commands/useActions";
import { useStore } from "../state/createStore";
import {
	useActivityState,
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import {
	selectAsset,
	selectItem,
	selectTrack,
	selectTrackItemIds,
} from "../state/selectors";
import { AUDIO_ROW_PX, sortByStart, VISUAL_ROW_PX } from "./geometry";
import { consumeDragClick } from "./gestures/lifecycle";
import { beginMove } from "./gestures/move";
import { clickSelection, gridLayout } from "./keyboard";

const TYPE_CLASS: Record<string, string> = {
	clip: "bg-ed-item-clip text-ed-item-clip-fg",
	audio: "bg-ed-item-audio text-ed-item-audio-fg",
	text: "bg-ed-item-text text-ed-item-text-fg",
	caption: "bg-ed-item-caption text-ed-item-caption-fg",
	shape: "bg-ed-item-shape text-ed-item-shape-fg",
};

export function rowHeight(kind: "visual" | "audio"): number {
	return kind === "audio" ? AUDIO_ROW_PX : VISUAL_ROW_PX;
}

// The label: the item name, else the asset name, else the text, else the
// localized type (SDD-012 section 7.4).
const ItemBlock = memo(function ItemBlock({ id }: { id: string }): ReactNode {
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
	if (item === undefined) return null;

	const typeName = t(`ui.editor.item.${item.type}`);
	const label =
		item.name ?? asset?.name ?? item.text?.split("\n")[0] ?? typeName;

	return (
		// biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard selects with Enter, handled by the grid (SDD-012 section 7.10).
		// biome-ignore lint/a11y/useSemanticElements: the cell sits in a CSS-positioned grid (SDD-012 section 7.1).
		<div
			role="gridcell"
			tabIndex={tabStop ? 0 : -1}
			aria-selected={selected}
			aria-label={t("ui.editor.timeline.item_summary", {
				type: typeName,
				name: label,
			})}
			data-item-id={id}
			className={cn(
				"absolute top-1 bottom-1 flex min-w-px cursor-pointer items-center overflow-hidden rounded-md border border-ed-track-line px-2 text-xs whitespace-nowrap",
				TYPE_CLASS[item.type],
				selected && "ring-2 ring-ed-selection",
				remote && "outline-2 outline-agent",
				conflicting && "outline-2 outline-destructive",
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
			onPointerDown={(event) => {
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
			<UntrustedText className="truncate">{label}</UntrustedText>
		</div>
	);
});

export function TrackHeader({ id }: { id: string }): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const track = useDocumentState((s) => selectTrack(s, id));
	if (track === undefined) return null;
	const Icon = track.kind === "audio" ? AudioLines : Video;
	return (
		// biome-ignore lint/a11y/useSemanticElements: the header sits in a CSS-positioned grid (SDD-012 section 7.1).
		<div
			role="rowheader"
			tabIndex={-1}
			onClick={() => runtime.uiWriter.selectTrack(id)}
			onKeyDown={(event) => {
				if (event.key !== "Enter") return;
				event.preventDefault();
				runtime.uiWriter.selectTrack(id);
			}}
			style={{ height: rowHeight(track.kind) }}
			className="flex items-center gap-2 border-b border-ed-track-line px-3 text-xs"
		>
			<Icon
				aria-hidden="true"
				className="size-4 shrink-0 text-muted-foreground"
			/>
			<span className="sr-only">
				{track.kind === "audio"
					? t("ui.editor.timeline.track_audio")
					: t("ui.editor.timeline.track_visual")}
			</span>
			<UntrustedText className="truncate font-medium">
				{track.name ?? id}
			</UntrustedText>
			{track.hidden === true && (
				<span className="text-muted-foreground">
					({t("ui.editor.timeline.hidden")})
				</span>
			)}
			{track.muted === true && (
				<span className="text-muted-foreground">
					({t("ui.editor.timeline.muted")})
				</span>
			)}
		</div>
	);
}

export const TrackRow = memo(function TrackRow({
	id,
}: {
	id: string;
}): ReactNode {
	const track = useDocumentState((s) => selectTrack(s, id));
	const ids = useDocumentState((s) => selectTrackItemIds(s, id));
	const items = useDocumentState((s) => s.document?.items);
	if (track === undefined || items === undefined) return null;
	const ordered = sortByStart(
		ids.map((itemId) => ({
			id: itemId,
			start: items[itemId].start,
			duration: items[itemId].duration,
		})),
	);
	return (
		// biome-ignore lint/a11y/useSemanticElements: the row sits in a CSS-positioned grid (SDD-012 section 7.1).
		<div
			role="row"
			tabIndex={-1}
			data-track-id={id}
			style={{ height: rowHeight(track.kind) }}
			className={cn(
				"relative border-b border-ed-track-line",
				(track.hidden === true || track.muted === true) && "opacity-60",
			)}
		>
			{ordered.map((entry) => (
				<ItemBlock key={entry.id} id={entry.id} />
			))}
		</div>
	);
});

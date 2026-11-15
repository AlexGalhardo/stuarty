import { memo, type ReactNode, useContext, useMemo, useState } from "react";

import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { useCanEdit } from "../commands/useActions";
import { useStore } from "../state/createStore";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { selectTrack } from "../state/selectors";
import GapContextMenuItems from "./GapContextMenu";
import { clientXToFrame } from "./geometry";
import ItemContextMenuItems from "./ItemContextMenu";
import { TimelineItem } from "./items/TimelineItem";
import { selectTrackItems } from "./trackItems";
import { mountedIds } from "./virtualize";
import { TimelineWindowContext } from "./windowContext";

// The height of the rows of one kind of track: the default or what the person
// dragged the splitter to (SDD-012 section 7.4).
export function useRowHeight(kind: "visual" | "audio"): number {
	return useUiState((s) => s.rowPx[kind]);
}

export const TrackRow = memo(function TrackRow({
	id,
}: {
	id: string;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const track = useDocumentState((s) => selectTrack(s, id));
	const rowPx = useRowHeight(track?.kind === "audio" ? "audio" : "visual");
	const items = useDocumentState((s) => selectTrackItems(s, id));
	const frames = useContext(TimelineWindowContext);
	// Items that stay mounted outside the window: selected, focused, renamed,
	// the one a move is outlining.
	const selected = useUiState((s) => s.selection.items);
	const focused = useUiState((s) => s.focusedItem);
	const renaming = useUiState((s) => s.renamingItem);
	const conflicting = useStore(runtime.ghosts, (s) => s.errorItem);
	const canEdit = useCanEdit();
	const [target, setTarget] = useState<
		| { kind: "item"; id: string; word: number | null }
		| { kind: "gap"; frame: number }
	>({ kind: "gap", frame: 0 });
	const mounted = useMemo(() => {
		const pinned = new Set<string>(selected);
		for (const extra of [focused, renaming, conflicting]) {
			if (extra !== null) pinned.add(extra);
		}
		return mountedIds(items, frames, pinned);
	}, [items, frames, selected, focused, renaming, conflicting]);
	if (track === undefined) return null;
	// A right-click picks the menu: an item (selected first when it was not
	// part of the selection) or the empty place under the pointer.
	const onContextMenu = (event: React.MouseEvent<HTMLElement>): void => {
		const cell = (event.target as HTMLElement).closest<HTMLElement>(
			"[data-item-id]",
		);
		const itemId = cell?.dataset.itemId;
		if (itemId !== undefined) {
			if (!runtime.ui.getState().selection.items.includes(itemId))
				runtime.uiWriter.select([itemId], itemId);
			const word = (event.target as HTMLElement).closest<HTMLElement>(
				"[data-word-index]",
			)?.dataset.wordIndex;
			setTarget({
				kind: "item",
				id: itemId,
				word: word === undefined ? null : Number(word),
			});
			return;
		}
		const left = event.currentTarget.getBoundingClientRect().left;
		setTarget({
			kind: "gap",
			frame: clientXToFrame(
				event.clientX,
				left,
				0,
				// Read when the menu opens: a zoom does not re-render the row (SDD-012 section 19.2 item 3).
				runtime.ui.getState().timeline.pxPerFrame,
			),
		});
	};
	return (
		<ContextMenu>
			<ContextMenuTrigger asChild disabled={!canEdit}>
				{/* biome-ignore lint/a11y/useSemanticElements: the row sits in a CSS-positioned grid (SDD-012 section 7.1). */}
				<div
					role="row"
					tabIndex={-1}
					data-track-id={id}
					style={{ height: rowPx }}
					className={cn(
						"relative border-b border-ed-track-line",
						(track.hidden === true || track.muted === true) && "opacity-60",
					)}
					onContextMenu={onContextMenu}
				>
					{mounted.map((itemId) => (
						<TimelineItem key={itemId} id={itemId} />
					))}
				</div>
			</ContextMenuTrigger>
			<ContextMenuContent
				aria-label={t(
					target.kind === "item"
						? "ui.editor.edit.item_menu"
						: "ui.editor.edit.gap_menu",
				)}
			>
				{target.kind === "item" ? (
					<ItemContextMenuItems itemId={target.id} word={target.word} />
				) : (
					<GapContextMenuItems trackId={id} frame={target.frame} />
				)}
			</ContextMenuContent>
		</ContextMenu>
	);
});

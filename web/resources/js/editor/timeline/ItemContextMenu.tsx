import { CopyPlus, Merge, Pencil, Scissors, Split, Trash2 } from "lucide-react";
import type { ReactNode } from "react";

import {
	ContextMenuItem,
	ContextMenuSeparator,
} from "@/components/ui/context-menu";
import { useT } from "@/lib/i18n";
import { useActions, useCanCut } from "../commands/useActions";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";

// The entries of the item menu (SDD-012 section 8.2). They act on the
// selection; the row selects the item under the pointer first when it was not
// part of it.
export default function ItemContextMenuItems({
	itemId,
	word = null,
}: {
	itemId: string;
	// The caption word under the pointer, if the menu was opened on one.
	word?: number | null;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const runtime = useRuntime();
	const canCut = useCanCut();
	const selection = useUiState((s) => s.selection.items);
	const isCaption = useDocumentState(
		(s) => s.document?.items[itemId]?.type === "caption",
	);
	// Merge takes exactly two captions; the core judges whether they share a track.
	const captionPair = useDocumentState(
		(s) =>
			selection.length === 2 &&
			selection.every((id) => s.document?.items[id]?.type === "caption"),
	);
	return (
		<>
			{isCaption && word !== null && (
				<ContextMenuItem
					disabled={word < 1}
					onSelect={() => void actions.splitCaption(itemId, word)}
				>
					<Split aria-hidden="true" />
					{t("ui.editor.edit.split_caption")}
				</ContextMenuItem>
			)}
			{captionPair && (
				<ContextMenuItem onSelect={() => void actions.mergeCaptions(selection)}>
					<Merge aria-hidden="true" />
					{t("ui.editor.edit.merge_captions")}
				</ContextMenuItem>
			)}
			<ContextMenuItem
				disabled={!canCut}
				onSelect={() => void actions.splitAtPlayhead()}
			>
				<Scissors aria-hidden="true" />
				{t("ui.editor.edit.split")}
			</ContextMenuItem>
			<ContextMenuItem onSelect={() => void actions.duplicateSelection()}>
				<CopyPlus aria-hidden="true" />
				{t("ui.editor.edit.duplicate")}
			</ContextMenuItem>
			<ContextMenuItem onSelect={() => runtime.uiWriter.startRename(itemId)}>
				<Pencil aria-hidden="true" />
				{t("ui.editor.edit.rename")}
			</ContextMenuItem>
			<ContextMenuSeparator />
			<ContextMenuItem
				variant="destructive"
				onSelect={() => void actions.deleteSelection()}
			>
				<Trash2 aria-hidden="true" />
				{t("ui.editor.edit.delete")}
			</ContextMenuItem>
			<ContextMenuItem
				variant="destructive"
				onSelect={() => void actions.deleteSelection("track")}
			>
				<Trash2 aria-hidden="true" />
				{t("ui.editor.edit.ripple_delete")}
			</ContextMenuItem>
			<ContextMenuItem
				variant="destructive"
				onSelect={() => void actions.deleteSelection("all")}
			>
				<Trash2 aria-hidden="true" />
				{t("ui.editor.edit.ripple_delete_all")}
			</ContextMenuItem>
		</>
	);
}

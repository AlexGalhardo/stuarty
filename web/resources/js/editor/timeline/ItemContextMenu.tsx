import { CopyPlus, Pencil, Scissors, Trash2 } from "lucide-react";
import type { ReactNode } from "react";

import {
	ContextMenuItem,
	ContextMenuSeparator,
} from "@/components/ui/context-menu";
import { useT } from "@/lib/i18n";
import { useActions, useCanCut } from "../commands/useActions";
import { useRuntime } from "../state/EditorContext";

// The entries of the item menu (SDD-012 section 8.2). They act on the
// selection; the row selects the item under the pointer first when it was not
// part of it.
export default function ItemContextMenuItems({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const runtime = useRuntime();
	const canCut = useCanCut();
	return (
		<>
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

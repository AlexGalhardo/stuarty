import { type ReactElement, type ReactNode, useState } from "react";

import RenameAssetDialog from "@/components/RenameAssetDialog";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { useT } from "@/lib/i18n";

// The context menu of a file in a library page: right click or the context-menu
// key on `children` offers Rename (epic 60, SDD-011 section 14.8). The dialog sits
// outside the trigger, so a right click inside it is the browser's own.
export default function RenameMenu({
	name,
	rename,
	returnFocus,
	extra,
	children,
}: {
	name: string;
	rename(name: string): Promise<void>;
	// The element that takes the focus when the dialog closes.
	returnFocus(): HTMLElement | null;
	// More items of the same menu, after Rename (Show in folder, Copy path: epic 94).
	extra?: ReactNode;
	children: ReactElement;
}): ReactNode {
	const t = useT();
	const [renaming, setRenaming] = useState(false);
	return (
		<ContextMenu>
			<ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
			<ContextMenuContent aria-label={t("ui.editor.library.rename.menu")}>
				<ContextMenuItem onSelect={() => setRenaming(true)}>
					{t("ui.editor.library.rename.action")}
				</ContextMenuItem>
				{extra}
			</ContextMenuContent>
			<RenameAssetDialog
				open={renaming}
				onOpenChange={setRenaming}
				name={name}
				rename={rename}
				returnFocus={returnFocus}
			/>
		</ContextMenu>
	);
}

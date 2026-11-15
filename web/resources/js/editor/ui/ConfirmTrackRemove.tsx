import type { ReactNode } from "react";

import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useT } from "@/lib/i18n";

// `E_NOT_EMPTY` of a `track.remove` (SDD-012 section 4.4): the sentence names how
// many items go with the track; confirming sends the same operation with
// `with_items: true` (the caller does it).
export default function ConfirmTrackRemove({
	count,
	open,
	onOpenChange,
	onConfirm,
}: {
	// Read from the read model by the caller, for the sentence only.
	count: number;
	open: boolean;
	onOpenChange(open: boolean): void;
	onConfirm(): void;
}): ReactNode {
	const t = useT();
	return (
		<AlertDialog open={open} onOpenChange={onOpenChange}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>
						{t("ui.editor.track.remove_title")}
					</AlertDialogTitle>
					<AlertDialogDescription>
						{t("ui.editor.track.remove_body", { count })}
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>
						{t("ui.editor.track.remove_cancel")}
					</AlertDialogCancel>
					<AlertDialogAction variant="destructive" onClick={onConfirm}>
						{t("ui.editor.track.remove_confirm")}
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}

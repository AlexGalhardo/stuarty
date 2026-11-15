import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { useT } from "@/lib/i18n";

export interface UsedBy {
	id: string;
	label: string;
	// Whether the id names an item the timeline can select.
	selectable: boolean;
}

// E_IN_USE of an asset.remove (SDD-012 section 4.4): the items that still use
// the asset, as links that select them.
export default function InUseDialog({
	open,
	users,
	onSelect,
	onClose,
}: {
	open: boolean;
	users: readonly UsedBy[];
	onSelect(id: string): void;
	onClose(): void;
}): ReactNode {
	const t = useT();
	return (
		<Dialog open={open} onOpenChange={(next) => !next && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t("ui.editor.library.in_use.title")}</DialogTitle>
					<DialogDescription>
						{t("ui.editor.library.in_use.body")}
					</DialogDescription>
				</DialogHeader>
				<ul className="flex flex-col gap-1">
					{users.map((user) => (
						<li key={user.id}>
							<Button
								type="button"
								variant="link"
								size="sm"
								data-used-by={user.id}
								disabled={!user.selectable}
								onClick={() => onSelect(user.id)}
							>
								<UntrustedText>{user.label}</UntrustedText>
							</Button>
						</li>
					))}
				</ul>
			</DialogContent>
		</Dialog>
	);
}

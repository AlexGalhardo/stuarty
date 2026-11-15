import { Redo2, Undo2 } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useActions, useCanEdit } from "../commands/useActions";
import { useDocumentState, useUiState } from "../state/EditorContext";

// The Undo and Redo buttons of the top bar (SDD-012 section 15). Without a
// history panel in the Prototype they are always enabled, except after the
// core answered that there is nothing to undo or redo, until the next batch.
export default function UndoRedo(): ReactNode {
	const t = useT();
	const actions = useActions();
	const revision = useDocumentState((s) => s.revision);
	const ready = useCanEdit();
	const blocked = useUiState((s) => s.historyBlocked);
	return (
		<div className="flex items-center gap-1">
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!ready || blocked.undo === revision}
				onClick={() => void actions.undo()}
			>
				<Undo2 aria-hidden="true" />
				{t("ui.editor.undo")}
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!ready || blocked.redo === revision}
				onClick={() => void actions.redo()}
			>
				<Redo2 aria-hidden="true" />
				{t("ui.editor.redo")}
			</Button>
		</div>
	);
}

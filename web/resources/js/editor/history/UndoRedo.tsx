import { Redo2, Undo2 } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useActions, useCanEdit } from "../commands/useActions";
import { useStore } from "../state/createStore";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";

// The Undo and Redo buttons of the top bar (SDD-012 section 15). They follow
// `undoable_by_me` and `redoable_by_me` of the first page of the history; while
// the history says nothing (the Prototype rule) they are enabled, except after
// the core answered that there is nothing to undo or redo, until the next batch.
export default function UndoRedo(): ReactNode {
	const t = useT();
	const actions = useActions();
	const revision = useDocumentState((s) => s.revision);
	const ready = useCanEdit();
	const blocked = useUiState((s) => s.historyBlocked);
	const runtime = useRuntime();
	const undoable = useStore(runtime.history, (s) => s.undoable);
	const redoable = useStore(runtime.history, (s) => s.redoable);
	return (
		<div className="flex items-center gap-1">
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!ready || undoable === false || blocked.undo === revision}
				onClick={() => void actions.undo()}
			>
				<Undo2 aria-hidden="true" />
				{t("ui.editor.undo")}
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!ready || redoable === false || blocked.redo === revision}
				onClick={() => void actions.redo()}
			>
				<Redo2 aria-hidden="true" />
				{t("ui.editor.redo")}
			</Button>
		</div>
	);
}

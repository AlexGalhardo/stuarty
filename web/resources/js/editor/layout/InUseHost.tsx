import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { revealItem } from "../timeline/revealItem";
import InUseDialog from "../ui/InUseDialog";

// Connects the dialog of E_IN_USE to the stores: it names the items from the
// read model and selecting one selects it in the timeline.
export default function InUseHost(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const inUse = useUiState((s) => s.inUse);
	const items = useDocumentState((s) => s.document?.items);
	const close = (): void => runtime.uiWriter.showInUse(null);
	const users = (inUse?.usedBy ?? []).map((id) => {
		const item = items?.[id];
		return {
			id,
			label:
				item === undefined
					? id
					: (item.name ?? t(`ui.editor.item.${item.type}`)),
			selectable: item !== undefined,
		};
	});
	return (
		<InUseDialog
			open={inUse !== null}
			users={users}
			onClose={close}
			onSelect={(id) => {
				runtime.uiWriter.select([id], id);
				runtime.uiWriter.focusItem(id);
				revealItem(id);
				close();
			}}
		/>
	);
}

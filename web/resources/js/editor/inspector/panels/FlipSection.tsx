import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { flipOperations } from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

// The Flip section of a visual item: the two mirrors of `clip.set_flip`, one
// command per switch naming only the side that changed. The preview and the
// export mirror the layer from the plan (SDD-004 section 3.10).
export default function FlipSection({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("clip.set_flip");
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (!exposed || item === undefined) return null;
	return (
		<Section title={t("ui.editor.clip.flip_title")}>
			<ToggleField
				label={t("ui.editor.clip.flip_horizontal")}
				stored={item.flip_x === true}
				disabled={!canEdit}
				commit={(horizontal) => send(flipOperations(itemId, { horizontal }))}
			/>
			<ToggleField
				label={t("ui.editor.clip.flip_vertical")}
				stored={item.flip_y === true}
				disabled={!canEdit}
				commit={(vertical) => send(flipOperations(itemId, { vertical }))}
			/>
		</Section>
	);
}

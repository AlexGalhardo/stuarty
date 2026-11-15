import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { captionStyleOperations } from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import StyleRows, { CAPTION_STYLE_ROWS } from "./StyleRows";

// The Caption style section of SDD-012 section 12.1: the CaptionStyle fields,
// including the highlight of the spoken word (RF-032), and "Use track style".
// `caption.set_style` replaces the override, so a change sends the stored style
// with the changed key; `null` clears it.
export default function CaptionStyle({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const style = useDocumentState((s) => s.document?.items[itemId]?.style);
	const found = useDocumentState(
		(s) => s.document?.items[itemId] !== undefined,
	);
	if (!found) return null;
	return (
		<Section title={t("ui.editor.inspector.caption_style_title")}>
			<StyleRows
				rows={CAPTION_STYLE_ROWS}
				stored={style ?? {}}
				disabled={!canEdit}
				commit={(changes) =>
					send(captionStyleOperations(itemId, { ...style, ...changes }))
				}
			/>
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={!canEdit || style === undefined}
				onClick={() => void send(captionStyleOperations(itemId, null))}
			>
				{t("ui.editor.inspector.use_track_style")}
			</Button>
		</Section>
	);
}

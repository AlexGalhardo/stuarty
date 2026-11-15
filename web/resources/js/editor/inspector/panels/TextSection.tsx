import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	type StyleChanges,
	textSetOperations,
	textStyleOperations,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import TextAreaField from "../fields/TextAreaField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import StyleRows, { TEXT_STYLE_ROWS } from "./StyleRows";

// The Text section of SDD-012 section 12.1: the content (`text.set {text}`) and
// every style field (`text.set {style}` with the changed key only). The font
// select lists the families of `getFonts()`; E_FONT_UNKNOWN reloads it and
// highlights the families the core named.
export default function TextSection({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (item === undefined) return null;
	return (
		<Section title={t("ui.editor.inspector.text_title")}>
			<TextAreaField
				label={t("ui.editor.inspector.text_content")}
				stored={item.text ?? ""}
				maxLength={2000}
				disabled={!canEdit}
				commit={(text) => send(textSetOperations(itemId, text))}
			/>
			<StyleRows
				rows={TEXT_STYLE_ROWS}
				stored={item.style ?? {}}
				disabled={!canEdit}
				commit={(style: StyleChanges) =>
					send(textStyleOperations(itemId, style))
				}
			/>
		</Section>
	);
}

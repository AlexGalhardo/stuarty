import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import TemplateInfoSection from "../inspector/panels/TemplateInfoSection";
import TemplateSection from "../inspector/panels/TemplateSection";
import PanelFrame from "../library/PanelFrame";
import { useDocumentState, useUiState } from "../state/EditorContext";

// The Templates tab of the material rail (SDD-012 section 6.4): the template
// info and the slots of the project (`template.set_info`, `slot.fill`), and
// "Template slot" for the one item selected (`slot.mark`, `slot.unmark`). The
// sections are those of the inspector, so each control sends what it always did.
export default function TemplatesTab(): ReactNode {
	const t = useT();
	const selected = useUiState((s) => s.selection.items);
	const only = selected.length === 1 ? selected[0] : undefined;
	const type = useDocumentState((s) =>
		only === undefined ? undefined : s.document?.items[only]?.type,
	);
	return (
		<PanelFrame title={t("ui.editor.panels.templates")}>
			<TemplateInfoSection />
			{only !== undefined && type !== undefined && type !== "shape" ? (
				<TemplateSection key={only} itemId={only} />
			) : (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.library.templates_select")}
				</p>
			)}
		</PanelFrame>
	);
}

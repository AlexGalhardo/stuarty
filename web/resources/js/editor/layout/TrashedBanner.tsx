import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { useDocumentState } from "../state/EditorContext";

// A trashed project opens read-only (SDD-012 section 2.2): this banner says so,
// and every control that sends an edit is disabled through useCanEdit.
export default function TrashedBanner(): ReactNode {
	const t = useT();
	const trashed = useDocumentState((s) => s.state === "trashed");
	if (!trashed) return null;
	return (
		<div
			role="status"
			className="flex items-center gap-3 border-b border-ed-track-line bg-ed-panel px-3 py-2 text-sm text-warning"
		>
			{t("ui.editor.banner.trashed")}
		</div>
	);
}

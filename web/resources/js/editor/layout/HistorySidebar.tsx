import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import HistoryPanel from "../history/HistoryPanel";
import { PanelTitle } from "../library/PanelFrame";
import { useUiState } from "../state/EditorContext";

// The history sidebar (SDD-012 sections 6.1 and 15, epic 60): a region of its own
// right next to the library, newest step on top. The button that opens it is the
// last icon of the library rail (LeftPanel), as the final design draws it; Alt+3
// does the same. Collapsed it takes no room. Whether it is open is remembered at
// this browser.
export default function HistorySidebar(): ReactNode {
	const t = useT();
	const open = useUiState((s) => s.panels.history);
	if (!open) return <div />;
	return (
		<aside
			aria-label={t("ui.editor.history.sidebar")}
			data-landmark="2.5"
			data-open={open}
			className="flex min-h-0 min-w-0 border-l border-border bg-ed-panel"
		>
			<div className="flex min-h-0 min-w-0 flex-1 flex-col gap-2 overflow-auto p-3">
				<PanelTitle>{t("ui.editor.history.title")}</PanelTitle>
				<HistoryPanel />
			</div>
		</aside>
	);
}

import type { ReactNode } from "react";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useT } from "@/lib/i18n";
import HistoryPanel from "../history/HistoryPanel";
import Library from "../library/Library";
import { useRuntime, useUiState } from "../state/EditorContext";
import { useAssetDrag } from "../timeline/gestures/dragAsset";

// The left panel of SDD-012 section 6.1: the library, and beside it the
// History tab (section 15). Alt+1 and Alt+3 show a tab or hide the panel.
export default function LeftPanel(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const startAssetDrag = useAssetDrag();
	const tab = useUiState((s) => s.activePanelTab);
	return (
		<aside
			aria-label={t(
				tab === "library"
					? "ui.editor.panels.library"
					: "ui.editor.history.tab",
			)}
			data-landmark="2"
			className="flex min-h-0 min-w-0 flex-col bg-ed-panel"
		>
			<Tabs
				value={tab}
				onValueChange={(value) =>
					runtime.uiWriter.showLeftTab(
						value === "history" ? "history" : "library",
					)
				}
				className="min-h-0 flex-1 gap-0"
			>
				<div className="px-3 pt-3">
					<TabsList>
						<TabsTrigger value="library">
							{t("ui.editor.panels.library")}
						</TabsTrigger>
						<TabsTrigger value="history">
							{t("ui.editor.history.tab")}
						</TabsTrigger>
					</TabsList>
				</div>
				<TabsContent
					value="library"
					className="flex min-h-0 flex-1 flex-col *:flex-1"
				>
					<Library onAssetDragStart={startAssetDrag} />
				</TabsContent>
				<TabsContent
					value="history"
					className="flex min-h-0 flex-1 flex-col overflow-auto p-3"
				>
					<HistoryPanel />
				</TabsContent>
			</Tabs>
		</aside>
	);
}

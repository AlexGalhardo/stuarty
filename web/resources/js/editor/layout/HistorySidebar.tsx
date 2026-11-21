import { History } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { useT } from "@/lib/i18n";
import HistoryPanel from "../history/HistoryPanel";
import { useRuntime, useUiState } from "../state/EditorContext";

// The history sidebar (SDD-012 sections 6.1 and 15, epic 60): a region of its own
// right next to the library, newest step on top. Collapsed it keeps a rail with
// the button that opens it, as the library does; Alt+3 does the same. Whether it
// is open is remembered at this browser.
export default function HistorySidebar(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const open = useUiState((s) => s.panels.history);
	const label = t(open ? "ui.editor.history.hide" : "ui.editor.history.show");
	return (
		<aside
			aria-label={t("ui.editor.history.sidebar")}
			data-open={open}
			className="flex min-h-0 min-w-0 border-l border-border bg-ed-panel"
		>
			<div className="flex w-10 shrink-0 flex-col items-center gap-1 border-r border-border p-1">
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							type="button"
							size="icon-sm"
							variant={open ? "secondary" : "ghost"}
							aria-label={label}
							aria-expanded={open}
							data-history-toggle=""
							onClick={() => runtime.uiWriter.toggleHistory()}
						>
							<History aria-hidden="true" />
						</Button>
					</TooltipTrigger>
					<TooltipContent side="right">{label}</TooltipContent>
				</Tooltip>
			</div>
			{open && (
				<div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-auto p-3">
					<HistoryPanel />
				</div>
			)}
		</aside>
	);
}

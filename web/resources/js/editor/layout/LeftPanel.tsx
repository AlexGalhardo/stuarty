import type { ReactNode } from "react";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { useT } from "@/lib/i18n";
import { useCommandNames } from "../commands/useExposedCommands";
import { useRuntime, useUiState } from "../state/EditorContext";
import type { LeftTab } from "../state/uiStore";
import { useAssetDrag } from "../timeline/gestures/dragAsset";
import { LIBRARY_TABS } from "./libraryTabs";

// The material region of SDD-012 sections 6.1 and 6.4: a vertical rail of 40 px
// with one icon tab per panel, and the panel of the selected tab beside it.
// Alt+1 hides the panel and keeps the rail (docs/design-system.md 3.4.4). The
// history is a sidebar of its own beside this region (HistorySidebar).
export default function LeftPanel(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const startAssetDrag = useAssetDrag();
	const tab = useUiState((s) => s.activePanelTab);
	const open = useUiState((s) => s.panels.left);
	const names = useCommandNames();
	const tabs = LIBRARY_TABS.filter((def) =>
		def.commands.every((name) => names.has(name)),
	);
	const active = LIBRARY_TABS.find((def) => def.id === tab);
	return (
		<aside
			aria-label={t(active?.labelKey ?? "ui.editor.panels.library")}
			data-landmark="2"
			className="flex min-h-0 min-w-0 bg-ed-panel"
		>
			<Tabs
				orientation="vertical"
				value={open ? tab : ""}
				onValueChange={(value) =>
					runtime.uiWriter.showLeftTab(value as LeftTab)
				}
				className="min-h-0 min-w-0 flex-1 flex-row gap-0"
			>
				<TabsList
					aria-label={t("ui.editor.library.rail")}
					className="w-10 shrink-0 justify-start group-data-vertical/tabs:h-full gap-1 rounded-none border-r border-border bg-transparent p-1"
				>
					{tabs.map((def) => (
						<Tooltip key={def.id}>
							<TooltipTrigger asChild>
								<TabsTrigger
									value={def.id}
									aria-label={t(def.labelKey)}
									data-rail-tab={def.id}
									className="size-8 flex-none px-0 group-data-vertical/tabs:w-8 group-data-vertical/tabs:justify-center data-active:bg-accent data-active:shadow-none group-data-vertical/tabs:after:right-auto group-data-vertical/tabs:after:-left-1 after:bg-ring data-active:after:opacity-100 dark:data-active:bg-accent"
								>
									<def.icon aria-hidden="true" />
								</TabsTrigger>
							</TooltipTrigger>
							<TooltipContent side="right">{t(def.labelKey)}</TooltipContent>
						</Tooltip>
					))}
				</TabsList>
				{LIBRARY_TABS.map((def) => (
					<TabsContent
						key={def.id}
						value={def.id}
						className={`flex min-h-0 min-w-0 flex-1 flex-col ${def.bodyClass ?? ""}`}
					>
						{def.render({ onAssetDragStart: startAssetDrag })}
					</TabsContent>
				))}
			</Tabs>
		</aside>
	);
}

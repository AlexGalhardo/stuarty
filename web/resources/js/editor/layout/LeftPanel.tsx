import { History } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { useT } from "@/lib/i18n";
import { useCommandNames } from "../commands/useExposedCommands";
import { TabIcon } from "../library/PanelFrame";
import { CutsSource } from "../stages/CutsPanels";
import TranscriptStage from "../stages/TranscriptStage";
import { useRuntime, useUiState } from "../state/EditorContext";
import type { LeftTab } from "../state/uiStore";
import { useAssetDrag } from "../timeline/gestures/dragAsset";
import { LIBRARY_TABS } from "./libraryTabs";

// The material region of SDD-012 sections 6.1 and 6.4: a vertical rail of 44 px
// with one icon tab per panel, and the panel of the selected tab beside it.
// Alt+1 hides the panel and keeps the rail (docs/design-system.md 3.4.4). The
// history button is the last icon of the rail; its panel is a sidebar of its own (HistorySidebar).
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
	// The edit by text screen puts the transcript where the panel of the tab is.
	const transcript = useUiState((s) => s.stage?.kind === "transcript");
	const cuts = useUiState((s) => s.stage?.kind === "cuts");
	const voice = useUiState((s) => s.stage?.kind === "voice");
	const historyOpen = useUiState((s) => s.panels.history);
	const historyLabel = t(
		historyOpen ? "ui.editor.history.hide" : "ui.editor.history.show",
	);
	// The icon lit on the rail is the tool the screen is for (final design 26, 27, 30), not the panel it replaced.
	const lit = transcript ? "audio" : cuts ? "text" : voice ? "audio" : tab;
	return (
		<aside
			aria-label={t(active?.labelKey ?? "ui.editor.panels.library")}
			data-landmark="2"
			className="flex min-h-0 min-w-0 bg-ed-panel"
		>
			<Tabs
				orientation="vertical"
				value={open || transcript || cuts || voice ? lit : ""}
				onValueChange={(value) =>
					runtime.uiWriter.showLeftTab(value as LeftTab)
				}
				className="min-h-0 min-w-0 flex-1 flex-row gap-0"
			>
				{/* A tablist owns only tabs (ARIA aria-required-children), so the history button sits below it in the rail, outside the list. */}
				<div
					data-rail=""
					className="flex w-11 shrink-0 flex-col gap-1 border-r border-border p-1"
				>
					<TabsList
						aria-label={t("ui.editor.library.rail")}
						className="w-full justify-start gap-1 rounded-none bg-transparent p-0"
					>
						{tabs.map((def) => (
							<Tooltip key={def.id}>
								<TooltipTrigger asChild>
									<TabsTrigger
										value={def.id}
										aria-label={t(def.labelKey)}
										data-rail-tab={def.id}
										className="size-8 flex-none px-0 group-data-vertical/tabs:w-8 group-data-vertical/tabs:justify-center data-active:bg-surface-selected data-active:shadow-none group-data-vertical/tabs:after:right-auto group-data-vertical/tabs:after:-left-1 after:bg-ring data-active:after:opacity-100 dark:data-active:bg-surface-selected"
									>
										<def.icon aria-hidden="true" />
									</TabsTrigger>
								</TooltipTrigger>
								<TooltipContent side="right">{t(def.labelKey)}</TooltipContent>
							</Tooltip>
						))}
					</TabsList>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								type="button"
								size="icon-sm"
								variant={historyOpen ? "secondary" : "ghost"}
								aria-label={historyLabel}
								aria-expanded={historyOpen}
								data-history-toggle=""
								className="mt-auto size-8"
								onClick={() => runtime.uiWriter.toggleHistory()}
							>
								<History aria-hidden="true" />
							</Button>
						</TooltipTrigger>
						<TooltipContent side="right">{historyLabel}</TooltipContent>
					</Tooltip>
				</div>
				{transcript && <TranscriptStage />}
				{cuts && <CutsSource />}
				{!transcript &&
					!cuts &&
					LIBRARY_TABS.map((def) => (
						<TabsContent
							key={def.id}
							value={def.id}
							className={`flex min-h-0 min-w-0 flex-1 flex-col ${def.bodyClass ?? ""}`}
						>
							<TabIcon.Provider value={def.icon}>
								{def.render({ onAssetDragStart: startAssetDrag })}
							</TabIcon.Provider>
						</TabsContent>
					))}
			</Tabs>
		</aside>
	);
}

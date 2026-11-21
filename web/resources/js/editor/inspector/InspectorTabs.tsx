import { type ReactNode, useLayoutEffect, useRef } from "react";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useT } from "@/lib/i18n";
import { useRuntime, useUiState } from "../state/EditorContext";
import type { InspectorTab } from "../state/uiStore";

export interface InspectorTabDef {
	id: InspectorTab;
	content: ReactNode;
}

// The tab strip of the properties panel (SDD-012 section 6.4, docs/design-system.md
// 3.4.6): one tab for each domain the selection has sections of. The caller lists
// only the tabs the selection has; the chosen tab is remembered across
// selections, and a selection without it shows its first tab. The strip scrolls
// by itself when the labels do not fit; they never truncate.
export default function InspectorTabs({
	tabs,
}: {
	tabs: readonly InspectorTabDef[];
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const chosen = useUiState((s) => s.inspectorTab);
	const active = tabs.some((tab) => tab.id === chosen)
		? chosen
		: (tabs[0]?.id ?? chosen);
	const strip = useRef<HTMLDivElement>(null);
	// The strip scrolls by itself when the labels do not fit; the chosen tab is
	// kept whole in view, and a strip with room left starts at its first tab
	// instead of at a scroll the browser left (VV-328).
	// biome-ignore lint/correctness/useExhaustiveDependencies: the tabs drawn change the widths the effect reads
	useLayoutEffect(() => {
		const list = strip.current;
		const tab = list?.querySelector<HTMLElement>(
			`[data-inspector-tab="${active}"]`,
		);
		if (!list || !tab) return;
		const box = list.getBoundingClientRect();
		const at = tab.getBoundingClientRect();
		const left = at.left - box.left + list.scrollLeft;
		if (left < list.scrollLeft) {
			list.scrollLeft = left;
		} else if (left + at.width > list.scrollLeft + list.clientWidth) {
			list.scrollLeft = left + at.width - list.clientWidth;
		}
	}, [active, tabs.length]);
	return (
		<Tabs
			value={active}
			onValueChange={(value) =>
				runtime.uiWriter.showInspectorTab(value as InspectorTab)
			}
			className="gap-3"
		>
			<TabsList
				ref={strip}
				variant="line"
				aria-label={t("ui.editor.inspector.tabs")}
				className="h-9 w-full justify-start overflow-x-auto border-b border-border p-0"
			>
				{tabs.map((tab) => (
					<TabsTrigger
						key={tab.id}
						value={tab.id}
						data-inspector-tab={tab.id}
						className="h-9 flex-none px-3 whitespace-nowrap text-muted-foreground after:bg-ring group-data-horizontal/tabs:after:bottom-0 data-active:text-foreground"
					>
						{t(`ui.editor.inspector.tab.${tab.id}`)}
					</TabsTrigger>
				))}
			</TabsList>
			{tabs.map((tab) => (
				<TabsContent
					key={tab.id}
					value={tab.id}
					className="flex flex-col gap-3"
				>
					{tab.content}
				</TabsContent>
			))}
		</Tabs>
	);
}

import { AudioLines, Film, History, type LucideIcon, Type } from "lucide-react";
import type { ReactNode } from "react";

import HistoryPanel from "../history/HistoryPanel";
import type { AssetDragStart } from "../library/AssetCard";
import AudioPanel from "../library/AudioPanel";
import Library from "../library/Library";
import TextPanel from "../library/TextPanel";
import type { LeftTab } from "../state/uiStore";

export interface LibraryTabDef {
	id: LeftTab;
	labelKey: string;
	icon: LucideIcon;
	// A tab is drawn only when the registry lists all of these (SDD-012 6.4).
	commands: readonly string[];
	bodyClass?: string;
	render(deps: { onAssetDragStart?: AssetDragStart }): ReactNode;
}

// The tabs of the material rail, in the order of docs/design-system.md 3.4.4.
export const LIBRARY_TABS: readonly LibraryTabDef[] = [
	{
		id: "library",
		labelKey: "ui.editor.panels.library",
		icon: Film,
		commands: ["clip.add"],
		bodyClass: "*:flex-1",
		render: ({ onAssetDragStart }) => (
			<Library onAssetDragStart={onAssetDragStart} />
		),
	},
	{
		id: "audio",
		labelKey: "ui.editor.panels.audio",
		icon: AudioLines,
		commands: ["audio.add"],
		bodyClass: "*:flex-1",
		render: ({ onAssetDragStart }) => (
			<AudioPanel onAssetDragStart={onAssetDragStart} />
		),
	},
	{
		id: "text",
		labelKey: "ui.editor.panels.text",
		icon: Type,
		commands: ["text.add"],
		bodyClass: "*:flex-1",
		render: () => <TextPanel />,
	},
	{
		id: "history",
		labelKey: "ui.editor.history.tab",
		icon: History,
		commands: [],
		bodyClass: "overflow-auto p-3",
		render: () => <HistoryPanel />,
	},
];

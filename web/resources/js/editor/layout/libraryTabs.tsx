import {
	ArrowLeftRight,
	AudioLines,
	Captions,
	Film,
	History,
	type LucideIcon,
	SlidersHorizontal,
	Type,
} from "lucide-react";
import type { ReactNode } from "react";

import HistoryPanel from "../history/HistoryPanel";
import type { AssetDragStart } from "../library/AssetCard";
import AudioPanel from "../library/AudioPanel";
import FiltersPanel from "../library/FiltersPanel";
import Library from "../library/Library";
import TextPanel from "../library/TextPanel";
import TransitionsPanel from "../library/TransitionsPanel";
import type { LeftTab } from "../state/uiStore";
import CaptionsTab from "./CaptionsTab";

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
		id: "captions",
		labelKey: "ui.editor.panels.captions",
		icon: Captions,
		commands: ["caption.from_transcript"],
		bodyClass: "*:flex-1",
		render: () => <CaptionsTab />,
	},
	{
		id: "transitions",
		labelKey: "ui.editor.panels.transitions",
		icon: ArrowLeftRight,
		commands: ["item.set_transition"],
		bodyClass: "*:flex-1",
		render: () => <TransitionsPanel />,
	},
	{
		id: "filters",
		labelKey: "ui.editor.panels.filters",
		icon: SlidersHorizontal,
		commands: ["clip.set_filter"],
		bodyClass: "*:flex-1",
		render: () => <FiltersPanel />,
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

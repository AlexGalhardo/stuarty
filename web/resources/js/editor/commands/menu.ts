import type { useT } from "@/lib/i18n";
import type { EditorRuntime } from "../state/runtime";
import { startTour } from "../state/tourStore";
import type { Actions } from "./actions";
import {
	pasteFromClipboard,
	selectAllItems,
	takeToClipboard,
} from "./clipboard";
import { openImport } from "./openImport";
import { requestSurface } from "./surface";

// The editor menu of SDD-012 section 6.4. One table feeds the "Menu" button of
// the web top bar and the native menu bar of the desktop shell, which calls
// `runMenuAction` through `window.vukemDesktop.onMenuAction`. An item holds only
// the id of an action that exists; the key shown beside it is the row of
// `SHORTCUTS` named by `row`, never a key written here.

// What an item needs to be enabled: an editable project, a selection, or a
// selected item the playhead lies inside.
export type MenuNeed = "edit" | "selection" | "cut";

export interface MenuItemDef {
	id: string;
	labelKey: string;
	row?: string;
	need?: MenuNeed;
	// The registry names the item needs; it is drawn only when all are listed.
	commands?: readonly string[];
}

export type MenuEntry = MenuItemDef | "separator";

export interface MenuDef {
	id: string;
	labelKey: string;
	items: readonly MenuEntry[];
}

export const MENUS: readonly MenuDef[] = [
	{
		id: "file",
		labelKey: "ui.editor.menu.file",
		items: [
			{ id: "surface.open_project", labelKey: "ui.editor.menu.open_project" },
			{
				id: "surface.import",
				labelKey: "ui.editor.menu.import",
				row: "import",
				need: "edit",
				commands: ["clip.add"],
			},
			{
				id: "surface.export",
				labelKey: "ui.editor.menu.export",
				row: "export",
			},
		],
	},
	{
		id: "edit",
		labelKey: "ui.editor.menu.edit",
		items: [
			{ id: "undo", labelKey: "ui.editor.undo", row: "undo", need: "edit" },
			{ id: "redo", labelKey: "ui.editor.redo", row: "redo", need: "edit" },
			"separator",
			{
				id: "surface.cut",
				labelKey: "ui.editor.menu.cut",
				row: "cut",
				need: "selection",
			},
			{
				id: "surface.copy",
				labelKey: "ui.editor.menu.copy",
				row: "copy",
				need: "selection",
			},
			{
				id: "surface.paste",
				labelKey: "ui.editor.menu.paste",
				row: "paste",
				need: "edit",
			},
			{
				id: "surface.select_all",
				labelKey: "ui.editor.menu.select_all",
				row: "select_all",
			},
			"separator",
			{
				id: "deleteSelection",
				labelKey: "ui.editor.edit.delete",
				row: "delete",
				need: "selection",
				commands: ["item.delete"],
			},
			{
				id: "duplicateSelection",
				labelKey: "ui.editor.edit.duplicate",
				row: "duplicate",
				need: "selection",
				commands: ["item.duplicate"],
			},
		],
	},
	{
		id: "clip",
		labelKey: "ui.editor.menu.clip",
		items: [
			{
				id: "splitAtPlayhead",
				labelKey: "ui.editor.edit.split",
				row: "split",
				need: "cut",
				commands: ["item.split"],
			},
			{
				id: "trimToPlayhead.start",
				labelKey: "ui.editor.edit.trim_start",
				row: "trim_start",
				need: "cut",
				commands: ["item.trim"],
			},
			{
				id: "trimToPlayhead.end",
				labelKey: "ui.editor.edit.trim_end",
				row: "trim_end",
				need: "cut",
				commands: ["item.trim"],
			},
			{
				id: "surface.open_inspector_tab.speed",
				labelKey: "ui.editor.menu.speed",
				need: "selection",
				commands: ["clip.set_speed"],
			},
			{
				id: "renameItem",
				labelKey: "ui.editor.edit.rename",
				row: "rename",
				need: "selection",
				commands: ["item.rename"],
			},
			{
				id: "freezeFrame",
				labelKey: "ui.editor.edit.freeze_frame",
				need: "cut",
				commands: ["item.freeze_frame"],
			},
			{
				id: "toggleEnabled",
				labelKey: "ui.editor.menu.toggle_enabled",
				row: "toggle_enabled",
				need: "selection",
				commands: ["item.set_enabled"],
			},
		],
	},
	{
		id: "timeline",
		labelKey: "ui.editor.menu.timeline",
		items: [
			{
				id: "addTrack.visual",
				labelKey: "ui.editor.menu.add_video_track",
				need: "edit",
				commands: ["track.add"],
			},
			{
				id: "addTrack.audio",
				labelKey: "ui.editor.menu.add_audio_track",
				need: "edit",
				commands: ["track.add"],
			},
			"separator",
			{
				id: "surface.toggle_snap",
				labelKey: "ui.editor.timeline.snap",
				row: "toggle_snap",
			},
			{
				id: "toggleMagnet",
				labelKey: "ui.editor.timeline.magnet",
				row: "toggle_magnet",
				need: "edit",
				commands: ["track.update"],
			},
			{
				id: "toggleMarkerAtPlayhead",
				labelKey: "ui.editor.menu.marker",
				row: "marker",
				need: "edit",
				commands: ["marker.add"],
			},
			"separator",
			{
				id: "addCaption",
				labelKey: "ui.editor.edit.caption",
				row: "add_caption",
				need: "edit",
				commands: ["caption.add"],
			},
			{
				id: "addText",
				labelKey: "ui.editor.edit.text",
				row: "add_text",
				need: "edit",
				commands: ["text.add"],
			},
		],
	},
	{
		id: "view",
		labelKey: "ui.editor.menu.view",
		items: [
			{
				id: "surface.toggle_panel.library",
				labelKey: "ui.editor.panels.library",
			},
			{
				id: "surface.toggle_panel.inspector",
				labelKey: "ui.editor.panels.inspector",
			},
			{
				id: "surface.toggle_panel.history",
				labelKey: "ui.editor.history.tab",
			},
		],
	},
	{
		id: "help",
		labelKey: "ui.editor.menu.help",
		items: [
			{
				id: "surface.cheat_sheet",
				labelKey: "ui.editor.menu.shortcuts",
				row: "cheat_sheet",
			},
			{ id: "surface.tour", labelKey: "ui.editor.menu.tour" },
		],
	},
];

export interface MenuDeps {
	actions: Actions;
	runtime: EditorRuntime;
	t: ReturnType<typeof useT>;
}

const MENU_ACTIONS: Readonly<Record<string, (deps: MenuDeps) => void>> = {
	"surface.open_project": () => location.assign("/projects"),
	"surface.import": ({ runtime }) => openImport(runtime),
	"surface.export": () => requestSurface("export"),
	undo: ({ actions }) => void actions.undo(),
	redo: ({ actions }) => void actions.redo(),
	"surface.cut": ({ runtime }) => void takeToClipboard(runtime, true),
	"surface.copy": ({ runtime }) => void takeToClipboard(runtime, false),
	"surface.paste": ({ runtime, actions }) =>
		void pasteFromClipboard(runtime, actions),
	"surface.select_all": ({ runtime }) => selectAllItems(runtime),
	deleteSelection: ({ actions }) => void actions.deleteSelection(),
	duplicateSelection: ({ actions }) => void actions.duplicateSelection(),
	splitAtPlayhead: ({ actions }) => void actions.splitAtPlayhead(),
	"trimToPlayhead.start": ({ actions }) => void actions.trimToPlayhead("start"),
	"trimToPlayhead.end": ({ actions }) => void actions.trimToPlayhead("end"),
	"surface.open_inspector_tab.speed": ({ runtime }) => {
		if (!runtime.ui.getState().panels.inspector) {
			runtime.uiWriter.toggleInspector();
		}
		runtime.uiWriter.showInspectorTab("speed");
	},
	renameItem: ({ runtime }) => {
		const { selection, focusedItem } = runtime.ui.getState();
		const id = selection.items.length === 1 ? selection.items[0] : focusedItem;
		if (id !== null && id !== undefined) runtime.uiWriter.startRename(id);
	},
	freezeFrame: ({ actions }) => void actions.freezeFrame(),
	toggleEnabled: ({ actions }) => void actions.toggleEnabled(),
	"addTrack.visual": ({ actions }) =>
		void actions.addTrack("visual", { position: "bottom" }),
	"addTrack.audio": ({ actions }) =>
		void actions.addTrack("audio", { position: "bottom" }),
	"surface.toggle_snap": ({ runtime }) => runtime.uiWriter.toggleSnap(),
	toggleMagnet: ({ actions }) => void actions.toggleMagnet(),
	toggleMarkerAtPlayhead: ({ actions }) =>
		void actions.toggleMarkerAtPlayhead(),
	addCaption: () => requestSurface("add_caption"),
	addText: ({ actions, t }) =>
		void actions.addText(t("ui.editor.defaults.text")),
	"surface.toggle_panel.library": ({ runtime }) =>
		runtime.uiWriter.toggleLeftRegion(),
	"surface.toggle_panel.inspector": ({ runtime }) =>
		runtime.uiWriter.toggleInspector(),
	"surface.toggle_panel.history": ({ runtime }) =>
		runtime.uiWriter.toggleHistory(),
	"surface.cheat_sheet": () => requestSurface("cheat_sheet"),
	"surface.tour": () => startTour(),
};

// The ids the menu can run; a native menu sends only these.
export const MENU_ACTION_IDS: readonly string[] = Object.keys(MENU_ACTIONS);

// Runs the action of a menu item; false for an id that is not one.
export function runMenuAction(id: string, deps: MenuDeps): boolean {
	const run = MENU_ACTIONS[id];
	if (run === undefined) return false;
	run(deps);
	return true;
}

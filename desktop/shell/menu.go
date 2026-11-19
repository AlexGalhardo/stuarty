package shell

import (
	"encoding/json"
	"strings"
)

// The editor menus of the native menu bar (SDD-012 sections 6.4 and 21.3 item
// 4). Each item names an action id of the editor's menu table
// (web/resources/js/editor/commands/menu.ts); a click runs it on the page
// through window.vukemDesktop.onMenuAction, so the shell decides nothing about
// editing. An item is here only because the page has the action.

// Text keys of the editor menus.
const (
	TextMenuEdit         = "menu_edit"
	TextMenuClip         = "menu_clip"
	TextMenuTimeline     = "menu_timeline"
	TextMenuOpenProject  = "menu_open_project"
	TextMenuImport       = "menu_import"
	TextMenuExport       = "menu_export"
	TextMenuUndo         = "menu_undo"
	TextMenuRedo         = "menu_redo"
	TextMenuCut          = "menu_cut"
	TextMenuCopy         = "menu_copy"
	TextMenuPaste        = "menu_paste"
	TextMenuSelectAll    = "menu_select_all"
	TextMenuDelete       = "menu_delete"
	TextMenuDuplicate    = "menu_duplicate"
	TextMenuSplit        = "menu_split"
	TextMenuTrimStart    = "menu_trim_start"
	TextMenuTrimEnd      = "menu_trim_end"
	TextMenuSpeed        = "menu_speed"
	TextMenuRename       = "menu_rename"
	TextMenuFreezeFrame  = "menu_freeze_frame"
	TextMenuToggle       = "menu_toggle_enabled"
	TextMenuAddVideo     = "menu_add_video_track"
	TextMenuAddAudio     = "menu_add_audio_track"
	TextMenuSnap         = "menu_snap"
	TextMenuMagnet       = "menu_magnet"
	TextMenuMarker       = "menu_marker"
	TextMenuAddCaption   = "menu_add_caption"
	TextMenuAddText      = "menu_add_text"
	TextMenuPanelMedia   = "menu_panel_library"
	TextMenuPanelInspect = "menu_panel_inspector"
	TextMenuPanelHistory = "menu_panel_history"
	TextMenuShortcuts    = "menu_shortcuts"
	TextMenuTour         = "menu_tour"
)

// MenuItem is one native item: the action the page runs, the catalogue key of
// its text and, for the three keys the shell shows but does not register, the
// key to display. A zero MenuItem is a separator.
type MenuItem struct {
	Action string
	Text   string
	// Hint is shown next to the text and is not an accelerator: Mod+Z, Mod+Shift+Z
	// and Mod+A reach the webview, where the editor decides between the native
	// undo of a field and the project undo (SDD-012 section 21.3 item 4).
	Hint string
}

// MenuGroup is a top-level menu. Groups that exist in the shell already (File,
// View, Help) add these items to their own.
type MenuGroup struct {
	Name  string
	Items []MenuItem
}

// EditorMenus are the groups, by name: "file", "edit", "clip", "timeline", "view" and "help".
var EditorMenus = []MenuGroup{
	{Name: "file", Items: []MenuItem{
		{Action: "surface.open_project", Text: TextMenuOpenProject},
		{Action: "surface.import", Text: TextMenuImport},
		{Action: "surface.export", Text: TextMenuExport},
	}},
	{Name: "edit", Items: []MenuItem{
		{Action: "undo", Text: TextMenuUndo, Hint: "Mod+Z"},
		{Action: "redo", Text: TextMenuRedo, Hint: "Mod+Shift+Z"},
		{},
		{Action: "surface.cut", Text: TextMenuCut},
		{Action: "surface.copy", Text: TextMenuCopy},
		{Action: "surface.paste", Text: TextMenuPaste},
		{Action: "surface.select_all", Text: TextMenuSelectAll, Hint: "Mod+A"},
		{},
		{Action: "deleteSelection", Text: TextMenuDelete},
		{Action: "duplicateSelection", Text: TextMenuDuplicate},
	}},
	{Name: "clip", Items: []MenuItem{
		{Action: "splitAtPlayhead", Text: TextMenuSplit},
		{Action: "trimToPlayhead.start", Text: TextMenuTrimStart},
		{Action: "trimToPlayhead.end", Text: TextMenuTrimEnd},
		{Action: "surface.open_inspector_tab.speed", Text: TextMenuSpeed},
		{Action: "renameItem", Text: TextMenuRename},
		{Action: "freezeFrame", Text: TextMenuFreezeFrame},
		{Action: "toggleEnabled", Text: TextMenuToggle},
	}},
	{Name: "timeline", Items: []MenuItem{
		{Action: "addTrack.visual", Text: TextMenuAddVideo},
		{Action: "addTrack.audio", Text: TextMenuAddAudio},
		{},
		{Action: "surface.toggle_snap", Text: TextMenuSnap},
		{Action: "toggleMagnet", Text: TextMenuMagnet},
		{Action: "toggleMarkerAtPlayhead", Text: TextMenuMarker},
		{},
		{Action: "addCaption", Text: TextMenuAddCaption},
		{Action: "addText", Text: TextMenuAddText},
	}},
	{Name: "view", Items: []MenuItem{
		{Action: "surface.toggle_panel.library", Text: TextMenuPanelMedia},
		{Action: "surface.toggle_panel.inspector", Text: TextMenuPanelInspect},
		{Action: "surface.toggle_panel.history", Text: TextMenuPanelHistory},
	}},
	{Name: "help", Items: []MenuItem{
		{Action: "surface.cheat_sheet", Text: TextMenuShortcuts},
		{Action: "surface.tour", Text: TextMenuTour},
	}},
}

// DisplayHint writes a hint for the system: Mod is Cmd on macOS, Ctrl elsewhere.
func DisplayHint(hint, goos string) string {
	mod := "Ctrl"
	if goos == "darwin" {
		mod = "Cmd"
	}
	return strings.ReplaceAll(hint, "Mod", mod)
}

// MenuActionScript is the script that tells the page an item was clicked. It
// reports false for an id that is not an item of EditorMenus, so the shell
// never sends the page a string it did not define.
func MenuActionScript(action string) (string, bool) {
	for _, group := range EditorMenus {
		for _, item := range group.Items {
			if item.Action != "" && item.Action == action {
				id, err := json.Marshal(action)
				if err != nil {
					return "", false
				}
				return `window._wails&&window._wails.vukemMenu&&window._wails.vukemMenu(` + string(id) + `);`, true
			}
		}
	}
	return "", false
}

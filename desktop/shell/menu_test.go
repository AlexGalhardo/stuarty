package shell

import (
	"strings"
	"testing"
)

func TestMenuActionScriptKnowsOnlyTheItemsOfTheMenus(t *testing.T) {
	s, ok := MenuActionScript("undo")
	if !ok || !strings.Contains(s, `vukemMenu("undo")`) {
		t.Fatalf("undo must have a script: %q, %v", s, ok)
	}
	for _, id := range []string{"", "nope", `undo");alert(1);("`, "pickFiles"} {
		if s, ok := MenuActionScript(id); ok || s != "" {
			t.Errorf("%q must be refused, got %q", id, s)
		}
	}
}

func TestEveryMenuItemHasATextInEveryLanguage(t *testing.T) {
	seen := map[string]bool{}
	for _, group := range EditorMenus {
		for _, item := range group.Items {
			if item.Action == "" {
				if item.Text != "" || item.Hint != "" {
					t.Errorf("a separator carries a text: %+v", item)
				}
				continue
			}
			if seen[item.Action] {
				t.Errorf("%s appears twice", item.Action)
			}
			seen[item.Action] = true
			for lang := range catalogues {
				if catalogues[lang][item.Text] == "" {
					t.Errorf("%s misses %s for %s", lang, item.Text, item.Action)
				}
			}
		}
	}
}

func TestOnlyUndoRedoAndSelectAllShowAKey(t *testing.T) {
	want := map[string]string{"undo": "Mod+Z", "redo": "Mod+Shift+Z", "surface.select_all": "Mod+A"}
	for _, group := range EditorMenus {
		for _, item := range group.Items {
			if item.Hint != want[item.Action] {
				t.Errorf("%s shows %q, want %q", item.Action, item.Hint, want[item.Action])
			}
		}
	}
	if got := DisplayHint("Mod+Shift+Z", "darwin"); got != "Cmd+Shift+Z" {
		t.Errorf("mac hint = %q", got)
	}
	if got := DisplayHint("Mod+Z", "windows"); got != "Ctrl+Z" {
		t.Errorf("windows hint = %q", got)
	}
}

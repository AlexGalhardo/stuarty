package shell

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// TestAdoptionMessageSaysOnceThatTheWorkspaceWasAdopted: the desktop shows the line Resolve prints when it
// adopts the workspace of Vukem Video, after a sentence in the language, and nothing for the other notice
// (both folders exist), which Resolve prints at every start.
func TestAdoptionMessageSaysOnceThatTheWorkspaceWasAdopted(t *testing.T) {
	a := &workspace.Adoption{
		Method: workspace.AdoptedByRename, Former: "/home/a/.vukem-video", Dir: "/home/a/.stuarty",
		Backup: "/home/a/.stuarty/backups/adopt-20261118T123000Z",
	}
	both := "stuarty: a workspace of Vukem Video is in /home/a/.vukem-video and was not adopted\n"

	got := AdoptionMessage("pt-BR", both+a.Notice())
	if !strings.HasPrefix(got, Text("pt-BR", TextAdopted)+"\n\nThe workspace of Vukem Video, /home/a/.vukem-video,") ||
		!strings.Contains(got, a.Backup) || strings.Contains(got, "stuarty:") {
		t.Errorf("AdoptionMessage = %q", got)
	}

	if got := AdoptionMessage("en", both); got != "" {
		t.Errorf("AdoptionMessage without an adoption = %q, want nothing", got)
	}
}

func TestLanguage(t *testing.T) {
	for in, want := range map[string]string{"pt-BR": "pt-BR", "pt_PT.UTF-8": "pt-BR", "en-US": "en", "": "en", "de-DE": "en"} {
		if got := Language(in); got != want {
			t.Errorf("Language(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEveryLanguageHasEveryKey(t *testing.T) {
	for lang, c := range catalogues {
		for key := range catalogues["en"] {
			if c[key] == "" {
				t.Errorf("%s misses %s", lang, key)
			}
		}
	}
	if Text("xx", TextQuit) != "Quit" {
		t.Error("unknown language must fall back to English")
	}
}

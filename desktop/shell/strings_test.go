package shell

import "testing"

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

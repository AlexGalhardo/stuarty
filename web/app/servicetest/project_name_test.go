package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// VV-049 and epic 56: one name rule for the desktop and the network mode, and it names the field.
func TestProjectCreateNameRule(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	for name, n := range map[string]string{
		"nul":                "ab\x00cd",
		"rtl override":       "ab\u202ecd",
		"isolate":            "ab\u2067cd",
		"newline":            "ab\ncd",
		"spaces only":        "     ",
		"tab only":           "\t\t\t\t",
		"zero width":         "\u200b\u200b\u200b\u200b",
		"empty":              "",
		"three characters":   "abc",
		"thirty-three":       "abcdefghijklmnopqrstuvwxyzabcdefg",
		"two spaces":         "Launch  teaser",
		"leading space":      " Launch teaser",
		"trailing space":     "Launch teaser ",
		"punctuation":        "Launch, teaser",
		"no-break space":     "Launch\u00a0teaser",
		"an emoji":           "Launch teaser \U0001F680",
		"slash":              "Launch/teaser",
		"only a single char": "x",
	} {
		req := testproject.Request("Launch teaser")
		req.Name = n

		_, err := e.svc.ProjectCreate(e.as(ui), req)
		if code(err) != command.CodeValidation {
			t.Errorf("%s: code = %q (%v), want E_VALIDATION", name, code(err), err)
		}
	}

	for _, n := range []string{"Café 日本語 ok", "Vídeo ação", "Take 2", "abcd"} {
		if _, err := testproject.Create(e.as(ui), e.svc, n); err != nil {
			t.Errorf("%q was refused: %v", n, err)
		}
	}
}

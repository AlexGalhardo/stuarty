package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// VV-049: one name rule for the desktop and the network mode.
func TestProjectCreateNameRule(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	for name, n := range map[string]string{
		"nul":          "a\x00b",
		"rtl override": "a\u202eb",
		"isolate":      "a\u2067b",
		"newline":      "a\nb",
		"spaces only":  "   ",
		"tab only":     "\t",
		"zero width":   "\u200b",
		"empty":        "",
	} {
		_, err := e.svc.ProjectCreate(e.as(ui), service.ProjectCreateRequest{Name: n})
		if code(err) != command.CodeValidation {
			t.Errorf("%s: code = %q (%v), want E_VALIDATION", name, code(err), err)
		}
	}

	if _, err := e.svc.ProjectCreate(e.as(ui), service.ProjectCreateRequest{Name: "Café 日本語 ok"}); err != nil {
		t.Errorf("a plain name was refused: %v", err)
	}
}

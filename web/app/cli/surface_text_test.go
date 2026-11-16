package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

func TestExpandPath(t *testing.T) {
	t.Parallel()

	env := func(k string) string {
		return map[string]string{"HOME": "/home/ana"}[k]
	}

	for in, want := range map[string]string{
		"~":                      "/home/ana",
		"~/clips/a.mp4":          "/home/ana/clips/a.mp4",
		"file:///srv/a.mp4":      "/srv/a.mp4",
		"file:///C:/Users/a.mp4": "C:/Users/a.mp4",
		"file:///my%20clip.mp4":  "/my clip.mp4",
		"a~b.mp4":                "a~b.mp4",
		"relative/a.mp4":         "relative/a.mp4",
	} {
		if got := expandPath(in, env); got != want {
			t.Errorf("expandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrorsSpeakTheLanguageOfTheCLI(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer

	rt := &Runtime{Stdout: &out, Stderr: &errOut, resolved: []string{"/work/missing.ttf"}}

	code := writeError(rt, false, &service.Error{
		Code: "E_FILE_NOT_FOUND", Message: "the file cannot be read.",
	})

	got := errOut.String()
	if code != ExitFailure || !strings.Contains(got, "the file cannot be read: /work/missing.ttf.") {
		t.Errorf("the missing path is not named: %q", got)
	}
}

func TestReadReasonHasNoStopAndNoOSLanguage(t *testing.T) {
	t.Parallel()

	if got := readReason(&fs.PathError{Op: "open", Path: "x", Err: fs.ErrNotExist}); got != "the file does not exist" {
		t.Errorf("missing file: %q", got)
	}

	localized := fmt.Errorf("%s", "O sistema nao pode encontrar o arquivo especificado.")
	if got := readReason(localized); strings.HasSuffix(got, ".") {
		t.Errorf("a trailing full stop is left: %q", got)
	}

	_, err := os.ReadFile("definitely-missing-file.json")
	if got := readReason(err); got != "the file does not exist" {
		t.Errorf("os.ReadFile error: %q", got)
	}
}

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/desktop/shell"
	"github.com/AlexGalhardo/vukem-video/desktop/shell/webviewenv"
)

const helperEnv = "VUKEM_VIDEO_TEST_STARTUP_ENV_HELPER"

// The Wails WebView2 loader blanks WEBVIEW2_* in its initialiser (VV-144). The shell must still see the
// value the process was started with, so the check runs in a child of this binary, which has all the
// initialisers of the real application behind it.
func TestStartupEnvSurvivesTheLoaderInitialiser(t *testing.T) {
	if os.Getenv(helperEnv) != "" {
		t.Skip("child process")
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupEnvHelper$", "-test.v")
	cmd.Env = append(os.Environ(), helperEnv+"=1", shell.WebviewArgsEnv+"=--remote-debugging-port=9223")

	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "startup-args=--remote-debugging-port=9223") {
		t.Fatalf("the argument was lost before main: %v\n%s", err, out)
	}
}

func TestStartupEnvHelper(t *testing.T) {
	if os.Getenv(helperEnv) == "" {
		t.Skip("run by TestStartupEnvSurvivesTheLoaderInitialiser")
	}

	t.Log("startup-args=" + webviewenv.Getenv(shell.WebviewArgsEnv))
}

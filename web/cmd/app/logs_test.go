package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service/logging"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

func TestOpenLogWritesToStderrAndTheWorkspaceFile(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	getenv := func(k string) string {
		return map[string]string{workspace.EnvHome: ws, "LOG_LEVEL": "debug"}[k]
	}

	var stderr bytes.Buffer

	log, closer := openLog(getenv, &stderr)
	log.Debug("hello", "name", "a\nb")

	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(ws, "logs", logging.File))
	if err != nil || !strings.Contains(string(data), "msg=hello") || !strings.Contains(string(data), "name=a?b") {
		t.Errorf("file = %q, %v", data, err)
	}

	if !strings.Contains(stderr.String(), "msg=hello") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestLevelOf(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"": slog.LevelInfo, "debug": slog.LevelDebug, "WARN": slog.LevelWarn, "error": slog.LevelError, "x": slog.LevelInfo,
	} {
		if got := levelOf(in); got != want {
			t.Errorf("levelOf(%q) = %v, want %v", in, got, want)
		}
	}
}

package main

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/AlexGalhardo/vukem-video/service/logging"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// openLog is the logger of an owner (ADR-0026 item 2): standard error and a
// rotating file under the workspace logs folder, with the level of LOG_LEVEL
// (vukem's own variable; debug is the only level that logs media text). A
// workspace whose logs folder cannot be written still logs to standard error,
// and says so there.
func openLog(getenv func(string) string, stderr io.Writer) (*slog.Logger, io.Closer) {
	o := logging.Options{Stderr: stderr, Level: levelOf(getenv("LOG_LEVEL"))}

	dir, err := workspace.Resolve(getenv)
	if err == nil {
		o.Dir = dir.Path("logs")
	}

	log, closer, lerr := logging.New(o)
	if lerr != nil {
		o.Dir = ""
		log, closer, _ = logging.New(o)

		_, _ = fmt.Fprintln(stderr, "vukem-video: the log file is not available, logging to standard error only:", lerr)
	}

	return log, closer
}

func levelOf(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}

	return slog.LevelInfo
}

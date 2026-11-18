package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service/logging"
)

const hostile = "clip\nlevel=ERROR msg=forged actor=root \x1b[31mred\x1b[0m \u202Eevil\u2028next\r\x00end"

// T-25: a name with a line feed, an ANSI escape and a bidirectional override
// produces one line, the characters replaced.
func TestSecurity_T25_ControlCharsNeverForgeLogLines(t *testing.T) {
	var out bytes.Buffer

	log, closer, err := logging.New(logging.Options{Stderr: &out, Level: slog.LevelDebug})
	if err != nil {
		t.Fatal(err)
	}

	defer closer.Close()

	ctx := context.Background()

	logging.Media(ctx, log, "imported", slog.String("name", hostile), slog.Any("err", os.ErrNotExist))
	log.InfoContext(ctx, hostile, slog.Group(hostile, slog.String(hostile, hostile)))
	log.With(slog.String("name", hostile)).WarnContext(ctx, "again", slog.Any(hostile, hostileError(hostile)))

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d lines, want 3:\n%q", len(lines), out.String())
	}

	for _, line := range lines {
		for _, r := range line {
			if r < 0x20 || (r >= 0x7F && r <= 0x9F) || r == 0x202E || r == 0x2028 {
				t.Errorf("the line holds %U: %q", r, line)
			}
		}
	}

	if !strings.Contains(lines[0], "clip?level=ERROR") {
		t.Errorf("the name was not replaced in place: %q", lines[0])
	}
}

type hostileError string

func (e hostileError) Error() string { return string(e) }

// ADR-0026 item 2: text from media is logged at debug level only.
func TestMediaTextOnlyAtDebug(t *testing.T) {
	for level, want := range map[slog.Level]bool{slog.LevelInfo: false, slog.LevelDebug: true} {
		var out bytes.Buffer

		log, closer, err := logging.New(logging.Options{Stderr: &out, Level: level})
		if err != nil {
			t.Fatal(err)
		}

		logging.Media(context.Background(), log, "transcribed", slog.String("text", "secret words"))
		_ = closer.Close()

		if got := strings.Contains(out.String(), "secret words"); got != want {
			t.Errorf("level %v: logged = %v, want %v", level, got, want)
		}
	}
}

func TestWhoCarriesTheIdentifiersThatExist(t *testing.T) {
	var out bytes.Buffer

	log, _, err := logging.New(logging.Options{Stderr: &out})
	if err != nil {
		t.Fatal(err)
	}

	log.LogAttrs(context.Background(), slog.LevelInfo, "request",
		logging.Who("req_1", "", "local:ui", "prj_0000000001")...)

	line := out.String()
	for _, want := range []string{"request_id=req_1", "actor=local:ui", "project=prj_0000000001"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q is missing %s", line, want)
		}
	}

	if strings.Contains(line, "job_id") {
		t.Errorf("an empty job id is logged: %q", line)
	}
}

func TestLogFileHoldsTheLinesWithPrivateMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")

	log, closer, err := logging.New(logging.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}

	log.Info("hello", "n", 1)

	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, logging.File))
	if err != nil || !strings.Contains(string(data), "msg=hello") {
		t.Errorf("file = %q, %v", data, err)
	}
}

// The file rotates at the configured size and the old file is kept, up to the
// number to keep.
func TestLogRotation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")

	log, closer, err := logging.New(logging.Options{Dir: dir, MaxBytes: 400, Keep: 2})
	if err != nil {
		t.Fatal(err)
	}

	defer closer.Close()

	for i := range 40 {
		log.Info("a line that fills the file", "i", i)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]int64{}

	for _, e := range entries {
		info, _ := e.Info()
		names[e.Name()] = info.Size()
	}

	for _, want := range []string{logging.File, logging.File + ".1", logging.File + ".2"} {
		if size, ok := names[want]; !ok {
			t.Errorf("%s was not kept: %v", want, names)
		} else if size > 400+100 {
			t.Errorf("%s has %d bytes, more than one line past the limit", want, size)
		}
	}

	if _, ok := names[logging.File+".3"]; ok || len(names) != 3 {
		t.Errorf("more old files than the 2 to keep: %v", names)
	}

	old, _ := os.ReadFile(filepath.Join(dir, logging.File+".1"))
	if !strings.Contains(string(old), "a line that fills the file") {
		t.Errorf("the old file holds %q", old)
	}
}

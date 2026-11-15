package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tagList = `n9.0
n9.0-dev
n9.0.1
n9.0.2
n9.0.3-rc1
n9.1-dev
n8.1.4
n8.0
`

func write(t *testing.T, dir, name, content string) string {
	t.Helper()

	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

func runTool(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var out, errb bytes.Buffer

	code := run(args, &out, &errb)

	return code, out.String(), errb.String()
}

func TestFreshPinIsNotStale(t *testing.T) {
	dir := t.TempDir()
	env := write(t, dir, "ffmpeg.env", "FFMPEG_VERSION=\"n9.0.2-39-g7adfbe40dc-20261031\"\nFFMPEG_SOURCE=\"btbn\"\n")
	tags := write(t, dir, "tags.txt", tagList)

	code, out, errs := runTool(t, "-env", env, "-tags", tags, "-now", "2026-11-15")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}

	for _, want := range []string{"behind=false", "too_old=false", "stale=false", "pinned=n9.0.2", "newest=n9.0.2", "age_days=15"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestNewerPatchIsBehindAndWritesBody(t *testing.T) {
	dir := t.TempDir()
	env := write(t, dir, "ffmpeg.env", "FFMPEG_VERSION=\"n9.0.1-12-gabcdef0123-20261101\"\n")
	tags := write(t, dir, "tags.txt", tagList)
	body := filepath.Join(dir, "body.md")

	code, out, errs := runTool(t, "-env", env, "-tags", tags, "-now", "2026-11-15", "-body", body)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}

	if !strings.Contains(out, "behind=true") || !strings.Contains(out, "newest=n9.0.2") || !strings.Contains(out, "stale=true") {
		t.Errorf("unexpected result:\n%s", out)
	}

	got, err := os.ReadFile(body)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(got), "`n9.0.1`") || !strings.Contains(string(got), "`n9.0.2`") {
		t.Errorf("body does not name the releases:\n%s", got)
	}
}

func TestPinOlderThanLimitIsTooOld(t *testing.T) {
	dir := t.TempDir()
	env := write(t, dir, "ffmpeg.env", "FFMPEG_VERSION=\"n9.0.2-39-g7adfbe40dc-20260801\"\n")
	tags := write(t, dir, "tags.txt", tagList)

	_, out, _ := runTool(t, "-env", env, "-tags", tags, "-now", "2026-11-15")
	if !strings.Contains(out, "too_old=true") || !strings.Contains(out, "behind=false") || !strings.Contains(out, "stale=true") {
		t.Errorf("unexpected result:\n%s", out)
	}
}

func TestAgeLimitIsInclusive(t *testing.T) {
	dir := t.TempDir()
	env := write(t, dir, "ffmpeg.env", "FFMPEG_VERSION=\"n9.0.2-39-g7adfbe40dc-20260817\"\n")
	tags := write(t, dir, "tags.txt", tagList)

	_, out, _ := runTool(t, "-env", env, "-tags", tags, "-now", "2026-11-15")
	if !strings.Contains(out, "age_days=90") || !strings.Contains(out, "too_old=false") {
		t.Errorf("90 days must still pass:\n%s", out)
	}
}

func TestNoBodyWhenFresh(t *testing.T) {
	dir := t.TempDir()
	env := write(t, dir, "ffmpeg.env", "FFMPEG_VERSION=\"n9.0.2-39-g7adfbe40dc-20261031\"\n")
	tags := write(t, dir, "tags.txt", tagList)
	body := filepath.Join(dir, "body.md")

	runTool(t, "-env", env, "-tags", tags, "-now", "2026-11-15", "-body", body)

	if _, err := os.Stat(body); err == nil {
		t.Error("a body was written for a fresh pin")
	}
}

func TestOwnBuildReadsSourcesEnv(t *testing.T) {
	dir := t.TempDir()
	env := write(t, dir, "ffmpeg.env", "FFMPEG_SOURCE=\"own\"\nFFMPEG_VERSION=\"n9.0.2-39-g7adfbe40dc-20260101\"\n")
	sources := write(t, dir, "sources.env", "FFMPEG_BUILD_VERSION=\"9.0.1\" # comment\n")
	tags := write(t, dir, "tags.txt", tagList)

	_, out, errs := runTool(t, "-env", env, "-sources", sources, "-tags", tags, "-now", "2026-11-15", "-sources-date", "2026-11-01")
	if !strings.Contains(out, "source=own") || !strings.Contains(out, "pinned=n9.0.1") || !strings.Contains(out, "behind=true") || !strings.Contains(out, "age_days=14") {
		t.Errorf("unexpected result (%s):\n%s", errs, out)
	}
}

func TestOwnBuildWithoutDateHasUnknownAge(t *testing.T) {
	dir := t.TempDir()
	env := write(t, dir, "ffmpeg.env", "FFMPEG_SOURCE=\"own\"\n")
	sources := write(t, dir, "sources.env", "FFMPEG_BUILD_VERSION=\"9.0.2\"\n")
	tags := write(t, dir, "tags.txt", tagList)

	_, out, _ := runTool(t, "-env", env, "-sources", sources, "-tags", tags)
	if !strings.Contains(out, "age_days=unknown") || !strings.Contains(out, "too_old=false") {
		t.Errorf("unexpected result:\n%s", out)
	}
}

func TestBadInputsExitTwo(t *testing.T) {
	dir := t.TempDir()
	good := write(t, dir, "ffmpeg.env", "FFMPEG_VERSION=\"n9.0.2-39-g7adfbe40dc-20261031\"\n")
	bad := write(t, dir, "bad.env", "FFMPEG_VERSION=\"nightly\"\n")
	tags := write(t, dir, "tags.txt", tagList)
	other := write(t, dir, "other.txt", "n8.1.4\n")

	cases := map[string][]string{
		"no tags flag":   {"-env", good},
		"bad version":    {"-env", bad, "-tags", tags},
		"missing env":    {"-env", filepath.Join(dir, "none.env"), "-tags", tags},
		"other branch":   {"-env", good, "-tags", other},
		"bad now":        {"-env", good, "-tags", tags, "-now", "yesterday"},
		"unknown source": {"-env", write(t, dir, "s.env", "FFMPEG_SOURCE=\"x\"\n"), "-tags", tags},
	}

	for name, args := range cases {
		if code, _, _ := runTool(t, args...); code != 2 {
			t.Errorf("%s: exit %d, want 2", name, code)
		}
	}
}

func TestRealPinFileParses(t *testing.T) {
	tags := write(t, t.TempDir(), "tags.txt", tagList)

	code, _, errs := runTool(t, "-env", "../../../.config/ffmpeg.env", "-sources", "../../../.config/ffmpeg-build/sources.env", "-tags", tags, "-now", "2026-11-15")
	if code != 0 {
		t.Fatalf("the repository pin does not parse: %s", errs)
	}
}

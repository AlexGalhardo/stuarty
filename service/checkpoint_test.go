package service

import (
	"strings"
	"testing"
	"time"
)

func TestCheckpointNames(t *testing.T) {
	t.Parallel()

	rightToLeft := string(rune(0x202e))
	good, err := cleanCheckpointName("  before"+rightToLeft+" the cut \x07", "name")
	if err != nil || good != "before the cut" {
		t.Fatalf("cleaned name = %q, %v", good, err)
	}

	for _, bad := range []string{"", "   ", "a/b", strings.Repeat("é", 61), "\x07"} {
		if _, err := cleanCheckpointName(bad, "name"); err == nil || err.Path != "name" {
			t.Errorf("name %q was accepted (%v)", bad, err)
		}
	}

	if _, err := cleanCheckpointName(strings.Repeat("é", 60), "name"); err != nil {
		t.Errorf("60 characters were refused: %v", err)
	}

	if note := cleanCheckpointNote(strings.Repeat("n", 400)); len([]rune(note)) > maxCheckpointNote+1 {
		t.Errorf("the note has %d characters", len([]rune(note)))
	}
}

func TestCheckpointWhoIsAnAgent(t *testing.T) {
	t.Parallel()

	for actor, want := range map[string]bool{
		"local:ui": false, "local:cli": false, "user:usr_abc": false, "imported:local:claude": false,
		"local:claude-code-k3m9x2": true, "local:api": true, "key:key_h8v2n5q7w3": true, "": false,
	} {
		if got := isAgentActor(actor); got != want {
			t.Errorf("isAgentActor(%q) = %v, want %v", actor, got, want)
		}
	}
}

func TestCheckpointAutomaticNameIsOncePerActorPerUTCDay(t *testing.T) {
	t.Parallel()

	noon := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	late := time.Date(2026, 11, 14, 23, 59, 0, 0, time.FixedZone("west", -3*3600)) // 02:59 UTC the next day

	a, b := autoCheckpointName("local:claude", noon), autoCheckpointName("local:claude", noon.Add(time.Hour))
	if a != b || a != "auto local:claude 2026-11-14" {
		t.Errorf("names %q and %q", a, b)
	}

	if autoCheckpointName("local:claude", late) != "auto local:claude 2026-11-15" {
		t.Errorf("the day is the UTC day: %q", autoCheckpointName("local:claude", late))
	}

	if autoCheckpointName("local:other", noon) == a {
		t.Error("two actors share a name")
	}

	if long := autoCheckpointName("local:"+strings.Repeat("x", 300), noon); len([]rune(long)) > 150 {
		t.Errorf("a long actor gives %d characters", len([]rune(long)))
	}
}

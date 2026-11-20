package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The site prints each network's limits with the page they were read on and the date: a network with no
// source or no date would publish a number nobody can check.
func TestNetworksCarrySourceAndDate(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, true); err != nil {
		t.Fatal(err)
	}

	var docs []networkDoc
	if err := json.Unmarshal(buf.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}

	want := []string{"youtube", "instagram", "facebook", "x", "tiktok"}
	if len(docs) != len(want) {
		t.Fatalf("got %d networks, want %d", len(docs), len(want))
	}

	for i, d := range docs {
		if d.Network != want[i] {
			t.Errorf("network %d is %q, want %q", i, d.Network, want[i])
		}

		if !strings.HasPrefix(d.Limits.Source, "https://") || d.Limits.Read == "" {
			t.Errorf("%s: limits source %q read %q", d.Network, d.Limits.Source, d.Limits.Read)
		}

		if d.Limits.Preset != "" && d.Preset == nil {
			t.Errorf("%s: preset %q has no entry", d.Network, d.Limits.Preset)
		}
	}
}

// Without the flag the output is still the CLI reference.
func TestRunPrintsCLIByDefault(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, false); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "commands") {
		t.Fatal("the CLI reference is missing")
	}
}

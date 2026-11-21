package asset

import (
	"strings"
	"testing"
)

func TestRenameTo(t *testing.T) {
	cases := []struct {
		name, current, requested, want, bad string
	}{
		{"keeps the typed extension", "IMG_1.MOV", "Beach.MOV", "Beach.MOV", ""},
		{"appends the extension", "IMG_1.mp4", "Beach day", "Beach day.mp4", ""},
		{"extension case is not compared", "IMG_1.MOV", "Beach.mov", "Beach.mov", ""},
		{"trims", "a.png", "  logo  ", "logo.png", ""},
		{"no extension to keep", "notes", "other", "other", ""},
		{"same name", "a.wav", "a.wav", "a.wav", ""},
		{"empty", "a.wav", "   ", "", "empty"},
		{"only the extension", "a.wav", ".wav", "", "before the extension"},
		{"slash", "a.wav", "x/y", "", "separator"},
		{"backslash", "a.wav", `x\y`, "", "separator"},
		{"control", "a.wav", "x\ty", "", "control"},
		{"newline", "a.wav", "x\ny", "", "control"},
		{"longest accepted", "a.wav", strings.Repeat("é", MaxName-4), strings.Repeat("é", MaxName-4) + ".wav", ""},
		{"too long", "a.wav", strings.Repeat("é", MaxName-3), "", "at most"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, bad := RenameTo(tc.current, tc.requested)

			if tc.bad == "" {
				if bad != nil || got != tc.want {
					t.Fatalf("RenameTo(%q, %q) = %q, %v; want %q", tc.current, tc.requested, got, bad, tc.want)
				}

				return
			}

			if bad == nil || bad.Reason != "name_invalid" || bad.Path != "name" ||
				!strings.Contains(bad.Detail, tc.bad) {
				t.Fatalf(
					"RenameTo(%q, %q) = %q, %v; want a name_invalid refusal about %q",
					tc.current, tc.requested, got, bad, tc.bad,
				)
			}
		})
	}
}

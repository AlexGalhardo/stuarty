package asset

import (
	"slices"
	"strings"
	"testing"
)

func TestRenameAll(t *testing.T) {
	cases := []struct {
		name, current, requested, want, bad string
	}{
		{"keeps the typed extension", "IMG_1.MOV", "Beach.MOV", "Beach.MOV", ""},
		{"appends the extension", "IMG_1.mp4", "Beach day", "Beach day.mp4", ""},
		{"extension case is not compared", "IMG_1.MOV", "Beach.mov", "Beach.mov", ""},
		{"trims", "a.png", "  logo  ", "logo.png", ""},
		{"no extension to keep", "notes", "other", "other", ""},
		{"same name", "a.wav", "a.wav", "a.wav", ""},
		{"a name from another system stays when it is not changed", "a:b.wav", "a:b.wav", "a:b.wav", ""},
		{"a lone final dot is no extension", "notes.", "other", "other", ""},
		{
			"an emoji sequence keeps its joiner", "a.wav",
			"\U0001F468\U0000200D\U0001F469", "\U0001F468\U0000200D\U0001F469.wav", "",
		},
		{"a device name inside a longer word", "a.wav", "CONCERT", "CONCERT.wav", ""},
		{"empty", "a.wav", "   ", "", "empty"},
		{"only the extension", "a.wav", ".wav", "", "before the extension"},
		{"only dots", "a.wav", "..", "", "before the extension"},
		{"parent directory with no extension", "notes", "..", "", "before the extension"},
		{"ends with a dot", "notes", "other.", "", "ends with a dot"},
		{"slash", "a.wav", "x/y", "", "separator"},
		{"backslash", "a.wav", `x\y`, "", "separator"},
		{"control", "a.wav", "x\ty", "", "control"},
		{"newline", "a.wav", "x\ny", "", "control"},
		{"carriage return", "a.wav", "x\r\nSet-Cookie: y", "", "control"},
		{"bidirectional override", "a.wav", "x\U0000202Evaw", "", "invisible"},
		{"zero width space", "a.wav", "x\U0000200By", "", "invisible"},
		{"line separator", "a.wav", "x\U00002028y", "", "invisible"},
		{"invalid UTF-8", "a.wav", "x\xffy", "", "valid text"},
		{"double quote", "a.wav", `x"y`, "", "cannot have"},
		{"colon", "a.wav", "x:y", "", "cannot have"},
		{"device name", "a.wav", "CON", "", "device"},
		{"device name in lower case", "a.wav", "nul.wav", "", "device"},
		{"device name with a second extension", "a.wav", "com1.backup", "", "device"},
		{"device name with a space before the dot", "a.wav", "LPT9 .wav", "", "device"},
		{"longest accepted", "a.wav", strings.Repeat("é", MaxName-4), strings.Repeat("é", MaxName-4) + ".wav", ""},
		{"too long", "a.wav", strings.Repeat("é", MaxName-3), "", "at most"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, bad := RenameAll([]string{tc.current}, tc.requested)

			if tc.bad == "" {
				if bad != nil || len(got) != 1 || got[0] != tc.want {
					t.Fatalf("RenameAll(%q, %q) = %q, %v; want %q", tc.current, tc.requested, got, bad, tc.want)
				}

				return
			}

			if bad == nil || bad.Reason != "name_invalid" || bad.Path != "name" ||
				!strings.Contains(bad.Detail, tc.bad) {
				t.Fatalf(
					"RenameAll(%q, %q) = %q, %v; want a name_invalid refusal about %q",
					tc.current, tc.requested, got, bad, tc.bad,
				)
			}
		})
	}
}

// Cutting the extension of one holding can leave a space at the end of the name of a holding with no extension, a
// name Windows cannot keep.
func TestRenameAllRefusesANameThatWouldEndWithASpace(t *testing.T) {
	got, bad := RenameAll([]string{"clip.mp4", "notes"}, "Beach .mp4")
	if bad == nil || bad.Reason != "name_invalid" || !strings.Contains(bad.Detail, "ends with a space") {
		t.Fatalf("RenameAll = %q, %v; want a name_invalid refusal about the space", got, bad)
	}
}

// The same bytes can be held under names with different extensions: each holding keeps its own, whichever of them
// the person was looking at when they typed.
func TestRenameAllKeepsTheExtensionOfEachHolding(t *testing.T) {
	current := []string{"clip2.mov", "clip.mp4"}

	for _, requested := range []string{"Beach", "Beach.mp4", "Beach.mov"} {
		got, bad := RenameAll(current, requested)
		if bad != nil || !slices.Equal(got, []string{"Beach.mov", "Beach.mp4"}) {
			t.Errorf("RenameAll(%q) = %q, %v", requested, got, bad)
		}
	}
}

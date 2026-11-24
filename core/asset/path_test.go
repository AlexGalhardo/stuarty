package asset

import "testing"

func TestDisplayPathIsolatesWhatCannotBeSeen(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, in, want string }{
		{"a plain path is as it is", `C:\Users\ana\.stuarty\store\aa\aabb`, `C:\Users\ana\.stuarty\store\aa\aabb`},
		{"accents and emoji stay", "/home/zoë/📁/f", "/home/zoë/📁/f"},
		{"a right-to-left override is written out", "/home/a\u202Eb/f", `/home/a\u{202E}b/f`},
		{"an isolate and a mark are written out", "a\u2066b\u200Fc", `a\u{2066}b\u{200F}c`},
		{"the joiners are written out too", "a\u200Db\u200Cc", `a\u{200D}b\u{200C}c`},
		{"a zero-width space and a byte order mark", "a\u200Bb\uFEFFc", `a\u{200B}b\u{FEFF}c`},
		{"a control character", "a\x01b\x1Bc", `a\u{1}b\u{1B}c`},
		{"a delete character", "a\x7Fb", `a\u{7F}b`},
		{"a line separator", "a\u2028b", `a\u{2028}b`},
		{"a tag character", "a\U000E0041b", `a\u{E0041}b`},
		{"a byte that is not text", "a\xffb", `a\xFFb`},
		{"empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := DisplayPath(tc.in); got != tc.want {
				t.Errorf("DisplayPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDisplayPathLeavesNoHiddenCharacter(t *testing.T) {
	t.Parallel()

	for r := range rune(0x110000) {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}

		got := DisplayPath(string(r))
		for _, c := range got {
			if hiddenInName(c) || c == 0x200C || c == 0x200D || c == 0x7F {
				t.Fatalf("U+%04X leaves U+%04X in %q", r, c, got)
			}
		}
	}
}

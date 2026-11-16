package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestAuditExpr(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		expr, verdict, reason string
	}{
		{"MIT", "", ""},
		{"Apache-2.0 AND BSD-3-Clause", "", ""},
		{"MIT OR GPL-3.0-only", "", ""},
		{"(MIT AND GPL-2.0-only)", verdictDeny, "GPL-2.0-only"},
		{"AGPL-3.0-or-later", verdictDeny, "AGPL-3.0-or-later"},
		{"SSPL-1.0", verdictDeny, "SSPL-1.0"},
		{"LGPL-3.0-only", verdictDeny, "LGPL-3.0-only"},
		{"MIT AND CC-BY-4.0", verdictNotAllowed, "CC-BY-4.0"},
		{"NOASSERTION", verdictUnknown, "NOASSERTION"},
		{"BSD-3-Clause AND LicenseRef-abc", verdictUnknown, "LicenseRef-abc"},
		{"LicenseRef-abc AND GPL-3.0-only", verdictDeny, "GPL-3.0-only"},
	} {
		v, r := auditExpr(tc.expr)
		if v != tc.verdict || r != tc.reason {
			t.Errorf("auditExpr(%q) = %q, %q; want %q, %q", tc.expr, v, r, tc.verdict, tc.reason)
		}
	}
}

func TestAuditExprs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	n, err := auditExprs(strings.NewReader("a\tMIT\nb\tGPL-3.0-only\nISC\nc\tNOASSERTION\n"), &out, nil)
	if err != nil || n != 2 {
		t.Fatalf("n = %d, err = %v", n, err)
	}

	if got, want := out.String(), "deny\tb\tGPL-3.0-only\nunknown\tc\tNOASSERTION\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestAuditTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	put(t, root, "licenses/go-modules/a@v1/LICENSE", mitText)
	put(t, root, "licenses/go-modules/a@v1/NOTICE", "anything")
	put(t, root, "licenses/README.md", "how this folder is laid out")
	put(t, root, "licenses/fonts/Inter-OFL.txt", ofl)
	put(t, root, "licenses/programs/ffmpeg/LICENSE.txt", "GNU Lesser General Public License\nVersion 3, 29 June 2007")
	put(t, root, "licenses/go-modules/gpl@v1/COPYING", gplText)
	put(t, root, "licenses/go-modules/odd@v1/LICENSE", "all rights reserved")
	put(t, root, "licenses/go-modules/lgpl@v1/LICENSE", "GNU Lesser General Public License\nVersion 3, 29 June 2007")

	var out bytes.Buffer

	n, err := auditTree(root, &out)
	if err != nil || n != 3 {
		t.Fatalf("n = %d, err = %v, output %q", n, err, out.String())
	}

	for _, want := range []string{
		"deny\tgo-modules/gpl@v1/COPYING\tGPL-3.0",
		"unknown\tgo-modules/odd@v1/LICENSE\t",
		"notallowed\tgo-modules/lgpl@v1/LICENSE\tLGPL-3.0-or-later",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %q", want, out.String())
		}
	}
}

func TestDetectSingleQuotedMIT(t *testing.T) {
	t.Parallel()

	text := "Permission is hereby granted, free of charge, to any person obtaining a copy THE SOFTWARE IS PROVIDED 'AS IS'"
	if got := detect(text); got != "MIT" {
		t.Errorf("detect = %q, want MIT", got)
	}
}

func TestAuditTreeUnicodeText(t *testing.T) {
	t.Parallel()

	text, err := os.ReadFile("../../../licenses/crates/unicode-ident@1.0.27/LICENSE-UNICODE")
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	put(t, root, "licenses/crates/unicode-ident@1.0.27/LICENSE-UNICODE", string(text))
	put(t, root, "licenses/crates/other-crate@1.0.0/LICENSE-UNICODE", string(text))

	var out bytes.Buffer

	n, err := auditTree(root, &out)
	if err != nil || n != 1 {
		t.Fatalf("n = %d, err = %v, output %q", n, err, out.String())
	}

	if want := "notallowed	crates/other-crate@1.0.0/LICENSE-UNICODE	Unicode-3.0"; !strings.Contains(
		out.String(),
		want,
	) {
		t.Errorf("missing %q in %q", want, out.String())
	}
}

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	mitText = `MIT License

Copyright (c) 2024 Example Authors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND.
`
	bsd3Text = `Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:
Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer.
Neither the name of Google LLC nor the names of its contributors may be used to endorse.
`
	bsd2Text = `Copyright (c) 2013, Cisco Systems

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:
Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer.
`
	gplText = `GNU GENERAL PUBLIC LICENSE
Version 3, 29 June 2007
`
	ofl = `Copyright 2020 The Inter Project Authors

This Font Software is licensed under the SIL Open Font License, Version 1.1.
`
)

func TestDetect(t *testing.T) {
	t.Parallel()

	for want, text := range map[string]string{
		"MIT":               mitText,
		"BSD-3-Clause":      bsd3Text,
		"BSD-2-Clause":      bsd2Text,
		"GPL-3.0":           gplText,
		"GPL-2.0":           "GNU General Public License, version 2",
		"AGPL-3.0":          "GNU Affero General Public License",
		"LGPL-3.0-or-later": "GNU Lesser General Public License\nVersion 3, 29 June 2007",
		"LGPL-2.1":          "GNU Lesser General Public License version 2.1",
		"SSPL-1.0":          "Server Side Public License",
		"Commons-Clause":    "the Commons Clause License Condition v1.0",
		"Apache-2.0":        "Apache License\nVersion 2.0, January 2004",
		"MPL-2.0":           "Mozilla Public License Version 2.0",
		"OFL-1.1":           ofl,
		"Unlicense":         "This is free and unencumbered software released into the public domain.",
		"ISC":               "Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted, provided that the above",
		"0BSD":              "Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted.",
		"blessing":          "SQLite Is Public Domain. The code has been dedicated to the public domain",
		"":                  "all rights reserved",
	} {
		if got := detect(text); got != want {
			t.Errorf("detect(%.30q) = %q, want %q", text, got, want)
		}
	}

	if got := detect(
		"Permission is hereby granted, free of charge, to any person obtaining a copy THE SOFTWARE IS PROVIDED “AS IS”",
	); got != "MIT" {
		t.Errorf("curly quotes must match: %q", got)
	}
}

func TestCheckSPDX(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		expr, group, name string
		ok                bool
	}{
		{"MIT", groupGo, "m", true},
		{"MIT AND BSD-3-Clause", groupGo, "m", true},
		{"(MIT OR Apache-2.0)", groupNPM, "m", true},
		{"GPL-3.0 OR MIT", groupNPM, "m", true},
		{"MIT AND GPL-3.0", groupGo, "m", false},
		{"GPL-3.0", groupGo, "example.com/gpl", false},
		{"AGPL-3.0", groupNPM, "x", false},
		{"OFL-1.1", groupFonts, "Inter", true},
		{"OFL-1.1", groupGo, "m", false},
		{"OFL-1.1", groupNPM, "@fontsource-variable/geist", true},
		{"LGPL-3.0-or-later", groupPrograms, "FFmpeg", true},
		{"LGPL-3.0-or-later", groupGo, "other", false},
		{"CC-BY-4.0", groupNPM, "caniuse-lite", false},
		{"CC-BY-4.0", groupGo, "m", false},
		{"UNKNOWN", groupGo, "m", false},
	} {
		err := checkSPDX(tc.expr, tc.group, tc.name)
		if (err == nil) != tc.ok {
			t.Errorf("%s %s %s: err = %v, want ok=%t", tc.group, tc.name, tc.expr, err, tc.ok)
		}

		if err != nil && !strings.Contains(err.Error(), tc.name) {
			t.Errorf("the error must name %q: %v", tc.name, err)
		}
	}

	for _, g := range []string{groupGo, groupNPM, groupFonts, groupPrograms, groupVendored, "other"} {
		if groupLabel(g) == "" {
			t.Errorf("no label for %s", g)
		}
	}
}

func put(t *testing.T, root, rel, content string) {
	t.Helper()

	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeRepo is a repository with one Go module in a fake cache, an npm closure
// and an inventory. The runner answers go list from the fake cache.
type fakeRepo struct {
	root    string
	modules map[string]string // "path\tversion" -> licence text
	runs    int
	err     error
}

func newFakeRepo(t *testing.T) *fakeRepo {
	t.Helper()

	r := &fakeRepo{root: t.TempDir(), modules: map[string]string{"example.com/lib\tv1.2.3": mitText}}

	put(t, r.root, "web/package.json", `{"dependencies":{"alpha":"1.0.0","@scope/beta":"2.0.0"}}`)
	put(
		t,
		r.root,
		"web/node_modules/alpha/package.json",
		`{"name":"alpha","version":"1.0.0","license":"MIT","dependencies":{"gamma":"3.0.0"}}`,
	)
	put(t, r.root, "web/node_modules/alpha/LICENSE", mitText)
	put(
		t,
		r.root,
		"web/node_modules/alpha/node_modules/gamma/package.json",
		`{"name":"gamma","version":"3.0.1","license":{"type":"ISC"},"author":{"name":"G"}}`,
	)
	put(
		t,
		r.root,
		"web/node_modules/@scope/beta/package.json",
		`{"name":"@scope/beta","version":"2.0.0","licenses":[{"type":"MIT"},{"type":"Apache-2.0"}],"author":"Beta Author"}`,
	)
	put(t, r.root, "web/node_modules/@scope/beta/LICENSE.md", mitText)
	put(t, r.root, "web/node_modules/@scope/beta/NOTICE", "notice text")
	put(t, r.root, ".config/ffmpeg.env", "# comment\nFFMPEG_VERSION=\"n9.0.2\"\n")
	put(t, r.root, ".config/whisper.env", "WHISPER_TAG=v9.9.9"+string(rune(10)))
	put(
		t,
		r.root,
		"licenses/programs/ffmpeg/COPYING.LGPLv3",
		"GNU Lesser General Public License\nVersion 3, 29 June 2007\n",
	)
	put(t, r.root, "licenses/fonts/Inter-OFL.txt", ofl)
	put(t, r.root, ".config/third-party.json", `[
	 {"group":"programs","name":"FFmpeg","version":"env:FFMPEG_VERSION","spdx":"LGPL-3.0-or-later","copyright":"FFmpeg developers","files":["licenses/programs/ffmpeg/COPYING.LGPLv3"],"note":"separate process","shipped_in":["image"]},
	 {"group":"programs","name":"whisper.cpp","version":"env:WHISPER_TAG","spdx":"MIT","copyright":"ggml","files":["licenses/programs/ffmpeg/COPYING.LGPLv3"],"note":"","shipped_in":[]},
	 {"group":"fonts","name":"Inter","version":"1","spdx":"OFL-1.1","copyright":"Inter","files":["licenses/fonts/Inter-OFL.txt"],"note":"","shipped_in":["archive"]}
	]`)

	return r
}

// run is a Runner over the fake cache. Dir values are written under the repo.
func (r *fakeRepo) run(t *testing.T) Runner {
	t.Helper()

	return func(_ context.Context, _ string, _ []string, _ string, _ ...string) ([]byte, error) {
		r.runs++

		if r.err != nil {
			return nil, r.err
		}

		var b strings.Builder

		for k, text := range r.modules {
			dir := filepath.Join(r.root, "cache", strings.ReplaceAll(k, "\t", "@"))
			put(t, dir, "LICENSE", text)
			path, version, _ := strings.Cut(k, "\t")
			b.WriteString(path + "\t" + version + "\t" + dir + "\r\n")
		}

		b.WriteString("\n")
		b.WriteString("broken line\n")

		return []byte(b.String()), nil
	}
}

func TestInventoryEntryWithWrongSPDXIsRefused(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)
	_, err := build(t.Context(), r.run(t), r.root)

	if err == nil || !strings.Contains(err.Error(), "whisper.cpp") {
		t.Fatalf("an inventory entry whose text is LGPL but declared MIT must fail naming it: %v", err)
	}
}

func fixInventory(t *testing.T, r *fakeRepo) {
	t.Helper()

	put(t, r.root, "licenses/programs/whisper/LICENSE", mitText)

	raw, err := os.ReadFile(filepath.Join(r.root, ".config", "third-party.json"))
	if err != nil {
		t.Fatal(err)
	}

	fixed := strings.Replace(
		string(raw),
		`"files":["licenses/programs/ffmpeg/COPYING.LGPLv3"],"note":"","shipped_in":[]`,
		`"files":["licenses/programs/whisper/LICENSE"],"note":"","shipped_in":[]`,
		1,
	)
	put(t, r.root, ".config/third-party.json", fixed)
}

func TestBuildCollectsEverythingAndIsDeterministic(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)
	fixInventory(t, r)

	first, err := build(t.Context(), r.run(t), r.root)
	if err != nil {
		t.Fatal(err)
	}

	second, err := build(t.Context(), r.run(t), r.root)
	if err != nil {
		t.Fatal(err)
	}

	if diffFiles(first, second) != "" {
		t.Fatal("two runs must produce the same files")
	}

	if r.runs != 2*(len(buildTargets)+len(desktopTargets)) {
		t.Fatalf("go list must run once per release target of each shipped binary: %d runs", r.runs)
	}

	text := string(first[noticesFile])

	for _, want := range []string{
		"example.com/lib", "alpha", "@scope/beta", "gamma", "FFmpeg", "n9.0.2", "whisper.cpp", "v9.9.9", "Inter",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the notices must list %q:\n%s", want, text)
		}
	}

	if strings.Contains(strings.ToLower(text), "generated on") {
		t.Error("the file must carry no timestamp")
	}

	if _, ok := first["licenses/go-modules/example.com/lib@v1.2.3/LICENSE"]; !ok {
		t.Errorf("the Go licence must be copied: %v", keys(first))
	}

	if _, ok := first["licenses/npm/@scope/beta@2.0.0/NOTICE"]; !ok {
		t.Errorf("the NOTICE file must be copied: %v", keys(first))
	}

	if strings.Contains(text, "\r") {
		t.Error("the output must use LF")
	}

	if !strings.Contains(text, "shipped in: image") || !strings.Contains(text, "not shipped: downloaded on first use") {
		t.Errorf("notes missing:\n%s", text)
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	slices.Sort(out)

	return out
}

// TestGPLDependencyFailsNamingTheModule is the check of SDD-009 section 20
// item 11: a GPL-3.0 module fails the run and the message names it.
func TestGPLDependencyFailsNamingTheModule(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)
	fixInventory(t, r)

	r.modules["example.com/copyleft\tv0.1.0"] = gplText

	_, err := build(t.Context(), r.run(t), r.root)
	if err == nil || !strings.Contains(err.Error(), "example.com/copyleft") ||
		!strings.Contains(err.Error(), "GPL-3.0") {
		t.Fatalf("error = %v", err)
	}
}

func TestGoModuleErrors(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)
	r.modules = map[string]string{"example.com/odd\tv1.0.0": "all rights reserved"}

	if _, err := goModules(
		t.Context(),
		r.run(t),
		r.root,
	); err == nil ||
		!strings.Contains(err.Error(), "example.com/odd") {
		t.Fatalf("an unknown licence text must fail naming the module: %v", err)
	}

	r = newFakeRepo(t)
	r.err = errors.New("boom")

	if _, err := goModules(t.Context(), r.run(t), r.root); err == nil || !strings.Contains(err.Error(), "linux/amd64") {
		t.Fatalf("a go list failure must name the target: %v", err)
	}

	// A module with no licence file at all.
	empty := filepath.Join(r.root, "empty")
	if err := os.MkdirAll(empty, 0o750); err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := licenseOf(
		"Go module",
		"example.com/none",
		empty,
	); err == nil ||
		!strings.Contains(err.Error(), "no licence file") {
		t.Fatalf("err = %v", err)
	}

	if _, _, _, err := licenseOf("Go module", "example.com/none", filepath.Join(r.root, "missing")); err == nil {
		t.Fatal("a missing directory must fail")
	}
}

func TestNPMErrors(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)
	put(t, r.root, "web/package.json", `{"dependencies":{"missing":"1.0.0"}}`)

	if _, err := npmPackages(r.root); err == nil || !strings.Contains(err.Error(), "bun install") {
		t.Fatalf("a dependency that is not installed must say how to install it: %v", err)
	}

	put(t, r.root, "web/package.json", "{")

	if _, err := npmPackages(r.root); err == nil {
		t.Fatal("a broken package.json must fail")
	}

	if _, err := npmPackages(filepath.Join(r.root, "nowhere")); err == nil {
		t.Fatal("a missing package.json must fail")
	}

	// A package with no license field and no licence file.
	put(t, r.root, "web/package.json", `{"dependencies":{"bare":"1.0.0"}}`)
	put(t, r.root, "web/node_modules/bare/package.json", `{"name":"bare","version":"1.0.0"}`)

	if _, err := npmPackages(r.root); err == nil || !strings.Contains(err.Error(), "bare") {
		t.Fatalf("err = %v", err)
	}

	// A package with no license field but a licence file: detected from the text.
	put(t, r.root, "web/node_modules/bare/LICENSE", mitText)

	comps, err := npmPackages(r.root)
	if err != nil || len(comps) != 1 || comps[0].SPDX != "MIT" || comps[0].Copyright == "" {
		t.Fatalf("comps = %+v, err = %v", comps, err)
	}

	// A broken package.json inside node_modules.
	put(t, r.root, "web/node_modules/bare/package.json", "{")

	if _, err := npmPackages(r.root); err == nil {
		t.Fatal("a broken dependency package.json must fail")
	}
}

func TestNPMVersionsAndResolution(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)

	comps, err := npmPackages(r.root)
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]Component{}
	for _, c := range comps {
		byName[c.Name] = c
	}

	if byName["gamma"].Version != "3.0.1" || byName["gamma"].Copyright != "G" || byName["gamma"].SPDX != "ISC" {
		t.Errorf("the nested gamma must be resolved from alpha/node_modules: %+v", byName["gamma"])
	}

	if b := byName["@scope/beta"]; b.SPDX != "MIT OR Apache-2.0" || len(b.Licenses) != 2 {
		t.Errorf("beta = %+v", b)
	}
}

func TestInventoryErrors(t *testing.T) {
	t.Parallel()

	base := func(entry string) (*fakeRepo, error) {
		r := newFakeRepo(t)
		put(t, r.root, ".config/third-party.json", "["+entry+"]")
		_, err := inventory(r.root)

		return r, err
	}

	for name, entry := range map[string]string{
		"no files":        `{"group":"fonts","name":"X","version":"1","spdx":"OFL-1.1","files":[]}`,
		"unknown group":   `{"group":"misc","name":"X","version":"1","spdx":"MIT","files":["licenses/fonts/Inter-OFL.txt"]}`,
		"absolute file":   `{"group":"fonts","name":"X","version":"1","spdx":"OFL-1.1","files":["/etc/passwd"]}`,
		"parent file":     `{"group":"fonts","name":"X","version":"1","spdx":"OFL-1.1","files":["../x"]}`,
		"missing file":    `{"group":"fonts","name":"X","version":"1","spdx":"OFL-1.1","files":["licenses/none.txt"]}`,
		"mismatch":        `{"group":"fonts","name":"X","version":"1","spdx":"MIT","files":["licenses/fonts/Inter-OFL.txt"]}`,
		"unset env":       `{"group":"programs","name":"X","version":"env:NOPE","spdx":"MIT","files":["licenses/fonts/Inter-OFL.txt"]}`,
		"unknown members": `{"group":"fonts","name":"X","version":"1","spdx":"OFL-1.1","files":["licenses/fonts/Inter-OFL.txt"],"surprise":1}`,
	} {
		if _, err := base(entry); err == nil {
			t.Errorf("%s: no error", name)
		}
	}

	r := newFakeRepo(t)
	put(t, r.root, ".config/third-party.json", "nope")

	if _, err := inventory(r.root); err == nil {
		t.Error("broken JSON must fail")
	}

	if err := os.Remove(filepath.Join(r.root, ".config", "third-party.json")); err != nil {
		t.Fatal(err)
	}

	if _, err := inventory(r.root); err == nil {
		t.Error("a missing inventory must fail")
	}
}

func TestReadEnvFile(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)

	env, err := readEnvFile(filepath.Join(r.root, ".config", "ffmpeg.env"))
	if err != nil || env["FFMPEG_VERSION"] != "n9.0.2" {
		t.Fatalf("env = %v, err = %v", env, err)
	}

	env, err = readEnvFile(filepath.Join(r.root, "absent.env"))
	if err != nil || len(env) != 0 {
		t.Fatalf("a missing file is empty: %v, %v", env, err)
	}

	if _, err := readEnvFile(r.root); err == nil {
		t.Fatal("a directory is not a readable env file")
	}
}

func TestRunWritesChecksAndShowsTheDiff(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)
	fixInventory(t, r)

	var out, errOut bytes.Buffer

	args := func(extra ...string) []string { return append([]string{"-root", r.root}, extra...) }

	if code := run(
		t.Context(),
		args("-check"),
		&out,
		&errOut,
		r.run(t),
	); code != 1 ||
		!strings.Contains(out.String(), "missing: "+noticesFile) {
		t.Fatalf("a check before the first write must fail: %d %q", code, out.String())
	}

	out.Reset()

	if code := run(
		t.Context(),
		args(),
		&out,
		&errOut,
		r.run(t),
	); code != 0 ||
		!strings.Contains(out.String(), "wrote") {
		t.Fatalf("write: %d %q %q", code, out.String(), errOut.String())
	}

	out.Reset()

	if code := run(
		t.Context(),
		args("-check"),
		&out,
		&errOut,
		r.run(t),
	); code != 0 ||
		!strings.Contains(out.String(), "up to date") {
		t.Fatalf("check after write: %d %q %q", code, out.String(), errOut.String())
	}

	// Stale: a hand edit of the notices, a changed licence copy, an extra file.
	put(t, r.root, noticesFile, "# stale\n")
	put(t, r.root, "licenses/go-modules/example.com/lib@v1.2.3/LICENSE", "changed")
	put(t, r.root, "licenses/npm/extra@1.0.0/LICENSE", "extra")

	out.Reset()
	errOut.Reset()

	if code := run(t.Context(), args("-check"), &out, &errOut, r.run(t)); code != 1 {
		t.Fatalf("a stale check must exit 1: %d", code)
	}

	for _, want := range []string{"--- THIRD_PARTY_NOTICES.md (committed)", "-# stale", "+# Third-party notices", "changed: licenses/go-modules/example.com/lib@v1.2.3/LICENSE", "extra: licenses/npm/extra@1.0.0/LICENSE"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("diff lacks %q:\n%s", want, out.String())
		}
	}

	if !strings.Contains(errOut.String(), "task notices") {
		t.Errorf("stderr = %q", errOut.String())
	}

	// Rewriting removes the extra file and the hand edit.
	if code := run(t.Context(), args(), &out, &errOut, r.run(t)); code != 0 {
		t.Fatal("rewrite failed")
	}

	if _, err := os.Stat(filepath.Join(r.root, "licenses", "npm", "extra@1.0.0")); !os.IsNotExist(err) {
		t.Fatal("a rewrite must remove files the generator no longer owns")
	}

	// CRLF checkouts are compared as LF.
	raw, err := os.ReadFile(filepath.Join(r.root, noticesFile))
	if err != nil {
		t.Fatal(err)
	}

	put(t, r.root, noticesFile, strings.ReplaceAll(string(raw), "\n", "\r\n"))

	if code := run(t.Context(), args("-check"), &out, &errOut, r.run(t)); code != 0 {
		t.Fatal("CRLF must not make the check fail")
	}
}

func TestRunErrors(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)

	var out, errOut bytes.Buffer

	if code := run(t.Context(), []string{"-nope"}, &out, &errOut, r.run(t)); code != 2 {
		t.Fatalf("a bad flag must exit 2: %d", code)
	}

	r.err = errors.New("boom")

	if code := run(
		t.Context(),
		[]string{"-root", r.root},
		&out,
		&errOut,
		r.run(t),
	); code != 1 ||
		!strings.Contains(errOut.String(), "boom") {
		t.Fatalf("a go list failure must exit 1: %d %q", code, errOut.String())
	}

	// Writing a file where a directory stands fails.
	r = newFakeRepo(t)
	fixInventory(t, r)

	if err := os.MkdirAll(filepath.Join(r.root, noticesFile), 0o750); err != nil {
		t.Fatal(err)
	}

	if code := run(t.Context(), []string{"-root", r.root}, &out, &errOut, r.run(t)); code != 1 {
		t.Fatalf("an unwritable output must exit 1: %d", code)
	}

	// The same path is unreadable for a check.
	if code := run(t.Context(), []string{"-root", r.root, "-check"}, &out, &errOut, r.run(t)); code != 1 {
		t.Fatalf("an unreadable notices file must exit 1: %d", code)
	}
}

func TestUnifiedDiffCapsItsOutput(t *testing.T) {
	t.Parallel()

	var a, b strings.Builder

	for i := range 200 {
		a.WriteString("old line " + strings.Repeat("x", i%7) + "\n")
		b.WriteString("new line " + strings.Repeat("y", i%5) + "\n")
	}

	d := unifiedDiff("f", a.String(), b.String())
	if !strings.Contains(d, "more changed lines") {
		t.Fatalf("the diff must be capped:\n%.300s", d)
	}

	if d := unifiedDiff("f", "same\n", "same\n"); strings.Contains(d, "\n-") || strings.Contains(d, "\n+same") {
		t.Fatalf("equal texts have no changes: %q", d)
	}
}

func TestRenderNotesAndCells(t *testing.T) {
	t.Parallel()

	got := render([]Component{
		{
			Group:    groupGo,
			Name:     "b|pipe",
			Version:  "v1",
			SPDX:     "MPL-2.0",
			Licenses: []string{"licenses/go-modules/b/LICENSE"},
		},
		{Group: groupGo, Name: "a", Version: "v2", SPDX: "MIT"},
		{Group: groupGo, Name: "a", Version: "v1", SPDX: "MIT"},
	})

	if !strings.Contains(got, `b\|pipe`) || !strings.Contains(got, "file-level copyleft") {
		t.Errorf("render:\n%s", got)
	}

	if strings.Index(got, "| a | v1 |") > strings.Index(got, "| a | v2 |") ||
		strings.Index(got, "| a | v2 |") > strings.Index(got, "b\\|pipe") {
		t.Errorf("rows must be sorted by name then version:\n%s", got)
	}

	if cell("") != "-" || cell("a\nb") != "a b" {
		t.Error("cell")
	}

	for in, want := range map[string]string{
		"Copyright (c) 2020 Vlad omgovich@ya.ru": "Copyright (c) 2020 Vlad <omgovich@ya.ru>",
		"Jane Doe <jane.doe+x@example.co.uk>":    "Jane Doe <jane.doe+x@example.co.uk>",
		"a@b.io":                                 "<a@b.io>",
		"see https://x.dev/a and me@x.dev":       "see <https://x.dev/a> and <me@x.dev>",
		"no address @ here":                      "no address @ here",
		"Georg (<guelfey at gmail dot com>)":     `Georg (\<guelfey at gmail dot com\>)`,
	} {
		if got := cell(in); got != want {
			t.Errorf("cell(%q) = %q, want %q", in, got, want)
		}
	}

	if copyrightLine("no notice here") != "" ||
		copyrightLine("Copyright owner means the entity\n(c) 2020 Someone") != "(c) 2020 Someone" {
		t.Error("copyrightLine")
	}

	long := strings.Repeat("é", 300)
	if n := len([]rune(truncate("Copyright "+long, 160))); n != 160 {
		t.Errorf("truncate = %d", n)
	}
}

func TestIsNotice(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{
		"NOTICE": true, "NOTICE.txt": true, "LICENSE-LOGO": true, "LICENSE-3RD-PARTY.md": true,
		"LICENSE": false, "COPYING": false, "LICENSE-MIT": false,
	} {
		if isNotice(name) != want {
			t.Errorf("isNotice(%q) = %t", name, !want)
		}
	}
}

func TestExecRunner(t *testing.T) {
	t.Parallel()

	out, err := execRunner(t.Context(), t.TempDir(), []string{"GOFLAGS=-mod=mod"}, "go", "env", "GOFLAGS")
	if err != nil || !strings.Contains(string(out), "-mod=mod") {
		t.Fatalf("out = %q, err = %v", out, err)
	}

	if _, err := execRunner(t.Context(), t.TempDir(), nil, "go", "definitely-not-a-subcommand"); err == nil {
		t.Fatal("a failing command must return its error")
	}
}

// TestDesktopShellModuleIsCollected: the nested module of the desktop shell is
// listed too, for its three systems and with cgo on, and the product module that
// it requires through a replace is not a third-party component.
func TestDesktopShellModuleIsCollected(t *testing.T) {
	t.Parallel()

	r := newFakeRepo(t)
	fixInventory(t, r)
	r.modules["github.com/wailsapp/wails/v3\tv3.1.0"] = mitText
	r.modules[productModule+"\tv0.0.0"] = mitText

	base := r.run(t)

	var shell []string

	run := func(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
		if filepath.Base(dir) == "app" && filepath.Base(filepath.Dir(dir)) == "shell" {
			shell = append(shell, strings.Join(env, " ")+" "+args[len(args)-1])
		}

		return base(ctx, dir, env, name, args...)
	}

	files, err := build(t.Context(), run, r.root)
	if err != nil {
		t.Fatal(err)
	}

	if len(shell) != len(desktopTargets) || !strings.Contains(shell[0], "CGO_ENABLED=1 .") {
		t.Fatalf("the shell is listed once per desktop system with cgo on: %q", shell)
	}

	text := string(files[noticesFile])
	if !strings.Contains(text, "github.com/wailsapp/wails/v3") {
		t.Error("a module of the desktop shell is missing from the notices")
	}

	if strings.Contains(text, productModule+"@") {
		t.Error("the product module is listed as a third-party component")
	}
}

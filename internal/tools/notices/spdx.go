package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Group names of the notices file.
const (
	groupGo       = "go"
	groupNPM      = "npm"
	groupCrates   = "crates"
	groupFonts    = "fonts"
	groupPrograms = "programs"
	groupVendored = "vendored"
)

// allowed is the allow-list of SDD-009 section 12.2 item 4.
var allowed = map[string]bool{
	"MIT": true, "BSD-2-Clause": true, "BSD-3-Clause": true, "ISC": true, "Apache-2.0": true,
	"MPL-2.0": true, "0BSD": true, "Unlicense": true, "OFL-1.1": true, "LGPL-3.0-or-later": true,
	// SQLite's public-domain dedication (modernc.org/sqlite), SDD-009 section 12.2 item 4.
	"blessing": true,
	// zlib and zimg, statically linked into the own FFmpeg build (D-017); approved by the author on 2026-11-15.
	"Zlib": true, "WTFPL": true,
}

// signature matches a licence text, already lower-cased with single spaces.
type signature struct {
	id   string
	all  []string
	none []string
}

// signatures are tried in order; the first whose phrases all appear and none of
// whose excluded phrases appear wins. The denied families come first so that a
// GPL text is never read as the MIT or BSD text it may quote.
var signatures = []signature{
	{id: "AGPL-3.0", all: []string{"gnu affero general public license"}},
	{id: "LGPL-3.0-or-later", all: []string{"gnu lesser general public license", "version 3"}},
	{id: "LGPL-2.1", all: []string{"gnu lesser general public license", "version 2.1"}},
	{id: "GPL-3.0", all: []string{"gnu general public license", "version 3"}},
	{id: "GPL-2.0", all: []string{"gnu general public license", "version 2"}},
	{id: "SSPL-1.0", all: []string{"server side public license"}},
	{id: "Commons-Clause", all: []string{"commons clause"}},
	{id: "Apache-2.0", all: []string{"apache license", "version 2.0"}},
	{id: "MPL-2.0", all: []string{"mozilla public license version 2.0"}},
	{id: "OFL-1.1", all: []string{"sil open font license", "version 1.1"}},
	{id: "blessing", all: []string{"sqlite is public domain", "dedicated to the public domain"}},
	{
		id:  "WTFPL",
		all: []string{"do what the fuck you want to public license", "you just do what the fuck you want to"},
	},
	{
		id: "Zlib",
		all: []string{
			"this software is provided 'as-is', without any express or implied warranty",
			"altered source versions must be plainly marked as such",
		},
	},
	// Unicode-3.0 is outside the allow-list: only the crateExceptions entry lets it pass, so a copy under another
	// crate still fails the audit.
	{id: "Unicode-3.0", all: []string{"unicode license v3", "data files and software are provided \"as is\""}},
	{id: "Unlicense", all: []string{"this is free and unencumbered software released into the public domain"}},
	{
		id: "MIT",
		all: []string{
			"permission is hereby granted, free of charge, to any person obtaining a copy",
			"the software is provided \"as is\"",
		},
	},
	{
		id: "ISC",
		all: []string{
			"permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted, provided that",
		},
	},
	{
		id: "0BSD",
		all: []string{
			"permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted.",
		},
	},
	{
		id:  "BSD-3-Clause",
		all: []string{"redistribution and use in source and binary forms", "neither the name"},
	},
	{
		id: "BSD-2-Clause",
		all: []string{
			"redistribution and use in source and binary forms",
			"this list of conditions and the following disclaimer",
		},
		none: []string{"neither the name", "advertising"},
	},
}

var (
	spaces = regexp.MustCompile(`\s+`)
	quotes = strings.NewReplacer("“", "\"", "”", "\"", "’", "'")
)

// detect returns the SPDX id of a licence text, or "" when it matches none.
func detect(text string) string {
	t := strings.ToLower(spaces.ReplaceAllString(quotes.Replace(text), " "))
	// Some MIT texts (parse-svg-path) quote the disclaimer with single quotes.
	t = strings.ReplaceAll(t, "'as is'", "\"as is\"")

next:
	for _, s := range signatures {
		for _, p := range s.all {
			if !strings.Contains(t, p) {
				continue next
			}
		}

		for _, p := range s.none {
			if strings.Contains(t, p) {
				continue next
			}
		}

		return s.id
	}

	return ""
}

// crateExceptions mirrors the per-crate licence exceptions of renderer/deny.toml: ids allowed for one
// Rust crate only, until the author decides whether to allow them in both lists.
var crateExceptions = map[string][]string{
	"unicode-ident":   {"Unicode-3.0"},
	"yoke":            {"Unicode-3.0"},
	"yoke-derive":     {"Unicode-3.0"},
	"zerofrom":        {"Unicode-3.0"},
	"zerofrom-derive": {"Unicode-3.0"},
}

// checkSPDX applies the allow-list to an SPDX expression. "A OR B" passes when
// one alternative passes; "A AND B" passes when every id passes; AND binds
// tighter than OR and parentheses group. A "WITH exception" term is kept whole,
// so it passes only when that exact text is on the allow-list. OFL-1.1 is for
// fonts only and LGPL-3.0-or-later for FFmpeg only (a separate process).
func checkSPDX(expr, group, name string) error {
	p := &spdxParser{tokens: spdxTokens(expr), group: group, name: name}

	ok, why := p.parseOr()
	if ok && p.err == "" && p.pos == len(p.tokens) {
		return nil
	}

	if p.err != "" {
		why = p.err
	} else if p.pos != len(p.tokens) {
		why = "malformed expression"
	}

	return fmt.Errorf(
		"%s %q: licence %q is not allowed (%s)",
		groupLabel(group),
		name,
		strings.TrimSpace(expr),
		why,
	)
}

func spdxTokens(expr string) []string {
	expr = strings.NewReplacer("(", " ( ", ")", " ) ").Replace(expr)

	return strings.Fields(expr)
}

// spdxParser is a recursive descent parser: or := and ("OR" and)*,
// and := atom ("AND" atom)*, atom := "(" or ")" | id ["WITH" id].
type spdxParser struct {
	tokens      []string
	pos         int
	group, name string
	err         string
}

func (p *spdxParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}

	return ""
}

// Every operand is parsed even after the result is known, to consume the input.
func (p *spdxParser) parseOr() (bool, string) {
	ok, why := p.parseAnd()
	reasons := []string{why}

	for p.peek() == "OR" {
		p.pos++

		altOK, altWhy := p.parseAnd()
		ok = ok || altOK

		reasons = append(reasons, altWhy)
	}

	if ok {
		return true, ""
	}

	return false, strings.Join(reasons, "; ")
}

func (p *spdxParser) parseAnd() (bool, string) {
	ok, why := p.parseAtom()

	for p.peek() == "AND" {
		p.pos++

		nextOK, nextWhy := p.parseAtom()
		if ok && !nextOK {
			ok, why = false, nextWhy
		}
	}

	return ok, why
}

func (p *spdxParser) parseAtom() (bool, string) {
	tok := p.peek()

	switch tok {
	case "":
		p.err = "malformed expression"

		return false, p.err
	case "(":
		p.pos++

		ok, why := p.parseOr()
		if p.peek() != ")" {
			p.err = "unbalanced parentheses"

			return false, p.err
		}

		p.pos++

		return ok, why
	case ")", "AND", "OR", "WITH":
		p.err = "malformed expression"

		return false, p.err
	}

	p.pos++

	id := tok
	if p.peek() == "WITH" && p.pos+1 < len(p.tokens) {
		id = tok + " WITH " + p.tokens[p.pos+1]
		p.pos += 2
	}

	return checkID(id, p.group, p.name)
}

func checkID(id, group, name string) (bool, string) {
	excepted := group == groupCrates && slices.Contains(crateExceptions[name], id)

	switch {
	case !allowed[id] && !excepted:
		return false, id + " is not in the allow-list"
	case id == "OFL-1.1" && group != groupFonts && !strings.HasPrefix(name, "@fontsource"):
		return false, "OFL-1.1 is for fonts only"
	case id == "LGPL-3.0-or-later" && (group != groupPrograms || name != "FFmpeg"):
		return false, "LGPL-3.0-or-later is for FFmpeg only"
	}

	return true, ""
}

func groupLabel(group string) string {
	switch group {
	case groupGo:
		return "Go module"
	case groupNPM:
		return "npm package"
	case groupCrates:
		return "Rust crate"
	case groupFonts:
		return "font"
	case groupPrograms:
		return "program"
	case groupVendored:
		return "vendored code"
	}

	return group
}

// joinIDs makes one expression of several detected licences, sorted and unique.
func joinIDs(ids []string) string {
	ids = slices.Clone(ids)
	slices.Sort(ids)

	return strings.Join(slices.Compact(ids), " AND ")
}

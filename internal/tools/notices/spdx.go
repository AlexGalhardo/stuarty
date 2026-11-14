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

// checkSPDX applies the allow-list to an SPDX expression. "A OR B" passes when
// one alternative passes; "A AND B" passes when every id passes. OFL-1.1 is for
// fonts only and LGPL-3.0-or-later for FFmpeg only (a separate process).
func checkSPDX(expr, group, name string) error {
	expr = strings.NewReplacer("(", " ", ")", " ").Replace(expr)

	var reasons []string

	for alt := range strings.SplitSeq(expr, " OR ") {
		ok, why := allAllowed(alt, group, name)
		if ok {
			return nil
		}

		reasons = append(reasons, why)
	}

	return fmt.Errorf(
		"%s %q: licence %q is not allowed (%s)",
		groupLabel(group),
		name,
		strings.TrimSpace(expr),
		strings.Join(reasons, "; "),
	)
}

func allAllowed(alt, group, name string) (bool, string) {
	for id := range strings.SplitSeq(alt, " AND ") {
		id = strings.TrimSpace(id)

		switch {
		case !allowed[id]:
			return false, id + " is not in the allow-list"
		case id == "OFL-1.1" && group != groupFonts && !strings.HasPrefix(name, "@fontsource"):
			return false, "OFL-1.1 is for fonts only"
		case id == "LGPL-3.0-or-later" && (group != groupPrograms || name != "FFmpeg"):
			return false, "LGPL-3.0-or-later is for FFmpeg only"
		}
	}

	return true, ""
}

func groupLabel(group string) string {
	switch group {
	case groupGo:
		return "Go module"
	case groupNPM:
		return "npm package"
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

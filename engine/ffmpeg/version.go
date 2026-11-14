package ffmpeg

import (
	"regexp"
	"strconv"
	"strings"
)

// License is the classification of SDD-011 8.2.
type License struct {
	Name     string // "lgpl", "gpl" or "nonfree"
	Version3 bool   // LGPL v3 (--enable-version3), else v2.1
}

// VersionInfo is what ffmpeg -hide_banner -version prints, parsed.
type VersionInfo struct {
	Version    string   // the token after "ffmpeg version "
	Major      int      // -1 when the version is not numeric (a master build)
	ConfigLine string   // the line after "configuration:"
	Flags      []string // the configure flags
	License    License
	Warnings   []string // untested_ffmpeg_version, old_ffmpeg_version
}

// TestedVersion is the release the product was tested on (SDD-011 8.2).
const TestedVersion = "9.0.2"

var majorRe = regexp.MustCompile(`^n?(\d+)\.`)

// ParseVersion reads the output of "-version". tool is "ffmpeg" or "ffprobe".
func ParseVersion(tool, out string) (VersionInfo, bool) {
	var v VersionInfo
	prefix := tool + " version "
	found := false
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, prefix) && !found:
			found = true
			v.Version, _, _ = strings.Cut(strings.TrimPrefix(line, prefix), " ")
		case strings.HasPrefix(line, "configuration:"):
			v.ConfigLine = strings.TrimSpace(strings.TrimPrefix(line, "configuration:"))
		}
	}
	if !found {
		return v, false
	}
	v.Flags = strings.Fields(v.ConfigLine)
	v.License = classify(v.Flags)
	v.Major = -1
	if m := majorRe.FindStringSubmatch(v.Version); m != nil {
		v.Major, _ = strconv.Atoi(m[1])
	}
	if !strings.Contains(strings.TrimPrefix(v.Version, "n"), TestedVersion) {
		v.Warnings = append(v.Warnings, "untested_ffmpeg_version")
	}
	if v.Major >= 0 && v.Major < 7 {
		v.Warnings = append(v.Warnings, "old_ffmpeg_version")
	}
	return v, true
}

// classify applies SDD-011 8.2: --enable-nonfree gives nonfree, else --enable-gpl gives
// gpl, else lgpl.
func classify(flags []string) License {
	var gpl, nonfree, v3 bool
	for _, f := range flags {
		switch f {
		case "--enable-nonfree":
			nonfree = true
		case "--enable-gpl":
			gpl = true
		case "--enable-version3":
			v3 = true
		}
	}
	switch {
	case nonfree:
		return License{Name: "nonfree", Version3: v3}
	case gpl:
		return License{Name: "gpl", Version3: v3}
	}
	return License{Name: "lgpl", Version3: v3}
}

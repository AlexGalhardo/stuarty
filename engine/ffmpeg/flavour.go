package ffmpeg

import (
	"regexp"
	"slices"
	"strings"
)

// FlavourName tells which build of FFmpeg is running (ADR-0011, D-017).
type FlavourName string

// The builds the product knows by their configure line.
const (
	// FlavourOwn is the build this project makes (P3-REL-001): LGPL, no OpenH264 compiled in, the Cisco
	// library loaded at run time. Its configure line carries --extra-version=stuarty<N>
	// (vukem<N> up to the build ffmpeg-9.0.2-2, VV-231).
	FlavourOwn FlavourName = "stuarty"
	// FlavourBtbN is the BtbN lgpl build pinned through the Prototype and ALPHA.
	FlavourBtbN FlavourName = "btbn"
	// FlavourOther is any other FFmpeg: the user's own.
	FlavourOther FlavourName = "other"
)

// ownExtraVersions are the prefixes of --extra-version that mark the own build. The build pipeline must
// set one (SDD-011 8.2); an FFmpeg without it is never taken for the own build. "vukem" is the marker of the
// builds installed before the rename (VV-231), accepted until release 1.0.0.
var ownExtraVersions = []string{"stuarty", "vukem"}

var extraVersionRe = regexp.MustCompile(`^--extra-version=(.*)$`)

// Flavour is what the configure line says about a build: which one it is and its licence flags.
// It is read from the output of -version, so a fake is enough to test it.
type Flavour struct {
	Name    FlavourName
	License License
	// GPL and Nonfree are true when --enable-gpl or --enable-nonfree is on the line.
	GPL, Nonfree bool
	// LinksOpenH264 is true when --enable-libopenh264 is on the line: the encoder is in the build
	// and needs a Cisco or a distribution library to work.
	LinksOpenH264 bool
	// ExtraVersion is the value of --extra-version.
	ExtraVersion string
}

// Flavour identifies the build from its configure line.
func (v VersionInfo) Flavour() Flavour {
	f := Flavour{Name: FlavourOther, License: v.License}
	btbn := false
	for _, flag := range v.Flags {
		switch flag {
		case "--enable-gpl":
			f.GPL = true
		case "--enable-nonfree":
			f.Nonfree = true
		case "--enable-libopenh264":
			f.LinksOpenH264 = true
		case "--prefix=/ffbuild/prefix":
			btbn = true
		}
		if m := extraVersionRe.FindStringSubmatch(flag); m != nil {
			f.ExtraVersion = m[1]
		}
	}
	switch {
	case slices.ContainsFunc(ownExtraVersions, func(p string) bool { return strings.HasPrefix(f.ExtraVersion, p) }):
		f.Name = FlavourOwn
	case btbn:
		f.Name = FlavourBtbN
	}
	return f
}

// LGPLOnly is true when neither --enable-gpl nor --enable-nonfree is on the line (NFR-043).
func (f Flavour) LGPLOnly() bool { return !f.GPL && !f.Nonfree }

// Problems lists what is wrong with a build the project distributes: an own or BtbN build with a
// GPL or nonfree flag must never reach a user. A user's FFmpeg is the user's choice and has none.
func (f Flavour) Problems() []string {
	if f.Name == FlavourOther {
		return nil
	}
	var p []string
	if f.GPL {
		p = append(p, "--enable-gpl")
	}
	if f.Nonfree {
		p = append(p, "--enable-nonfree")
	}
	// The own build loads the Cisco library at run time (D-017); OpenH264 compiled in is the thing it replaces.
	if f.Name == FlavourOwn && f.LinksOpenH264 {
		p = append(p, "--enable-libopenh264")
	}
	return p
}

// Flavour is the build of the ffmpeg of the installation.
func (in *Installation) Flavour() Flavour { return in.Info.Flavour() }

// Package skills holds the skills that ship with Stuarty (SDD-018 section 16): one folder per skill in the
// Agent Skills layout, embedded in the binary. The service installs them into the library of the workspace as
// built-in skills; this package knows nothing of the library.
package skills

import (
	"embed"
	"io/fs"
)

// The pattern takes the SKILL.md of each folder and the other files it holds (references/, assets/).
//
//go:embed */*
var embedded embed.FS

// FS returns the built-in skills, one folder per skill.
func FS() fs.FS { return embedded }

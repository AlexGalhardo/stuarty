// Package schema holds the generated contracts of ADR-0014: the JSON Schema of the
// timeline document and of every command, the registry snapshot, the error table and
// the limits. The files are written by internal/tools/contractgen; do not edit them.
package schema

import "embed"

// Files holds the generated JSON files.
//
//go:embed *.json commands/*.json
var Files embed.FS

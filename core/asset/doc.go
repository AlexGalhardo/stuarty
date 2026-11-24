// Package asset holds the pure rules of the asset library (SDD-011 section 14, PRD-004 RF-320 to RF-323): the
// origin of a file and what each kind of origin must say, the tags, the filters that pick a file out of the
// library, the full-text query, the projects that use a file, and how a size is written and a total is added
// (epic 94). It does no I/O: the service reads the rows
// and gives them in.
package asset

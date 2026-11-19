// Package assets is the asset library over the files of the workspace (SDD-011 section 14): the entry of a file
// with its origin, tags and uses, the filters and the full-text search, and the writes of the library side of an
// asset. The rules are in core/asset; this package joins them to the rows the service reads. It does not import
// the service: the service gives it the files the caller may see and a Store, as service/connections does.
package assets

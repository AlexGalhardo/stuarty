// Package migrate carries a raw timeline document from the schema version it was
// written at to the current one (SDD-001 §6). It works on decoded JSON, depends on
// nothing but the standard library and knows no document type, so that the timeline
// package can call it before it types a document.
package migrate

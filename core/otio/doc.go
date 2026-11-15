// Package otio converts a timeline document to and from OpenTimelineIO (RF-065,
// SDD-001 §9). It is pure: no file, network or clock access. Export writes the
// supported subset and reports what it cannot carry; import returns a batch of
// command operations, never a document, and never opens a media reference.
package otio

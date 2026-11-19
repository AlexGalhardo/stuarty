// Package image is the second kind of document of Stuarty, a canvas with a tree of layers (SDD-021, ADR-0038):
// its types, its catalogue of commands, the batch machinery with undo, its one-frame plan for the renderer,
// its templates and its lint. It is pure: no I/O, no clock, no randomness; everything a command needs from
// outside comes in an Env.
package image

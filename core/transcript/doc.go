// Package transcript holds the rules of editing by text (SDD-023): the mapping from the words of a
// transcript to the ranges of the timeline that show them, and the detection of filler words and
// repetitions. It is pure: no I/O, no clock, no model. The commands that cut live in core/command and
// the service reads the transcript; this package only answers questions about words.
package transcript

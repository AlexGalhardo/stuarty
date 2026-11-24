// Package aihost starts the local AI helper, stuarty-aihelper (SDD-024, ADR-0050), and turns what it
// writes into the face track of an asset. The helper is a separate process under the process policy of
// SDD-007 section 6 (ffmpeg.Runner.RunHelper); this package never links ONNX Runtime and never decides
// anything about a project: the rules live in core/facetrack.
package aihost

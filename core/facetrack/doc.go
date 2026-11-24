// Package facetrack holds the rules of face-aware framing (SDD-024): the face track of an asset, the choice
// of the face to follow, and the crop keyframes that keep it in a vertical frame. It is pure: no I/O, no
// clock, no model, no floating point on the path that decides a keyframe. The helper that finds the faces
// lives in engine/aihost; the command that writes the keyframes lives in core/command.
package facetrack

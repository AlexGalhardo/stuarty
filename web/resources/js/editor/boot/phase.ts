import type { EditorBootstrap } from "./bootstrap";

// The build phase of SDD-012 section 10: the phase the server says this
// editor is built for. Every control that exists only from a later phase asks
// isPhaseAtLeast; nothing compares the phase string itself.
export type BuildPhase = EditorBootstrap["phase"];

const ORDER: Readonly<Record<BuildPhase, number>> = {
	prototype: 0,
	alpha: 1,
	beta: 2,
};

let current: BuildPhase = "prototype";

export function setBuildPhase(phase: BuildPhase): void {
	current = phase;
}

export function buildPhase(): BuildPhase {
	return current;
}

// Whether the build is at `minimum` or later. `phase` is for tests and for a
// caller that holds the bootstrap; the default is the phase set at boot.
export function isPhaseAtLeast(
	minimum: BuildPhase,
	phase: BuildPhase = current,
): boolean {
	return ORDER[phase] >= ORDER[minimum];
}

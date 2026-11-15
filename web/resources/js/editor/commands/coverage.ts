// The coverage declaration of SDD-012 section 9.3 for the Prototype: for each
// command the Prototype UI can send, the entry points that exist in code. The
// test in coverage.test.ts checks it against the committed registry snapshot.

export interface EntryPoint {
	kind: "gesture" | "inspector" | "menu" | "toolbar" | "key" | "palette";
	// An id of ACTIONS.
	action: string;
	phase: "P" | "A" | "B";
}

// The actions of the registry and the operations each one sends. `asset.add`
// is internal (the service writes it when a file is imported), so it has no
// entry here.
export const ACTIONS: Readonly<Record<string, readonly string[]>> = {
	"library.add_to_timeline": ["clip.add", "track.add"],
	"timeline.split": ["item.split"],
	"timeline.trim_start": ["item.trim"],
	"timeline.trim_end": ["item.trim"],
	"caption.add": ["caption.add", "track.add"],
};

export const COVERAGE: Readonly<Record<string, readonly EntryPoint[]>> = {
	"clip.add": [
		{ kind: "gesture", action: "library.add_to_timeline", phase: "P" },
	],
	"track.add": [
		{ kind: "gesture", action: "library.add_to_timeline", phase: "P" },
		{ kind: "toolbar", action: "caption.add", phase: "P" },
	],
	"item.split": [
		{ kind: "toolbar", action: "timeline.split", phase: "P" },
		{ kind: "key", action: "timeline.split", phase: "P" },
	],
	"item.trim": [
		{ kind: "toolbar", action: "timeline.trim_start", phase: "P" },
		{ kind: "toolbar", action: "timeline.trim_end", phase: "P" },
		{ kind: "key", action: "timeline.trim_start", phase: "P" },
		{ kind: "key", action: "timeline.trim_end", phase: "P" },
	],
	"caption.add": [{ kind: "toolbar", action: "caption.add", phase: "P" }],
};

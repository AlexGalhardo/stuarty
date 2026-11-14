import type { ProjectDocument } from "@/generated/timeline";

import type { Change, JsonValue } from "../api/types";

export type DeepReadonly<T> = T extends (infer U)[]
	? readonly DeepReadonly<U>[]
	: T extends object
		? { readonly [K in keyof T]: DeepReadonly<T[K]> }
		: T;

type Dict = Record<string, unknown>;

const COLLECTIONS = {
	asset: "assets",
	track: "tracks",
	item: "items",
	font: "fonts",
} as const;

export function deepEqual(a: unknown, b: unknown): boolean {
	if (a === b) return true;
	if (
		typeof a !== "object" ||
		typeof b !== "object" ||
		a === null ||
		b === null
	) {
		return false;
	}
	if (Array.isArray(a) !== Array.isArray(b)) return false;
	const ka = Object.keys(a);
	const kb = Object.keys(b);
	if (ka.length !== kb.length) return false;
	return ka.every(
		(key) => key in b && deepEqual((a as Dict)[key], (b as Dict)[key]),
	);
}

// An absent field and a null before-value mean the same (SDD-002 section A.5).
function same(local: unknown, before: JsonValue): boolean {
	return deepEqual(local ?? null, before);
}

function applyFields(
	target: Dict,
	changes: { [field: string]: [JsonValue, JsonValue] },
): Dict | null {
	const next: Dict = { ...target };
	for (const [field, [before, after]] of Object.entries(changes)) {
		if (!same(target[field], before)) return null;
		if (after === null) delete next[field];
		else next[field] = after;
	}
	return next;
}

// applyDiff is the mechanical patch of SDD-012 section 5.4: every entry states
// the after-values, so the patch only copies them in. It creates new objects
// along the changed path only, validates nothing and derives nothing. It
// returns "unpatchable" for an unknown kind, an `updated` or `removed` change
// for a missing id, an `added` change for an existing id, or a before-value
// that does not match the local copy; the caller then resyncs.
export function applyDiff(
	doc: DeepReadonly<ProjectDocument>,
	diff: readonly Change[],
): ProjectDocument | "unpatchable" {
	let next: Dict = { ...(doc as unknown as Dict) };

	for (const change of diff) {
		const [scope, verb] = change.kind.split(".") as [string, string];

		if (scope === "project" && verb === "updated" && "changes" in change) {
			const patched = applyFields(next, change.changes);
			if (patched === null) return "unpatchable";
			next = patched;
			continue;
		}

		if (scope === "settings" && verb === "updated" && "changes" in change) {
			const patched = applyFields(
				(next.settings ?? {}) as Dict,
				change.changes,
			);
			if (patched === null) return "unpatchable";
			next = { ...next, settings: patched };
			continue;
		}

		if (!(scope in COLLECTIONS)) return "unpatchable";
		const key = COLLECTIONS[scope as keyof typeof COLLECTIONS];
		const collection = (next[key] ?? {}) as Dict;
		const exists = Object.hasOwn(collection, change.id);

		if (verb === "added" && "object" in change) {
			if (exists) return "unpatchable";
			next = { ...next, [key]: { ...collection, [change.id]: change.object } };
		} else if (verb === "removed" && "object" in change) {
			if (!exists) return "unpatchable";
			const { [change.id]: _gone, ...rest } = collection;
			next = { ...next, [key]: rest };
		} else if (
			verb === "updated" &&
			"changes" in change &&
			(scope === "item" || scope === "track")
		) {
			if (!exists) return "unpatchable";
			const patched = applyFields(
				collection[change.id] as Dict,
				change.changes,
			);
			if (patched === null) return "unpatchable";
			next = { ...next, [key]: { ...collection, [change.id]: patched } };
		} else {
			return "unpatchable";
		}
	}

	return next as unknown as ProjectDocument;
}

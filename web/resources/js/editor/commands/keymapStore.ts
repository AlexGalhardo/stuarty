// The current bindings of the rebindable shortcuts (SDD-012 section 18.2): the
// defaults of keymap.ts plus the overrides of the person, kept by prefs.ts.
// Handlers call bindingFor() when a key is pressed, so a rebinding is live at
// once and no listener has to be set up again.

import { createStore, readOnly, useStore } from "../state/createStore";
import { readKeymap, writeKeymap } from "../state/prefs";
import {
	BINDING_IDS,
	type BindingId,
	type Bindings,
	comboFromEvent,
	DEFAULT_BINDINGS,
	isBindingId,
	RESERVED_KEYS,
	SHORTCUTS,
} from "./keymap";

function load(): Bindings {
	const next: Record<BindingId, string> = { ...DEFAULT_BINDINGS };
	for (const [id, key] of Object.entries(readKeymap())) {
		if (isBindingId(id) && key !== "") next[id] = key;
	}
	return next;
}

const store = createStore<Bindings>(load());
export const bindingsStore = readOnly(store);

export function getBindings(): Bindings {
	return store.getState();
}

export function useBindings(): Bindings {
	return useStore(bindingsStore, (s) => s);
}

function overrides(bindings: Bindings): Record<string, string> {
	const out: Record<string, string> = {};
	for (const id of BINDING_IDS) {
		if (bindings[id] !== DEFAULT_BINDINGS[id]) out[id] = bindings[id];
	}
	return out;
}

export type RebindResult =
	| { ok: true }
	// The id of the row that already uses the key, or "reserved".
	| { ok: false; conflict: string };

// The keys another row holds. A rebindable row's own keys are looked up in the
// bindings, but its aliases (`+` and `-` of the zoom rows) are fixed.
function fixedCombos(): Map<string, string> {
	const map = new Map<string, string>();
	for (const row of SHORTCUTS) {
		for (const key of aliasesOf(row.id)) map.set(key, row.id);
		if (isBindingId(row.id)) continue;
		for (const key of row.keys.split(", ")) map.set(key, row.id);
	}
	return map;
}

function aliasesOf(id: string): readonly string[] {
	const row = SHORTCUTS.find((candidate) => candidate.id === id);
	return row?.aliases === undefined ? [] : row.aliases.split(", ");
}

// Looks for another row that holds the key: a fixed one, a rebindable one or a
// reserved key. A row never conflicts with itself.
export function findConflict(
	id: BindingId,
	combo: string,
	bindings: Bindings = store.getState(),
): string | null {
	// Alt combos are the panels' and the browser's.
	if (RESERVED_KEYS.includes(combo) || combo.includes("Alt+"))
		return "reserved";
	const fixed = fixedCombos().get(combo);
	if (fixed !== undefined && fixed !== id) return fixed;
	for (const other of BINDING_IDS) {
		if (other !== id && bindings[other] === combo) return other;
	}
	return null;
}

export function rebind(id: BindingId, combo: string): RebindResult {
	const conflict = findConflict(id, combo);
	if (conflict !== null) return { ok: false, conflict };
	const next = { ...store.getState(), [id]: combo };
	store.setState(() => next);
	writeKeymap(overrides(next));
	return { ok: true };
}

export function resetBindings(): void {
	store.setState(() => DEFAULT_BINDINGS);
	writeKeymap({});
}

// The rebindable row the key press is the key of, among `ids`.
export function bindingFor<T extends BindingId>(
	event: Parameters<typeof comboFromEvent>[0],
	ids: readonly T[],
): T | null {
	const combo = comboFromEvent(event);
	if (combo === null) return null;
	const bindings = store.getState();
	for (const id of ids) {
		if (bindings[id] === combo) return id;
		// The aliases of the default key, and `=`, the unshifted key of `+` on
		// most layouts. A key the person chose has none.
		if (bindings[id] !== DEFAULT_BINDINGS[id]) continue;
		if (aliasesOf(id).includes(combo)) return id;
		if (id === "zoom_in" && combo === "=") return id;
	}
	return null;
}

// The current keys of the registry (SDD-012 sections 18.2 and 18.9): the preset
// and the overrides of the person on top of the table of keymap.ts, kept by
// prefs.ts. Handlers ask rowFor() when a key is pressed, so a rebinding is
// live at once and no listener has to be set up again.

import { createStore, readOnly, useStore } from "../state/createStore";
import { readKeymap, writeKeymap } from "../state/prefs";
import {
	aliasKeys,
	type ComboEvent,
	comboFromEvent,
	DEFAULT_PRESET,
	displayCombo,
	isBindingId,
	PRESET_IDS,
	type PresetId,
	presetKeys,
	RESERVED_KEYS,
	rowById,
	type Scope,
	SHORTCUTS,
	type Shortcut,
	scopesOverlap,
} from "./keymap";

export interface Keymap {
	preset: PresetId;
	// Row id to the one key the person chose; it replaces the row's keys.
	overrides: Readonly<Record<string, string>>;
}

export const DEFAULT_KEYMAP: Keymap = { preset: DEFAULT_PRESET, overrides: {} };

function isPreset(value: string | null): value is PresetId {
	return (PRESET_IDS as readonly (string | null)[]).includes(value);
}

function load(): Keymap {
	const stored = readKeymap();
	const overrides: Record<string, string> = {};
	for (const [id, key] of Object.entries(stored.overrides)) {
		if (isBindingId(id) && key !== "") overrides[id] = key;
	}
	return {
		preset: isPreset(stored.preset) ? stored.preset : DEFAULT_PRESET,
		overrides,
	};
}

const store = createStore<Keymap>(load());
export const keymapStore = readOnly(store);

export function getKeymap(): Keymap {
	return store.getState();
}

export function useKeymap(): Keymap {
	return useStore(keymapStore, (s) => s);
}

function save(next: Keymap): void {
	store.setState(() => next);
	writeKeymap({
		preset: next.preset === DEFAULT_PRESET ? null : next.preset,
		overrides: { ...next.overrides },
	});
}

// The keys a row answers to now: the key the person chose, or the keys of the
// preset and the aliases of the row.
export function keysOf(
	row: Shortcut,
	keymap: Keymap = store.getState(),
): string[] {
	const chosen = keymap.overrides[row.id];
	if (chosen !== undefined) return [chosen];
	return [...presetKeys(row, keymap.preset), ...aliasKeys(row)];
}

// The combos and the rows that hold them, for one keymap. Fixed rows hold no
// key.
const tables = new WeakMap<Keymap, Map<string, Shortcut[]>>();

function tableOf(keymap: Keymap): Map<string, Shortcut[]> {
	const cached = tables.get(keymap);
	if (cached !== undefined) return cached;
	const table = new Map<string, Shortcut[]>();
	for (const row of SHORTCUTS) {
		if (row.fixed === true) continue;
		for (const key of keysOf(row, keymap)) {
			const held = table.get(key);
			if (held === undefined) table.set(key, [row]);
			else held.push(row);
		}
	}
	tables.set(keymap, table);
	return table;
}

// The row a key press is the key of, among the rows of `scopes`, or null.
export function rowFor(
	event: ComboEvent,
	scopes: readonly Scope[],
	keymap: Keymap = store.getState(),
): string | null {
	const combo = comboFromEvent(event);
	if (combo === null) return null;
	const held = tableOf(keymap).get(combo);
	return held?.find((row) => scopes.includes(row.scope))?.id ?? null;
}

// Whether the key press is a key of the row `id` now.
export function isRowKey(event: ComboEvent, id: string): boolean {
	const combo = comboFromEvent(event);
	if (combo === null) return false;
	return (
		tableOf(store.getState())
			.get(combo)
			?.some((row) => row.id === id) ?? false
	);
}

// The id of another row that holds the key in a scope that overlaps, "reserved"
// for a key no row may take, or null. A row never conflicts with itself.
export function findConflict(
	id: string,
	combo: string,
	keymap: Keymap = store.getState(),
): string | null {
	if (RESERVED_KEYS.includes(combo)) return "reserved";
	const row = rowById(id);
	if (row === undefined) return null;
	for (const other of tableOf(keymap).get(combo) ?? []) {
		if (other.id !== id && scopesOverlap(row.scope, other.scope))
			return other.id;
	}
	return null;
}

export type RebindResult =
	| { ok: true }
	// The id of the row that already uses the key, or "reserved".
	| { ok: false; conflict: string };

function withOverride(keymap: Keymap, row: Shortcut, combo: string): Keymap {
	const overrides = { ...keymap.overrides };
	const base = presetKeys(row, keymap.preset);
	// A key that is already the row's own in the preset is no override.
	if (base.length === 1 && base[0] === combo) delete overrides[row.id];
	else overrides[row.id] = combo;
	return { preset: keymap.preset, overrides };
}

export function rebind(id: string, combo: string): RebindResult {
	const row = rowById(id);
	if (row === undefined || row.fixed === true)
		return { ok: false, conflict: "reserved" };
	const conflict = findConflict(id, combo);
	if (conflict !== null) return { ok: false, conflict };
	save(withOverride(store.getState(), row, combo));
	return { ok: true };
}

// Back to the preset's own keys; the preset stays.
export function resetBindings(): void {
	save({ preset: store.getState().preset, overrides: {} });
}

// Changes the preset. An override that would collide with a key of the new
// preset is dropped; the rows it names are returned so the dialog can say so.
export function setPreset(preset: PresetId): { dropped: string[] } {
	let overrides: Record<string, string> = { ...store.getState().overrides };
	const dropped: string[] = [];
	for (let again = true; again; ) {
		again = false;
		const candidate: Keymap = { preset, overrides };
		for (const [id, combo] of Object.entries(overrides)) {
			if (findConflict(id, combo, candidate) === null) continue;
			const { [id]: _removed, ...rest } = overrides;
			overrides = rest;
			dropped.push(id);
			again = true;
			break;
		}
	}
	save({ preset, overrides });
	return { dropped };
}

// The first key of the row that is current now, as shown ("Ctrl+B"), or null
// when the row does not exist or has no key.
export function keyLabel(
	id: string,
	keymap: Keymap,
	mac: boolean,
): string | null {
	const row = rowById(id);
	if (row === undefined) return null;
	const first = keysOf(row, keymap)[0];
	return first === undefined || first === "" ? null : displayCombo(first, mac);
}

// "Split (Ctrl+B)": the label with the first key of the row that is current
// now, so a tooltip never states a key that was rebound. A row that does not
// exist, or has no key, leaves the label alone.
export function labelWithKey(
	label: string,
	id: string,
	keymap: Keymap,
	mac: boolean,
): string {
	const key = keyLabel(id, keymap, mac);
	return key === null ? label : `${label} (${key})`;
}

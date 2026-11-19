// The only file that touches the storage of the browser (rule R7). A blocked
// storage (private mode) leaves the defaults and the choice lasts for the page.

export type TimeDisplay = "timecode" | "frames" | "seconds";
export type TimeUnit = "s" | "f";

// How every time of the editor is shown (SDD-012 section 12.2): one preference
// for the library, the preview, the ruler and the inspector.
export function readTimeDisplay(): TimeDisplay {
	try {
		const value = localStorage.getItem("vv.timeDisplay");
		return value === "frames" || value === "seconds" ? value : "timecode";
	} catch {
		return "timecode";
	}
}

// The unit appended to a bare number typed in a time field (SDD-012 section 12.2).
export function readTimeUnit(): TimeUnit {
	try {
		return localStorage.getItem("vv.timeUnit") === "f" ? "f" : "s";
	} catch {
		return "s";
	}
}

export function writeTimeUnit(unit: TimeUnit): void {
	try {
		localStorage.setItem("vv.timeUnit", unit);
	} catch {
		// Storage can be blocked: the choice lasts for this page only.
	}
}

// The shortcut overrides (SDD-012 section 18.2): a JSON object from binding id
// to key. Anything that is not an object of strings reads as no overrides.
export function readKeymap(): Record<string, string> {
	try {
		const parsed: unknown = JSON.parse(
			localStorage.getItem("vv.keymap") ?? "null",
		);
		if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed))
			return {};
		const out: Record<string, string> = {};
		for (const [id, key] of Object.entries(parsed)) {
			if (typeof key === "string") out[id] = key;
		}
		return out;
	} catch {
		return {};
	}
}

export function writeKeymap(overrides: Readonly<Record<string, string>>): void {
	try {
		if (Object.keys(overrides).length === 0) {
			localStorage.removeItem("vv.keymap");
			return;
		}
		localStorage.setItem("vv.keymap", JSON.stringify(overrides));
	} catch {
		// Storage can be blocked: the bindings last for this page only.
	}
}

// Whether the first-run tips were shown on this browser (SDD-012 section 18.5).
export function readShortcutsSeen(): boolean {
	try {
		return localStorage.getItem("vv.shortcutsSeen") === "1";
	} catch {
		return false;
	}
}

export function writeShortcutsSeen(): void {
	try {
		localStorage.setItem("vv.shortcutsSeen", "1");
	} catch {
		// Storage can be blocked: the tips show again on the next load.
	}
}

// The track row heights in pixels, one per kind of track (SDD-012 section 7.4),
// kept under `vv.layout`. Anything that is not a pair of numbers inside the
// bounds reads as the default of that kind.
export interface LayoutPrefs {
	rows: { visual: number; audio: number };
}

export const ROW_BOUNDS = { min: 32, max: 160 } as const;
export const DEFAULT_ROWS = { visual: 64, audio: 48 } as const;

function validRow(value: unknown, fallback: number): number {
	return typeof value === "number" &&
		Number.isFinite(value) &&
		value >= ROW_BOUNDS.min &&
		value <= ROW_BOUNDS.max
		? Math.round(value)
		: fallback;
}

export function readLayout(): LayoutPrefs {
	const defaults = { rows: { ...DEFAULT_ROWS } };
	try {
		const parsed: unknown = JSON.parse(
			localStorage.getItem("vv.layout") ?? "null",
		);
		if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed))
			return defaults;
		const rows = (parsed as { rows?: unknown }).rows;
		if (typeof rows !== "object" || rows === null) return defaults;
		const { visual, audio } = rows as { visual?: unknown; audio?: unknown };
		return {
			rows: {
				visual: validRow(visual, DEFAULT_ROWS.visual),
				audio: validRow(audio, DEFAULT_ROWS.audio),
			},
		};
	} catch {
		return defaults;
	}
}

export function writeLayout(layout: LayoutPrefs): void {
	try {
		localStorage.setItem("vv.layout", JSON.stringify(layout));
	} catch {
		// Storage can be blocked: the heights last for this page only.
	}
}

// The language of the interface is a preference of this browser (SDD-012
// section 16.3), not of the workspace: two browsers on one server keep their own.
// It is sent on every request in the Stuarty-Lang header.
const LOCALE_TAG = /^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$/;

export function readLocale(): string | null {
	try {
		const value = localStorage.getItem("vv.lang");
		return value !== null && LOCALE_TAG.test(value) ? value : null;
	} catch {
		return null;
	}
}

// The preview executor this browser forces (SDD-012 section 11.1), for
// comparison and bug reports; `preview/executorChoice.ts` reads the value.
export function readPreviewExecutor(): string | null {
	try {
		return localStorage.getItem("stuarty.preview.executor");
	} catch {
		return null;
	}
}

export function writeLocale(locale: string): void {
	try {
		localStorage.setItem("vv.lang", locale);
	} catch {
		// Storage can be blocked: the choice lasts for this page only.
	}
}

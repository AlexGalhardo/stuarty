// The preferences of the teleprompter (SDD-025 section 20.3): the speed of the scroll, the size of the text and the
// mirror. They belong to the person's UI, so they live in the local storage of the browser or the window and are
// never sent to the server; they decide nothing about an edit.

export const PREFS_KEY = "stuarty.teleprompter";

export const SPEED_MIN = 10;
export const SPEED_MAX = 400;
export const SIZE_MIN = 24;
export const SIZE_MAX = 120;

export interface TeleprompterPrefs {
	// Pixels a second.
	speed: number;
	// Pixels of the text.
	size: number;
	// The text flipped left to right, for a glass in front of the lens.
	mirror: boolean;
}

export const DEFAULT_PREFS: TeleprompterPrefs = {
	speed: 60,
	size: 48,
	mirror: false,
};

const clamp = (value: number, low: number, high: number): number =>
	Math.min(high, Math.max(low, Math.round(value)));

// A stored or typed value held to its range; anything that is not a number is the default.
export function cleanPrefs(raw: unknown): TeleprompterPrefs {
	const given = typeof raw === "object" && raw !== null ? raw : {};
	const field = (name: string): unknown =>
		(given as Record<string, unknown>)[name];
	const number = (name: string, fallback: number): number => {
		const value = field(name);
		return typeof value === "number" && Number.isFinite(value)
			? value
			: fallback;
	};
	return {
		speed: clamp(number("speed", DEFAULT_PREFS.speed), SPEED_MIN, SPEED_MAX),
		size: clamp(number("size", DEFAULT_PREFS.size), SIZE_MIN, SIZE_MAX),
		mirror: field("mirror") === true,
	};
}

export function loadPrefs(
	storage: Pick<Storage, "getItem"> | null,
): TeleprompterPrefs {
	try {
		const text = storage?.getItem(PREFS_KEY);
		return text ? cleanPrefs(JSON.parse(text)) : DEFAULT_PREFS;
	} catch {
		// A storage that refuses, or text that is not JSON: the defaults.
		return DEFAULT_PREFS;
	}
}

export function savePrefs(
	storage: Pick<Storage, "setItem"> | null,
	prefs: TeleprompterPrefs,
): void {
	try {
		storage?.setItem(PREFS_KEY, JSON.stringify(prefs));
	} catch {
		// A full or refused storage only loses the preference.
	}
}

export function browserStorage(): Storage | null {
	try {
		return window.localStorage;
	} catch {
		return null;
	}
}

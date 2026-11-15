// Time strings and their display (SDD-012 section 12.2). This file and
// ui/TimeField.tsx are the only places that handle time values (rule R5): the
// core parses and rounds, the browser only checks the shape of what was typed
// and appends the unit (rule T8: a bare number never leaves the browser).

export type TimeUnit = "s" | "f";
export type TimeDisplay = "timecode" | "frames" | "seconds";

export const DEFAULT_TIME_UNIT: TimeUnit = "s";

export type NormalizedTime = { ok: true; value: string } | { ok: false };

const SECONDS = /^\d+(?:\.\d+)?s$/;
const FRAMES = /^\d+f$/;
const BARE_DECIMAL = /^\d+(?:\.\d+)?$/;
const BARE_INTEGER = /^\d+$/;
// HH:MM:SS.mmm or MM:SS.mmm, minutes and seconds below 60 (SDD-002 A.4).
const TIMECODE = /^(?:(\d{1,2}):)?(\d{1,2}):(\d{1,2})(?:\.\d+)?$/;

// The decimal comma of pt-BR (D-030): replaced by a point before the text is
// checked. Any other locale passes the text through, so `12,5` stays invalid.
export function localizeDecimalInput(raw: string, locale: string): string {
	return locale.toLowerCase().startsWith("pt") ? raw.replace(",", ".") : raw;
}

function validTimecode(text: string): boolean {
	const match = TIMECODE.exec(text);
	if (match === null) return false;
	const [, hours, minutes, seconds] = match;
	if (Number(seconds) >= 60) return false;
	return hours === undefined || Number(minutes) < 60;
}

// A pure string function: trim, check the shape against the three forms of
// SDD-002 A.4, append the default unit to a bare number. It does not convert
// to frames; the core parses and rounds. A decimal with the unit `f`, an empty
// string, a sign, a space inside and a fourth timecode part are rejected.
export function normalizeTimeInput(
	raw: string,
	defaultUnit: TimeUnit = DEFAULT_TIME_UNIT,
): NormalizedTime {
	const text = raw.trim();
	if (text === "") return { ok: false };
	if (SECONDS.test(text) || FRAMES.test(text)) return { ok: true, value: text };
	if (text.includes(":")) {
		return validTimecode(text) ? { ok: true, value: text } : { ok: false };
	}
	if (defaultUnit === "f") {
		return BARE_INTEGER.test(text)
			? { ok: true, value: `${text}f` }
			: { ok: false };
	}
	return BARE_DECIMAL.test(text)
		? { ok: true, value: `${text}s` }
		: { ok: false };
}

const pad = (n: number, width: number): string =>
	String(n).padStart(width, "0");

function timecodeOf(totalMs: number): string {
	const ms = totalMs % 1000;
	const whole = Math.floor(totalMs / 1000);
	const hours = Math.floor(whole / 3600);
	const minutes = Math.floor((whole % 3600) / 60);
	const clock = `${pad(minutes, 2)}:${pad(whole % 60, 2)}.${pad(ms, 3)}`;
	return hours > 0 ? `${pad(hours, 2)}:${clock}` : clock;
}

// Display only, always with a point (timecodes are not localized, section
// 16.4). `fps` is the project rational; the result is never parsed back.
export function formatTime(
	frame: number,
	fps: { num: number; den: number },
	mode: TimeDisplay,
): string {
	if (mode === "frames") return `${frame}f`;
	const ms = Math.round((frame * 1000 * fps.den) / fps.num);
	return mode === "seconds" ? `${(ms / 1000).toFixed(3)}s` : timecodeOf(ms);
}

// A duration the core gives in seconds (an asset's `duration_seconds`),
// formatted without a trip through frames.
export function formatSeconds(seconds: number, mode: TimeDisplay): string {
	const ms = Math.round(seconds * 1000);
	return mode === "timecode" ? timecodeOf(ms) : `${(ms / 1000).toFixed(3)}s`;
}

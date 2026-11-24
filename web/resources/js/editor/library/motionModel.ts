import type { MotionCheck, MotionMismatch } from "../api/types";

// What the motion panel says about the answers of the service. It only words them: the brief and the check
// are the service's (SDD-011 section 15), and nothing here decides whether a file fits.

export function sizeLabel(width: number, height: number): string {
	return `${width} x ${height}`;
}

// 30/1 is "30", 30000/1001 is "29.97": the rate as a person reads it.
export function rateLabel(rate: { num: number; den: number }): string {
	if (rate.den <= 0) return `${rate.num}/${rate.den}`;
	const value = rate.num / rate.den;
	return Number.isInteger(value) ? String(value) : value.toFixed(2);
}

const KNOWN_FIELDS = new Set([
	"size",
	"frame_rate",
	"duration",
	"alpha",
	"kind",
]);

// The message key of the name of a mismatch; a field the panel does not know is drawn as the service named it.
export function fieldKey(mismatch: MotionMismatch): string | null {
	return KNOWN_FIELDS.has(mismatch.field)
		? `ui.editor.handoff.field.${mismatch.field}`
		: null;
}

export type Verdict = "fits" | "mismatch";

export function verdictOf(check: MotionCheck): Verdict {
	return check.ok ? "fits" : "mismatch";
}

// The engine and the render key are optional members of the origin; a blank one is left out.
export function originOf(fields: {
	source: string;
	engine: string;
	engineVersion: string;
	renderKey: string;
}): {
	kind: "rendered_externally";
	source: string;
	engine?: string;
	engine_version?: string;
	render_key?: string;
} {
	const trim = (value: string): string | undefined =>
		value.trim() === "" ? undefined : value.trim();
	const engine = trim(fields.engine);
	const version = trim(fields.engineVersion);
	const key = trim(fields.renderKey);
	return {
		kind: "rendered_externally",
		source: fields.source.trim(),
		...(engine === undefined ? {} : { engine }),
		...(version === undefined ? {} : { engine_version: version }),
		...(key === undefined ? {} : { render_key: key }),
	};
}

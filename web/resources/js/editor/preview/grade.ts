import type { PlanCurvePoint, PlanGrade, PlanWheel } from "../api/types";

// What the preview needs to draw the advanced colour of SDD-004 section 3.14.
// The plan carries the bands, the wheels and the curve points as stored; this
// turns them into what the shader reads. It decides nothing about the edit: the
// parameters are quantised and the curve is evaluated exactly as the renderers
// do (the integer form of the SDD), so the 256 entry tables are the export's.
// The shader itself works in floats, so a pixel can differ from the export by a
// rounding step.

export const HSL_RANGES = [
	"red",
	"orange",
	"yellow",
	"green",
	"cyan",
	"blue",
	"purple",
	"magenta",
] as const;

const MAX_SLOPE = 2 ** 26;

// Half away from zero, as Go math.Round.
function roundHalfAway(v: number): number {
	return Math.sign(v) * Math.round(Math.abs(v));
}

// A parameter in -1..1 in 1/1024; NaN is 0.
function q1024(v: number | undefined): number {
	if (v === undefined || Number.isNaN(v)) return 0;
	return roundHalfAway(Math.min(Math.max(v * 1024, -1024), 1024));
}

// An arithmetic shift right of a value that can pass 32 bits.
function shr(v: number, bits: number): number {
	return Math.floor(v / 2 ** bits);
}

interface Point {
	x: number;
	y: number;
}

function curvePoints(points: readonly PlanCurvePoint[]): Point[] {
	const out: Point[] = [];
	for (const p of points) {
		const q16 = (v: number): number =>
			Number.isNaN(v)
				? 0
				: roundHalfAway(Math.min(Math.max(v * 65536, 0), 65536));
		const x = q16(p.x);
		const y = q16(p.y);
		const last = out[out.length - 1];
		if (last !== undefined && x <= last.x) continue;
		out.push({ x, y });
	}
	return out;
}

// The 256 entries of the monotone cubic through the points (Fritsch-Carlson,
// the tangents cut to 3 times the secant), flat before the first point and
// after the last; null for no points.
export function curveTable(
	points: readonly PlanCurvePoint[],
): Uint8Array | null {
	const pts = curvePoints(points);
	const n = pts.length;
	if (n === 0) return null;
	const delta: number[] = [];
	for (let k = 0; k + 1 < n; k++) {
		delta.push(
			Math.trunc(
				((pts[k + 1].y - pts[k].y) * 65536) / (pts[k + 1].x - pts[k].x),
			),
		);
	}
	const m = new Array<number>(n).fill(0);
	if (n > 1) {
		m[0] = delta[0];
		m[n - 1] = delta[n - 2];
		for (let k = 1; k < n - 1; k++) {
			if (Math.sign(delta[k - 1]) === Math.sign(delta[k]) && delta[k] !== 0) {
				m[k] = Math.trunc((delta[k - 1] + delta[k]) / 2);
			}
		}
		for (const [k, d] of delta.entries()) {
			for (const e of [k, k + 1]) {
				if (d === 0 || Math.sign(m[e]) !== Math.sign(d)) {
					m[e] = 0;
				} else if (Math.abs(m[e]) > 3 * Math.abs(d)) {
					m[e] = Math.sign(d) * 3 * Math.abs(d);
				}
				m[e] = Math.min(Math.max(m[e], -MAX_SLOPE), MAX_SLOPE);
			}
		}
	}
	const table = new Uint8Array(256);
	for (let v = 0; v < 256; v++) {
		const x = Math.trunc((v * 65536 + 127) / 255);
		let y: number;
		if (x <= pts[0].x) {
			y = pts[0].y;
		} else if (x >= pts[n - 1].x) {
			y = pts[n - 1].y;
		} else {
			let k = 0;
			while (x >= pts[k + 1].x) k++;
			const h = pts[k + 1].x - pts[k].x;
			const u = Math.trunc(((x - pts[k].x) * 65536) / h);
			const u2 = shr(u * u, 16);
			const u3 = shr(u2 * u, 16);
			const h00 = 2 * u3 - 3 * u2 + 65536;
			const h10 = u3 - 2 * u2 + u;
			const h01 = -2 * u3 + 3 * u2;
			const h11 = u3 - u2;
			y = shr(
				h00 * pts[k].y +
					h01 * pts[k + 1].y +
					h10 * shr(h * m[k], 16) +
					h11 * shr(h * m[k + 1], 16) +
					32768,
				16,
			);
		}
		y = Math.min(Math.max(y, 0), 65536);
		table[v] = shr(y * 255 + 32768, 16);
	}
	return table;
}

// The RGBA texels (256 by 1) of the curves of a grade: the red, green and blue
// tables each after the master table. Null when the grade has no curve.
export function curveTexels(
	curves: NonNullable<PlanGrade["curves"]>,
): Uint8Array | null {
	const master = curveTable(curves.master ?? []);
	const chans = [
		curveTable(curves.red ?? []),
		curveTable(curves.green ?? []),
		curveTable(curves.blue ?? []),
	];
	if (master === null && chans.every((c) => c === null)) return null;
	const texels = new Uint8Array(256 * 4);
	for (let ch = 0; ch < 3; ch++) {
		for (let v = 0; v < 256; v++) {
			let o = master === null ? v : master[v];
			const own = chans[ch];
			if (own !== null) o = own[o];
			texels[v * 4 + ch] = o;
		}
	}
	for (let v = 0; v < 256; v++) texels[v * 4 + 3] = 255;
	return texels;
}

// The tint of a wheel: the colour of its hue at full saturation and value minus
// the mean of its three channels, so that it has no luma.
function tint(hue: number): [number, number, number] {
	const hq = roundHalfAway(Math.min(Math.max(hue * 1000, 0), 360000)) % 360000;
	const h6 = hq * 6;
	const sector = Math.floor(h6 / 360000);
	const f = (h6 % 360000) / 360000;
	const rgb: [number, number, number][] = [
		[1, f, 0],
		[1 - f, 1, 0],
		[0, 1, f],
		[0, 1 - f, 1],
		[f, 0, 1],
		[1, 0, 1 - f],
	];
	const c = rgb[Math.min(sector, 5)];
	const mean = (c[0] + c[1] + c[2]) / 3;
	return [c[0] - mean, c[1] - mean, c[2] - mean];
}

// `xyz` is what the wheel adds to a pixel at full weight (amount times tint) and
// `w` is its luma lift (a quarter of the luma, as the SDD says).
function wheelUniform(wheel: PlanWheel | undefined): Float32Array {
	const out = new Float32Array(4);
	if (wheel === undefined) return out;
	const amount = q1024(wheel.amount) / 1024;
	const luma = q1024(wheel.luma) / 1024;
	const t = tint(wheel.hue);
	out[0] = amount * t[0];
	out[1] = amount * t[1];
	out[2] = amount * t[2];
	out[3] = luma * 0.25;
	return out;
}

export interface GradeUniforms {
	// 1 when the step runs: bands, wheels, curves.
	flags: Float32Array;
	// One vec4 per range (hue, saturation, lightness, 0), in HSL_RANGES order.
	bands: Float32Array[];
	// Shadows, midtones, highlights.
	wheels: Float32Array[];
	// The texels of the curves, or null.
	curves: Uint8Array | null;
}

export function gradeUniforms(grade: PlanGrade): GradeUniforms {
	const bands = HSL_RANGES.map(() => new Float32Array(4));
	let hasHsl = false;
	for (const band of grade.hsl ?? []) {
		const i = HSL_RANGES.indexOf(band.range);
		if (i < 0) continue;
		const q = [q1024(band.hue), q1024(band.saturation), q1024(band.lightness)];
		bands[i] = new Float32Array([q[0] / 1024, q[1] / 1024, q[2] / 1024, 0]);
		hasHsl ||= q.some((v) => v !== 0);
	}
	const w = grade.wheels;
	const wheels = [
		wheelUniform(w?.shadows),
		wheelUniform(w?.midtones),
		wheelUniform(w?.highlights),
	];
	const hasWheel = wheels.some((v) => v.some((c) => c !== 0));
	const curves = grade.curves === undefined ? null : curveTexels(grade.curves);
	return {
		flags: new Float32Array([
			hasHsl ? 1 : 0,
			hasWheel ? 1 : 0,
			curves ? 1 : 0,
			0,
		]),
		bands,
		wheels,
		curves,
	};
}

// A grade that changes no pixel is not drawn.
export function gradeIsIdentity(u: GradeUniforms): boolean {
	return u.flags[0] === 0 && u.flags[1] === 0 && u.flags[2] === 0;
}

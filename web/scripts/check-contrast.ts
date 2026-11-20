// Contrast check of docs/design-system.md section 2.6 and SDD-012 section 18.
//
// Reads the tokens of resources/css/app.css for :root (light) and .dark, converts
// OKLCH to sRGB (Bjorn Ottosson's matrices, gamut clamp, sRGB encoding, no
// rounding to 8 bits), composites a colour with alpha over the surface below it in sRGB as
// the browser paints it, and computes the WCAG 2.x contrast ratio of every pair.
//
// A pair fails when the ratio is below its threshold, and also when it differs
// by more than 0.02 from the number printed in design-system.md section 2.6:
// the document and the code must not drift. No dependency: Bun runs this file.
//
//   bun run check:contrast

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

type Theme = "light" | "dark";
type Rgb = [number, number, number];
type Tokens = Map<string, string>;

const TOLERANCE = 0.02;

const cssPath = join(
	dirname(fileURLToPath(import.meta.url)),
	"../resources/css/app.css",
);

// block returns the declarations of the first rule whose selector is exactly
// `selector`, outside any at-rule.
function block(css: string, selector: string): Tokens {
	const start = css.indexOf(`\n${selector} {`);
	if (start < 0) throw new Error(`app.css has no ${selector} block`);
	const open = css.indexOf("{", start);
	const close = css.indexOf("\n}", open);
	const tokens: Tokens = new Map();
	for (const m of css.slice(open + 1, close).matchAll(/(--[\w-]+):\s*([^;]+);/g)) {
		tokens.set(m[1], m[2].trim());
	}
	return tokens;
}

// The tokens of the stylesheet being checked; evaluate() sets them.
let themes: Record<Theme, Tokens>;

interface Colour {
	rgb: Rgb; // 0 to 255, not rounded
	alpha: number;
}

function srgbEncode(linear: number): number {
	const c = Math.min(1, Math.max(0, linear));
	return c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055;
}

function oklchToRgb(l: number, c: number, hDeg: number): Rgb {
	const h = (hDeg * Math.PI) / 180;
	const a = c * Math.cos(h);
	const b = c * Math.sin(h);
	const l_ = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
	const m_ = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
	const s_ = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
	const r = 4.0767416621 * l_ - 3.3077115913 * m_ + 0.2309699292 * s_;
	const g = -1.2684380046 * l_ + 2.6097574011 * m_ - 0.3413193965 * s_;
	const bl = -0.0041960863 * l_ - 0.7034186147 * m_ + 1.707614701 * s_;
	return [r, g, bl].map((v) => srgbEncode(v) * 255) as Rgb;
}

// parse reads "oklch(L C H)" or "oklch(L C H / A)", A as a number or a percentage.
function parse(value: string): Colour {
	const m = value.match(/^oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)(?:\s*\/\s*([\d.]+)(%?))?\s*\)$/);
	if (!m) throw new Error(`not an oklch colour: ${value}`);
	const alpha = m[4] === undefined ? 1 : m[5] ? Number(m[4]) / 100 : Number(m[4]);
	return { rgb: oklchToRgb(Number(m[1]), Number(m[2]), Number(m[3])), alpha };
}

function colour(theme: Theme, name: string, seen: string[] = []): Colour {
	let value = themes[theme].get(`--${name}`);
	if (value === undefined) throw new Error(`no token --${name} in ${theme}`);
	const alias = value.match(/^var\(--([\w-]+)\)$/);
	if (alias) {
		if (seen.includes(name)) throw new Error(`token cycle at --${name}`);
		return colour(theme, alias[1], [...seen, name]);
	}
	value = value.trim();
	return parse(value);
}

// over paints a colour with its alpha (times `extra`) on an opaque surface.
function over(fg: Colour, extra: number, surface: Rgb): Rgb {
	const a = fg.alpha * extra;
	return fg.rgb.map((v, i) => v * a + surface[i] * (1 - a)) as Rgb;
}

function luminance([r, g, b]: Rgb): number {
	const lin = (v: number) => {
		const c = v / 255;
		return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
	};
	return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

function ratio(a: Rgb, b: Rgb): number {
	const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	return (hi + 0.05) / (lo + 0.05);
}

// A surface is a token painted over `base` (opaque tokens ignore it).
function surface(theme: Theme, name: string, base: Rgb = [255, 255, 255]): Rgb {
	return over(colour(theme, name), 1, base);
}

interface Pair {
	name: string;
	need: number;
	light: number;
	dark: number;
	// measure returns the ratio in a theme.
	measure: (theme: Theme) => number;
}

// plain is foreground token on background token.
function plain(fg: string, bg: string, need: number, light: number, dark: number, label = `${fg} on ${bg}`): Pair {
	return {
		name: label,
		need,
		light,
		dark,
		measure: (t) => {
			const back = surface(t, bg);
			return ratio(over(colour(t, fg), 1, back), back);
		},
	};
}

// tint is a state colour as text on its own tint: the colour at 10% (light) or
// 20% (dark) over a base surface.
function tint(fg: string, base: string, need: number, light: number, dark: number, label: string): Pair {
	return {
		name: label,
		need,
		light,
		dark,
		measure: (t) => {
			const back = surface(t, base);
			const fill = over(colour(t, fg), t === "light" ? 0.1 : 0.2, back);
			return ratio(over(colour(t, fg), 1, fill), fill);
		},
	};
}

// tintRange is the lowest or highest ratio of the four states on their own tint
// over the editor panel: the document gives the range, not each state.
function tintRange(label: string, pick: (...n: number[]) => number, light: number, dark: number): Pair {
	const states = ["success", "warning", "info", "agent"];
	return {
		name: `${label} of success, warning, info and agent on their tint over ed-panel`,
		need: 4.5,
		light,
		dark,
		measure: (t) => pick(...states.map((s) => tint(s, "ed-panel", 4.5, 0, 0, s).measure(t))),
	};
}

const T = "ed-timeline";
const S = "ed-preview-surround";

const pairs: Pair[] = [
	plain("foreground", "background", 4.5, 15.35, 17.57),
	plain("foreground", "ed-panel", 4.5, 16.89, 16.07),
	plain("muted-foreground", "background", 4.5, 6.87, 6.69),
	plain("muted-foreground", "muted", 4.5, 6.17, 5.38),
	plain("muted-foreground", T, 4.5, 6.17, 6.69),
	plain("primary-foreground", "primary", 4.5, 5.35, 11.21),
	plain("secondary-foreground", "secondary", 4.5, 13.79, 14.12),
	plain("destructive", "background", 4.5, 6.08, 8.26),
	tint("destructive", "background", 4.5, 5.07, 5.85, "destructive on its tint"),
	// The destructive button of a dialog or a menu sits on the popover, lighter than the background in dark.
	tint("destructive", "popover", 4.5, 5.56, 4.59, "destructive on its tint over popover"),
	plain("success", "background", 4.5, 5.59, 8.29),
	plain("warning", "background", 4.5, 6.37, 8.8),
	plain("info", "background", 4.5, 4.86, 10.44),
	plain("agent", "background", 4.5, 6.33, 10.44),
	plain("agent", "ed-panel", 4.5, 6.97, 9.55),
	tintRange("lowest", Math.min, 4.66, 5.28),
	tintRange("highest", Math.max, 5.98, 6.22),
	plain("brand-foreground", "brand", 4.5, 5.35, 11.21),
	plain("ring", "background", 3, 4.86, 10.44),
	plain("ring", T, 3, 4.37, 10.44),
	plain("input", "background", 3, 6.87, 6.69),
	plain("ed-track-line", T, 3, 6.17, 6.69),
	plain("ed-playhead", T, 3, 14.48, 18.88),
	plain("ed-snap", T, 3, 4.37, 10.44),
	plain("ed-ghost", T, 3, 11.29, 3.85),
	plain("ed-error-flash", T, 3, 3.83, 5.14),
	plain("ed-safe-area", S, 3, 4.32, 11.21),
	plain("ed-handle", S, 3, 4.32, 11.21),
	plain("ed-item-clip-fg", "ed-item-clip", 4.5, 5.17, 6.18),
	plain("ed-item-audio-fg", "ed-item-audio", 4.5, 5.02, 5.98),
	plain("ed-item-text-fg", "ed-item-text", 4.5, 6.97, 6.66),
	plain("ed-item-caption-fg", "ed-item-caption", 4.5, 6.03, 6.69),
	plain("ed-item-shape-fg", "ed-item-shape", 4.5, 5.19, 9.45),
	plain("ed-selection", "ed-item-clip", 3, 5.17, 6.18),
	plain("ed-selection", "ed-item-text", 3, 6.97, 6.66),
];

export interface Row {
	theme: Theme;
	name: string;
	got: number;
	want: number;
	need: number;
	problems: string[];
}

// evaluate measures every pair in both themes for the given stylesheet text.
export function evaluate(css: string): Row[] {
	themes = {
		light: block(css, ":root"),
		dark: new Map([...block(css, ":root"), ...block(css, ".dark")]),
	};
	const rows: Row[] = [];
	for (const p of pairs) {
		for (const theme of ["light", "dark"] as const) {
			const got = p.measure(theme);
			const want = theme === "light" ? p.light : p.dark;
			const problems: string[] = [];
			if (got < p.need) problems.push(`below ${p.need}:1`);
			if (Math.abs(got - want) > TOLERANCE)
				problems.push(`design-system.md says ${want.toFixed(2)}`);
			rows.push({ theme, name: p.name, got, want, need: p.need, problems });
		}
	}
	return rows;
}

function main(): void {
	const rows = evaluate(readFileSync(cssPath, "utf8"));
	let failures = 0;
	for (const r of rows) {
		if (r.problems.length > 0) failures++;
		console.log(
			`${r.problems.length === 0 ? "ok  " : "FAIL"} ${r.theme.padEnd(5)} ${r.got.toFixed(2).padStart(6)}  ${r.name}${r.problems.length ? `  (${r.problems.join(", ")})` : ""}`,
		);
	}
	if (failures > 0) {
		console.error(`check-contrast: ${failures} failing pair(s)`);
		process.exit(1);
	}
	console.log(`check-contrast: ${rows.length} pairs pass in both themes`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
	main();
}

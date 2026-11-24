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

// layered measures a colour on a translucent surface painted over the page background, as the
// active item of the sidebar (brand text on the brand tint) is drawn.
function layered(fg: string, bg: string, need: number, light: number, dark: number): Pair {
	return {
		name: `${fg} on ${bg} over background`,
		need,
		light,
		dark,
		measure: (t) => {
			const back = over(colour(t, bg), 1, surface(t, "background"));
			return ratio(over(colour(t, fg), 1, back), back);
		},
	};
}

const T = "ed-timeline";
const S = "ed-preview-surround";

const pairs: Pair[] = [
	// Text on every surface of the redesign.
	plain("color-text", "color-background", 4.5, 16.03, 15.72),
	plain("color-text-muted", "color-background", 4.5, 8.22, 9.87),
	plain("color-text-subtle", "color-background", 4.5, 5.75, 6.78),
	plain("color-text", "color-surface", 4.5, 16.88, 14.78),
	plain("color-text-muted", "color-surface", 4.5, 8.66, 9.28),
	plain("color-text-subtle", "color-surface", 4.5, 6.05, 6.38),
	plain("color-text", "color-surface-raised", 4.5, 16.88, 13.48),
	plain("color-text-muted", "color-surface-raised", 4.5, 8.66, 8.46),
	plain("color-text-subtle", "color-surface-raised", 4.5, 6.05, 5.81),
	plain("color-text", "color-surface-sunken", 4.5, 14.95, 16.56),
	plain("color-text-muted", "color-surface-sunken", 4.5, 7.67, 10.4),
	plain("color-text-subtle", "color-surface-sunken", 4.5, 5.36, 7.14),
	plain("color-text", "color-surface-hover", 4.5, 13.66, 12.08),
	plain("color-text-muted", "color-surface-hover", 4.5, 7.01, 7.58),
	plain("color-text-subtle", "color-surface-hover", 4.5, 4.9, 5.21),
	plain("color-text", "color-surface-selected", 4.5, 14.01, 14.08),
	plain("color-text-muted", "color-surface-selected", 4.5, 7.19, 8.84),
	plain("color-text-subtle", "color-surface-selected", 4.5, 5.02, 6.07),
	// The focus ring and the boundary of a control (3:1, WCAG 1.4.11).
	plain("color-focus", "color-background", 3, 7.4, 9.44),
	plain("color-focus", "color-surface", 3, 7.79, 8.87),
	plain("color-focus", "color-surface-raised", 3, 7.79, 8.09),
	plain("color-focus", "color-surface-sunken", 3, 6.9, 9.94),
	plain("color-focus", "color-surface-selected", 3, 6.47, 8.45),
	plain("color-border-strong", "color-background", 3, 3.71, 4.23),
	plain("color-border-strong", "color-surface", 3, 3.91, 3.98),
	plain("color-border-strong", "color-surface-raised", 3, 3.91, 3.63),
	plain("color-border-strong", "color-surface-sunken", 3, 3.46, 4.46),
	// Each family: its text on its soft surface, its fill with its text, and its colour as text on the page and on a card.
	plain("color-accent-text", "color-accent-soft", 4.5, 6.47, 7.26),
	plain("color-accent-text", "color-background", 4.5, 7.4, 9.44),
	plain("color-accent-text", "color-surface", 4.5, 7.79, 8.87),
	plain("color-agent-text", "color-agent-soft", 4.5, 5.15, 10.96),
	plain("color-agent-text", "color-background", 4.5, 5.72, 12.7),
	plain("color-agent-text", "color-surface", 4.5, 6.02, 11.93),
	plain("color-success-text", "color-success-soft", 4.5, 5.8, 10.06),
	plain("color-success-text", "color-background", 4.5, 6.34, 11.71),
	plain("color-success-text", "color-surface", 4.5, 6.67, 11.01),
	plain("color-warning-text", "color-warning-soft", 4.5, 5.47, 10.06),
	plain("color-warning-text", "color-background", 4.5, 5.94, 11.91),
	plain("color-warning-text", "color-surface", 4.5, 6.26, 11.19),
	plain("color-danger-text", "color-danger-soft", 4.5, 5.43, 8.67),
	plain("color-danger-text", "color-background", 4.5, 6.28, 9.82),
	plain("color-danger-text", "color-surface", 4.5, 6.61, 9.23),
	plain("color-text-on-accent", "color-accent", 4.5, 7.79, 5.01),
	plain("color-text-on-agent", "color-agent", 4.5, 6.02, 11.12),
	plain("color-text-on-danger", "color-danger", 4.5, 6.61, 5.28),
	plain("color-text-on-accent", "color-accent-hover", 4.5, 8.95, 6.28),
	plain("color-text-on-danger", "color-danger-hover", 4.5, 5.28, 6.61),
	plain("color-success", "color-background", 4.5, 6.34, 5.95),
	plain("color-success", "color-surface", 4.5, 6.67, 5.59),
	plain("color-warning", "color-background", 4.5, 5.94, 8.19),
	plain("color-warning", "color-surface", 4.5, 6.26, 7.7),
	plain("color-agent", "color-background", 4.5, 5.72, 10.56),
	plain("color-agent", "color-surface", 4.5, 6.02, 9.92),
	// Names of the former system, kept as aliases (section 2.1).
	plain("muted-foreground", "muted", 4.5, 7.67, 10.4),
	plain("primary-foreground", "primary", 4.5, 7.79, 5.01),
	plain("secondary-foreground", "secondary", 4.5, 16.88, 13.48),
	plain("destructive", "background", 4.5, 6.28, 9.82),
	tint("destructive", "background", 4.5, 5.34, 6.42, "destructive on its tint"),
	// The destructive button of a dialog or a menu sits on the popover.
	tint("destructive", "popover", 4.5, 5.61, 5.41, "destructive on its tint over popover"),
	plain("info", "background", 4.5, 7.4, 9.44),
	tint("color-agent", "ed-panel", 4.5, 5.22, 6.33, "agent on its tint over ed-panel"),
	plain("color-agent", "ed-panel", 4.5, 6.02, 9.92),
	plain("brand-foreground", "brand", 4.5, 8.28, 5.66),
	plain("brand-text", "background", 4.5, 8, 9.23),
	layered("brand-text", "brand-subtle", 4.5, 7.67, 8.66),
	plain("ring", "background", 3, 7.4, 9.44),
	plain("ring", T, 3, 6.9, 9.94),
	plain("input", "background", 3, 3.71, 4.23),
	// The editor.
	plain("ed-track-line", T, 3, 3.46, 4.46),
	plain("ed-playhead", T, 3, 15.71, 19.01),
	plain("ed-snap", T, 3, 6.9, 9.94),
	plain("ed-ghost", T, 3, 4.08, 3.74),
	plain("ed-error-flash", T, 3, 4.16, 5.18),
	plain("ed-safe-area", S, 3, 9.94, 9.94),
	plain("ed-handle", S, 3, 9.94, 9.94),
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
	// The aliases of the former names sit in one rule for both themes.
	const aliases = block(css, ":root,\n.dark");
	themes = {
		light: new Map([...block(css, ":root"), ...aliases]),
		dark: new Map([...block(css, ":root"), ...aliases, ...block(css, ".dark")]),
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

// Checks the built site in out/ (run `npm run build` first, then `npm run check:built`):
//   1. only static files: no server entry, no adapter output (D-015);
//   2. no external script, font, stylesheet, image or analytics: nothing the page loads comes from another
//      origin, and no other host appears in the built files except the inert ones listed below (D-015, D-016);
//   3. every root-relative href and src starts with the base path (SDD-010 section 12.4).
// Node strips the types (no build step).
import fs from "node:fs";
import path from "node:path";

const outDir = path.join(import.meta.dirname, "..", "out");

/** Hosts that may appear as text: namespaces, error-message links and the places the site links to. */
const INERT_HOSTS = new Set(["www.w3.org", "www.sitemaps.org", "react.dev", "base-ui.com", "semver.org", "keepachangelog.com", "github.com", "127.0.0.1"]);

/** Attributes whose value the browser fetches by itself when the page loads. */
const LOADING = /<(script|img|source|iframe|video|audio|embed|track|input)\b[^>]*?\b(?:src|srcset|poster)\s*=\s*"([^"]*)"|<link\b[^>]*?\bhref\s*=\s*"([^"]*)"[^>]*>/gi;

export function hostsIn(text: string): string[] {
	return [...new Set([...text.matchAll(/\bhttps?:\/\/([a-z0-9.-]+)/gi)].map((m) => m[1].toLowerCase()))];
}

/** Problems of the "no external origin" property for one built file. */
export function foreignOriginProblems(file: string, text: string, ownHost: string): string[] {
	const problems: string[] = [];
	for (const host of hostsIn(text)) {
		if (host !== ownHost && !INERT_HOSTS.has(host)) problems.push(`${file}: refers to ${host}`);
	}
	if (file.endsWith(".html")) {
		for (const m of text.matchAll(LOADING)) {
			const value = m[2] ?? m[3] ?? "";
			if (/^(?:https?:)?\/\//i.test(value) && hostsIn(value.replace(/^\/\//, "https://"))[0] !== ownHost) {
				problems.push(`${file}: loads ${value}`);
			}
		}
	}
	if (/@import\s+(?:url\()?["']?(?:https?:)?\/\//i.test(text) && file.endsWith(".css")) problems.push(`${file}: @import of a remote stylesheet`);
	if (/\burl\(\s*["']?(?:https?:)?\/\/(?!(?:www\.w3\.org)\b)/i.test(text) && file.endsWith(".css")) problems.push(`${file}: url() of a remote resource`);
	return problems;
}

/** Root-relative href and src values that miss the base path. */
export function basePathProblems(file: string, html: string, base: string): string[] {
	if (base === "") return [];
	const problems: string[] = [];
	for (const m of html.matchAll(/\b(?:href|src)\s*=\s*"(\/[^"]*)"/g)) {
		const value = m[1];
		if (value.startsWith("//")) continue;
		if (value !== base && !value.startsWith(`${base}/`)) problems.push(`${file}: ${value} does not start with ${base}`);
	}
	return problems;
}

function walk(dir: string): string[] {
	return fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]));
}

function main(): number {
	if (!fs.existsSync(outDir)) {
		console.error("out/ does not exist: run `npm run build` first");
		return 1;
	}
	const siteUrl = new URL(process.env.SITE_URL || "https://alexgalhardo.github.io/vukem-video");
	const base = siteUrl.pathname.replace(/\/$/, "");
	const problems: string[] = [];
	const files = walk(outDir);

	for (const file of files) {
		const rel = path.relative(outDir, file).split(path.sep).join("/");
		if (/(^|\/)(server|_worker\.js|entry\.mjs|\.vercel|\.netlify)(\/|$)/.test(rel) || /\.(mjs|cjs)$/.test(rel) && !rel.startsWith("_astro/")) {
			problems.push(`${rel}: server output in a static site`);
		}
		if (!/\.(html|js|css|svg|json|txt|xml|md)$/.test(rel)) continue;
		const text = fs.readFileSync(file, "utf8");
		problems.push(...foreignOriginProblems(rel, text, siteUrl.hostname));
		if (rel.endsWith(".html")) problems.push(...basePathProblems(rel, text, base));
	}

	for (const p of problems) console.error(p);
	console.log(`check:built: ${files.length} files, ${problems.length} problems`);
	return problems.length > 0 ? 1 : 0;
}

if (import.meta.main) process.exit(main());

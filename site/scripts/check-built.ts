// Checks the built site in out/ (run `npm run build` first, then `npm run check:built`):
//   1. only static files: no server entry, no adapter output (D-015);
//   2. no external script, font, stylesheet, image or analytics: nothing the page loads comes from another
//      origin, and no other host appears in the built files except the inert ones listed below (D-015, D-016);
//   3. every root-relative href and src starts with the base path (SDD-010 section 12.4);
//   4. every docs page has a clean Markdown twin and llms.txt lists them, and what it lists exists.
// Node strips the types (no build step).
import fs from "node:fs";
import path from "node:path";

const outDir = path.join(import.meta.dirname, "..", "out");

/** Hosts that may appear as text: namespaces, error-message links and the places the site links to. */
const INERT_HOSTS = new Set([
	"www.w3.org",
	"www.sitemaps.org",
	"react.dev",
	"base-ui.com",
	"semver.org",
	"keepachangelog.com",
	"github.com",
	"127.0.0.1",
]);

/** Attributes whose value the browser fetches by itself when the page loads. */
const LOADING =
	/<(script|img|source|iframe|video|audio|embed|track|input)\b[^>]*?\b(?:src|srcset|poster)\s*=\s*"([^"]*)"|<link\b[^>]*?\bhref\s*=\s*"([^"]*)"[^>]*>/gi;

/** The host of every http(s) URL in text, read with the URL parser rather than a pattern on the host. */
export function hostsIn(text: string): string[] {
	const hosts = new Set<string>();
	for (const [candidate] of text.matchAll(/\bhttps?:\/\/[^\s"'<>()\\]+/gi)) {
		try {
			hosts.add(new URL(candidate).hostname.toLowerCase());
		} catch {
			// Not a URL (a template fragment such as "https://${host}"): nothing to compare.
		}
	}
	return [...hosts];
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
	if (/@import\s+(?:url\()?["']?(?:https?:)?\/\//i.test(text) && file.endsWith(".css"))
		problems.push(`${file}: @import of a remote stylesheet`);
	if (/\burl\(\s*["']?(?:https?:)?\/\/(?!(?:www\.w3\.org)\b)/i.test(text) && file.endsWith(".css"))
		problems.push(`${file}: url() of a remote resource`);
	return problems;
}

/** Root-relative href and src values that miss the base path. */
export function basePathProblems(file: string, html: string, base: string): string[] {
	if (base === "") return [];
	const problems: string[] = [];
	for (const m of html.matchAll(/\b(?:href|src)\s*=\s*"(\/[^"]*)"/g)) {
		const value = m[1];
		if (value.startsWith("//")) continue;
		if (value !== base && !value.startsWith(`${base}/`))
			problems.push(`${file}: ${value} does not start with ${base}`);
	}
	return problems;
}

/**
 * Twins and llms.txt (SDD-010 section 9, properties 1 to 3): every docs page has a Markdown twin
 * without JSX or imports, llms.txt lists every page and the twin of each, and every URL it lists
 * resolves to a file. `files` are paths relative to out/ with forward slashes.
 */
export function twinProblems(
	files: string[],
	read: (rel: string) => string,
	siteUrl: string,
): string[] {
	const problems: string[] = [];
	const set = new Set(files);
	const pages = files
		.filter((f) => /^docs\/.+\/index\.html$/.test(f))
		.map((f) => f.slice(0, -"/index.html".length));
	if (!set.has("llms.txt")) return ["llms.txt is missing"];
	const listed = new Set(
		[...read("llms.txt").matchAll(/\]\((https?:\/\/[^)\s]+)\)/g)].map((m) => m[1].replace(siteUrl, "").replace(/^\//, "").replace(/\/$/, "")),
	);

	for (const page of pages) {
		if (!set.has(`${page}.md`)) problems.push(`${page}: no Markdown twin (${page}.md)`);
		else {
			const md = read(`${page}.md`);
			if (/^import\s/m.test(md) || /<[A-Z][A-Za-z]*[\s/>]/.test(md)) problems.push(`${page}.md: has JSX or an import`);
		}
		if (!listed.has(page)) problems.push(`llms.txt does not list ${page}`);
	}
	for (const url of listed) {
		if (!set.has(`${url}/index.html`) && !set.has(url)) problems.push(`llms.txt lists ${url}, which is not in out/`);
		if (!pages.includes(url) && url.startsWith("docs/")) problems.push(`llms.txt lists ${url}, which is not a page`);
	}
	return problems;
}

function walk(dir: string): string[] {
	return fs
		.readdirSync(dir, { withFileTypes: true })
		.flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]));
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
		if (
			/(^|\/)(server|_worker\.js|entry\.mjs|\.vercel|\.netlify)(\/|$)/.test(rel) ||
			(/\.(mjs|cjs)$/.test(rel) && !rel.startsWith("_astro/"))
		) {
			problems.push(`${rel}: server output in a static site`);
		}
		if (!/\.(html|js|css|svg|json|txt|xml|md)$/.test(rel)) continue;
		const text = fs.readFileSync(file, "utf8");
		problems.push(...foreignOriginProblems(rel, text, siteUrl.hostname));
		if (rel.endsWith(".html")) problems.push(...basePathProblems(rel, text, base));
	}

	const rels = files.map((f) => path.relative(outDir, f).split(path.sep).join("/"));
	problems.push(...twinProblems(rels, (rel) => fs.readFileSync(path.join(outDir, rel), "utf8"), siteUrl.origin + base));

	for (const p of problems) console.error(p);
	console.log(`check:built: ${files.length} files, ${problems.length} problems`);
	return problems.length > 0 ? 1 : 0;
}

if (import.meta.main) process.exit(main());

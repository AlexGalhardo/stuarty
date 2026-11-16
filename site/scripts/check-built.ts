// Checks the built site in out/ (run `npm run build` first, then `npm run check:built`):
//   1. only static files: no server entry, no adapter output (D-015);
//   2. no external script, font, stylesheet, image or analytics: nothing the page loads comes from another
//      origin, and no other host appears in the built files except the inert ones listed below (D-015, D-016);
//   3. every root-relative href and src starts with the base path (SDD-010 section 12.4);
//   4. every docs page has a clean Markdown twin and llms.txt lists them, and what it lists exists;
//   5. the search index answers the queries of SDD-010 section 14 check 8: `trim` finds the editing tools page
//      and `keyframes` the keyframes page.
// Node strips the types (no build step).
import fs from "node:fs";
import path from "node:path";
import { search } from "../lib/search-rank.ts";

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
	"localhost",
	// Example names of the self-hosting pages (a domain you would own); text, never loaded.
	"video.example.com",
	"collector.example.com",
	"www.conventionalcommits.org",
	"caddyserver.com",
	"opentimelineio.org",
	"taskfile.dev",
	// The OIDC issuer that the cosign commands of the verification page pin; it is text, never loaded.
	"token.actions.githubusercontent.com",
]);


/** Attributes whose value the browser fetches by itself when the page loads. */
const LOADING =
	/<(script|img|source|iframe|video|audio|embed|track|input)\b[^>]*?\b(?:src|srcset|poster)\s*=\s*"([^"]*)"|<link\b[^>]*?\bhref\s*=\s*"([^"]*)"[^>]*>/gi;

/** The host of every http(s) URL in text, read with the URL parser rather than a pattern on the host. */
export function hostsIn(text: string): string[] {
	const hosts = new Set<string>();
	for (const [candidate] of text.matchAll(/\bhttps?:\/\/[^\s"'<>()`${}\\]+/gi)) {
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
 * resolves to a file. `files` are paths relative to out/ with forward slashes. The Portuguese files
 * (`pt-br/...`) are checked the same way, except that a page that is not translated may be listed by
 * its English URL (the "Untranslated" heading).
 */
export function twinProblems(files: string[], read: (rel: string) => string, siteUrl: string): string[] {
	const problems: string[] = [];
	const set = new Set(files);
	const pagesOf = (prefix: string) =>
		files
			.filter((f) => f.startsWith(`${prefix}docs/`) && f.endsWith("/index.html") && f !== `${prefix}docs/index.html`)
			.map((f) => f.slice(0, -"/index.html".length));
	const listedIn = (llms: string) =>
		new Set(
			[...read(llms).matchAll(/\]\((https?:\/\/[^)\s]+)\)/g)].map((m) =>
				m[1].replace(siteUrl, "").replace(/^\//, "").replace(/\/$/, ""),
			),
		);

	for (const prefix of ["", "pt-br/"]) {
		const pages = pagesOf(prefix);
		if (prefix !== "" && pages.length === 0) continue;
		const llms = `${prefix}llms.txt`;
		if (!set.has(llms)) {
			problems.push(`${llms} is missing`);
			continue;
		}
		const listed = listedIn(llms);

		for (const page of pages) {
			if (!set.has(`${page}.md`)) problems.push(`${page}: no Markdown twin (${page}.md)`);
			else {
				const md = read(`${page}.md`);
				if (/^import\s/m.test(md) || /<[A-Z][A-Za-z]*[\s/>]/.test(md)) problems.push(`${page}.md: has JSX or an import`);
			}
			const english = page.replace(/^pt-br\//, "");
			if (!listed.has(page) && !(prefix !== "" && listed.has(english))) problems.push(`${llms} does not list ${page}`);
		}
		for (const url of listed) {
			if (!set.has(`${url}/index.html`) && !set.has(url)) problems.push(`${llms} lists ${url}, which is not in out/`);
			if (url.startsWith("docs/") && !pagesOf("").includes(url)) problems.push(`${llms} lists ${url}, which is not a page`);
		}
	}
	return problems;
}

const attr = (html: string, rel: string): { hreflang?: string; href: string }[] =>
	[...html.matchAll(/<link\b[^>]*>/g)]
		.map((m) => m[0])
		.filter((tag) => new RegExp(`\\brel="${rel}"`).test(tag))
		.map((tag) => ({
			hreflang: /\bhreflang="([^"]*)"/.exec(tag)?.[1],
			href: /\bhref="([^"]*)"/.exec(tag)?.[1] ?? "",
		}));

/**
 * The locale properties of SDD-010 sections 4 and 14 on the built pages: every English page has a
 * Portuguese page and twin; a translated pair names each other symmetrically; a page that is not
 * translated names the English page as canonical and claims no alternate and its twin says so; every
 * page has one canonical that exists; the sitemap lists no untranslated page and no twin.
 */
export function localeProblems(files: string[], read: (rel: string) => string, siteUrl: string): string[] {
	const problems: string[] = [];
	const set = new Set(files);
	const at = (url: string) => url.replace(siteUrl, "").replace(/^\//, "").replace(/\/$/, "");
	const englishPages = files
		.filter((f) => f.startsWith("docs/") && f.endsWith("/index.html") && f !== "docs/index.html")
		.map((f) => f.slice(0, -"/index.html".length));
	const sitemap = set.has("sitemap.xml") ? read("sitemap.xml") : "";

	for (const en of englishPages) {
		const pt = `pt-br/${en}`;
		for (const f of [`${pt}/index.html`, `${en}.md`, `${pt}.md`]) {
			if (!set.has(f)) problems.push(`${en}: ${f} is missing`);
		}
		if (!set.has(`${pt}/index.html`)) continue;

		const enHtml = read(`${en}/index.html`);
		const ptHtml = read(`${pt}/index.html`);
		const enCanonical = attr(enHtml, "canonical");
		const ptCanonical = attr(ptHtml, "canonical");
		const enAlternates = attr(enHtml, "alternate").filter((a) => a.hreflang);
		const ptAlternates = attr(ptHtml, "alternate").filter((a) => a.hreflang);

		for (const [page, canon] of [
			[en, enCanonical],
			[pt, ptCanonical],
		] as const) {
			if (canon.length !== 1) problems.push(`${page}: ${canon.length} canonical links, want 1`);
			else if (!set.has(`${at(canon[0].href)}/index.html`)) problems.push(`${page}: canonical ${canon[0].href} is not in out/`);
		}

		const fallback = ptCanonical[0] !== undefined && at(ptCanonical[0].href) === en;
		const ptLangMark = /<html[^>]*\blang="pt-BR"/.test(ptHtml);
		if (!ptLangMark) problems.push(`${pt}: html lang is not pt-BR`);

		if (fallback) {
			if (ptAlternates.length > 0) problems.push(`${pt}: a page that is not translated has hreflang alternates`);
			if (!read(`${pt}.md`).startsWith("> Not translated yet")) problems.push(`${pt}.md: does not say it is not translated`);
			if (sitemap.includes(`/${pt}/`)) problems.push(`sitemap.xml lists ${pt}, which is not translated`);
			if (enAlternates.some((a) => a.hreflang === "pt-BR")) problems.push(`${en}: claims a pt-BR alternate that is only a fallback`);
		} else {
			const names = (alts: { hreflang?: string; href: string }[], lang: string) => alts.find((a) => a.hreflang === lang)?.href;
			if (at(names(enAlternates, "pt-BR") ?? "") !== pt) problems.push(`${en}: hreflang pt-BR should point to ${pt}`);
			if (at(names(enAlternates, "en") ?? "") !== en) problems.push(`${en}: hreflang en should point to itself`);
			if (at(names(ptAlternates, "en") ?? "") !== en) problems.push(`${pt}: hreflang en should point to ${en}`);
			if (at(names(ptAlternates, "pt-BR") ?? "") !== pt) problems.push(`${pt}: hreflang pt-BR should point to itself`);
			if (at(names(ptAlternates, "x-default") ?? "") !== en) problems.push(`${pt}: x-default should be the English page`);
			if (sitemap && !sitemap.includes(`/${pt}/`)) problems.push(`sitemap.xml does not list ${pt}`);
		}
	}

	if (/\.md</.test(sitemap)) problems.push("sitemap.xml lists a Markdown twin");
	if (set.has("404.html")) {
		const html = read("404.html");
		if (!/data-lang="en"/.test(html) || !/data-lang="pt-br"/.test(html)) problems.push("404.html does not hold both languages");
	} else problems.push("404.html is missing");

	return problems;
}

function walk(dir: string): string[] {
	return fs
		.readdirSync(dir, { withFileTypes: true })
		.flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]));
}

/** The queries that must find a page: the first result of each is on that page (SDD-010 section 14 check 8). The index holds paths without the base path. */
export const SEARCH_EXPECTATIONS: { query: string; page: string }[] = [
	{ query: "trim", page: "/docs/guide/editing-tools" },
	{ query: "keyframes", page: "/docs/guide/keyframes" },
];

type IndexEntry = { title: string; heading?: string; href: string; text: string };

/** Runs the expected queries against the search index of the build, ranked as the search dialog ranks. */
export function searchProblems(read: (rel: string) => string, expectations = SEARCH_EXPECTATIONS): string[] {
	let index: IndexEntry[];
	try {
		index = JSON.parse(read("search-index.json")) as IndexEntry[];
	} catch {
		return ["search-index.json is missing or is not JSON"];
	}
	const problems: string[] = [];
	for (const { query, page } of expectations) {
		const top = search(index, query)[0];
		const where = top ? top.href.split("#")[0] : undefined;
		if (where !== page) problems.push(`search "${query}": the first result is ${where ?? "nothing"}, want ${page}`);
	}
	return problems;
}

function main(): number {
	if (!fs.existsSync(outDir)) {
		console.error("out/ does not exist: run `npm run build` first");
		return 1;
	}
	const siteUrl = new URL(process.env.SITE_URL || "https://video.vukem.dev");
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
	const read = (rel: string) => fs.readFileSync(path.join(outDir, rel), "utf8");
	problems.push(...twinProblems(rels, read, siteUrl.origin + base));
	problems.push(...localeProblems(rels, read, siteUrl.origin + base));
	problems.push(...searchProblems(read));

	for (const p of problems) console.error(p);
	console.log(`check:built: ${files.length} files, ${problems.length} problems`);
	return problems.length > 0 ? 1 : 0;
}

if (import.meta.main) process.exit(main());

import fs from "node:fs";
import path from "node:path";
import matter from "gray-matter";
import { CONTENT_ROOT, navigation, pagePath, pageSource, type PageKind } from "@/lib/page-files";
import { localizePath, type Locale } from "@/lib/locale";
import { lastUpdated } from "@/lib/last-updated";
import { extractHeadings, type Heading } from "@/lib/toc";

export type DocMeta = {
	/** URL segments after /docs, e.g. ["getting-started"]. */
	slug: string[];
	/** Path under site/content, e.g. "docs/en/getting-started/introduction.mdx". */
	source: string;
	/** The path the page is served at in its locale, without the base path and the trailing slash. */
	href: string;
	/** The same page in the English tree: /docs/<prefix or dir>/<slug>. */
	englishHref: string;
	/** `<dir>/<slug>`: the identity of the page across locales. */
	id: string;
	locale: Locale;
	kind: PageKind;
	title: string;
	description: string;
	category: string;
	/** ISO 8601 date of the source file's last commit, if known. */
	updated?: string;
};

export type Doc = DocMeta & {
	body: string;
	headings: Heading[];
};

function readDoc(source: string): { title: string; description: string; body: string } {
	const raw = fs.readFileSync(path.join(CONTENT_ROOT, source), "utf8");
	const { data, content } = matter(raw);
	if (typeof data.title !== "string" || typeof data.description !== "string") {
		throw new Error(`content/${source}: front matter needs "title" and "description"`);
	}
	return { title: data.title, description: data.description, body: content };
}

const cache = new Map<Locale, Doc[]>();

/** Every page of a locale in navigation order. A page without a translation is the English page. */
export function getAllDocs(locale: Locale): Doc[] {
	const hit = cache.get(locale);
	if (hit) return hit;

	const docs = navigation.flatMap((cat) =>
		cat.pages.map((page): Doc => {
			const src = pageSource(locale, cat.dir, page);
			if (!src) throw new Error(`content/navigation.ts lists ${cat.dir}/${page}, which has no file`);
			const { title, description, body } = readDoc(src.file);
			const englishHref = pagePath(cat, page);
			return {
				slug: englishHref.slice("/docs/".length).split("/"),
				source: src.file,
				href: localizePath(locale, englishHref),
				englishHref,
				id: `${cat.dir}/${page}`,
				locale,
				kind: src.kind,
				title,
				description,
				category: cat.title[locale],
				updated: lastUpdated(src.file),
				body,
				headings: extractHeadings(body),
			};
		}),
	);
	cache.set(locale, docs);
	return docs;
}

export function getDoc(locale: Locale, slug: string[]): Doc | undefined {
	const key = slug.join("/");
	return getAllDocs(locale).find((d) => d.slug.join("/") === key);
}

export function getNeighbours(doc: DocMeta): { prev?: DocMeta; next?: DocMeta } {
	const all = getAllDocs(doc.locale);
	const i = all.findIndex((d) => d.href === doc.href);
	return { prev: all[i - 1], next: all[i + 1] };
}

export type NavGroup = { title: string; items: { title: string; href: string }[] };

export function getNavGroups(locale: Locale): NavGroup[] {
	const all = getAllDocs(locale);
	return navigation.map((cat) => ({
		title: cat.title[locale],
		items: all.filter((d) => d.category === cat.title[locale]).map((d) => ({ title: d.title, href: d.href })),
	}));
}

/** Whether a page has text of its own in the locale: a translation, a generated page or the English page itself. */
export function isTranslated(doc: DocMeta): boolean {
	return doc.kind !== "fallback";
}

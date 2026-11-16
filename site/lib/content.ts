import fs from "node:fs";
import path from "node:path";
import matter from "gray-matter";
import { CONTENT_ROOT, navigation, pagePath, pageSource, type PageKind } from "@/lib/page-files";
import { lastUpdated } from "@/lib/last-updated";
import { extractHeadings, type Heading } from "@/lib/toc";

export type DocMeta = {
	/** URL segments after /docs, e.g. ["getting-started"]. */
	slug: string[];
	/** Path under site/content, e.g. "docs/en/getting-started/introduction.mdx". */
	source: string;
	/** The path the page is served at, without the base path and the trailing slash. */
	href: string;
	/** `<dir>/<slug>`: the identity of the page. */
	id: string;
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

let cache: Doc[] | undefined;

/** Every page in navigation order. */
export function getAllDocs(): Doc[] {
	if (cache) return cache;

	cache = navigation.flatMap((cat) =>
		cat.pages.map((page): Doc => {
			const src = pageSource(cat.dir, page);
			if (!src) throw new Error(`content/navigation.ts lists ${cat.dir}/${page}, which has no file`);
			const { title, description, body } = readDoc(src.file);
			const href = pagePath(cat, page);
			return {
				slug: href.slice("/docs/".length).split("/"),
				source: src.file,
				href,
				id: `${cat.dir}/${page}`,
				kind: src.kind,
				title,
				description,
				category: cat.title,
				updated: lastUpdated(src.file),
				body,
				headings: extractHeadings(body),
			};
		}),
	);
	return cache;
}

export function getDoc(slug: string[]): Doc | undefined {
	const key = slug.join("/");
	return getAllDocs().find((d) => d.slug.join("/") === key);
}

export function getNeighbours(doc: DocMeta): { prev?: DocMeta; next?: DocMeta } {
	const all = getAllDocs();
	const i = all.findIndex((d) => d.href === doc.href);
	return { prev: all[i - 1], next: all[i + 1] };
}

export type NavGroup = { title: string; items: { title: string; href: string }[] };

export function getNavGroups(): NavGroup[] {
	const all = getAllDocs();
	return navigation.map((cat) => ({
		title: cat.title,
		items: all.filter((d) => d.category === cat.title).map((d) => ({ title: d.title, href: d.href })),
	}));
}

import fs from "node:fs";
import path from "node:path";
import matter from "gray-matter";
import { navigation } from "@/content/navigation";
import { lastUpdated } from "@/lib/last-updated";
import { extractHeadings, type Heading } from "@/lib/toc";

const CONTENT_ROOT = path.join(process.cwd(), "content");

export type DocMeta = {
	/** URL segments after /docs, e.g. ["getting-started"]. */
	slug: string[];
	/** Path under site/content, e.g. "docs/introduction.mdx". */
	source: string;
	href: string;
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

function makeDoc(source: string, slug: string[], category: string): Doc {
	const { title, description, body } = readDoc(source);
	return {
		slug,
		source,
		href: `/docs/${slug.join("/")}`,
		title,
		description,
		category,
		updated: lastUpdated(source),
		body,
		headings: extractHeadings(body),
	};
}

let docsCache: Doc[] | undefined;

/** Every page in navigation order. Throws on missing files or front matter. */
export function getAllDocs(): Doc[] {
	docsCache ??= navigation.flatMap((cat) =>
		cat.pages.map((page) => {
			const prefix = cat.prefix ?? cat.dir;
			const slug = [prefix, page].filter((s) => s !== "");
			return makeDoc(`docs/${cat.dir}/${page}.mdx`, slug, cat.title);
		}),
	);
	return docsCache;
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

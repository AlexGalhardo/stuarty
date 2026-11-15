import GithubSlugger from "github-slugger";
import { getAllDocs } from "@/lib/content";
import type { Locale } from "@/lib/locale";
import { markdownToPlain, mdxToMarkdown } from "@/lib/text";
import { plainInline } from "@/lib/toc";

export type SearchEntry = {
	/** Page title. */
	title: string;
	/** Section heading, absent for the page entry. */
	heading?: string;
	href: string;
	category: string;
	text: string;
	/** An untranslated page, indexed in English so that a search still finds it. */
	fallback?: boolean;
};

const MAX_TEXT = 700;

const fallback = (doc: { kind: string }) => (doc.kind === "fallback" ? { fallback: true } : {});

/** One entry per page plus one per h2/h3 section. */
export function buildSearchIndex(locale: Locale): SearchEntry[] {
	const entries: SearchEntry[] = [];

	for (const doc of getAllDocs(locale)) {
		const md = mdxToMarkdown(doc.body);
		const slugger = new GithubSlugger();
		let heading: string | undefined;
		let id = "";
		let buf: string[] = [];
		let inFence = false;

		const flush = () => {
			const text = markdownToPlain(buf.join("\n")).slice(0, MAX_TEXT);
			if (heading) {
				entries.push({ title: doc.title, heading, href: `${doc.href}#${id}`, category: doc.category, text, ...fallback(doc) });
			} else {
				entries.push({
					title: doc.title,
					href: doc.href,
					category: doc.category,
					text: `${doc.description} ${text}`.trim(),
					...fallback(doc),
				});
			}
			buf = [];
		};

		for (const line of md.split("\n")) {
			if (/^\s*```/.test(line)) inFence = !inFence;
			const h = !inFence && line.match(/^(#{1,6})\s+(.+)$/);
			if (h) {
				const text = plainInline(h[2]);
				const slug = slugger.slug(text);
				if (h[1].length === 2 || h[1].length === 3) {
					flush();
					heading = text;
					id = slug;
					continue;
				}
			}
			buf.push(line);
		}
		flush();
	}

	return entries;
}

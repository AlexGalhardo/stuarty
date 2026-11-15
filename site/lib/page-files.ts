// Where the file of a page is, per locale (SDD-010 sections 2.2 and 4.2). Shared by the site and the
// scripts, so it imports nothing through the "@/" alias. Node only.
import fs from "node:fs";
import path from "node:path";
import { navigation, type NavCategory } from "../content/navigation.ts";
import type { Locale } from "./locale.ts";

export const CONTENT_ROOT = path.join(process.cwd(), "content");

/** How a page reached a locale: its own file, the English file (not translated yet) or a generated file. */
export type PageKind = "translated" | "fallback" | "generated" | "english";

export type PageSource = {
	/** Path under content/, with forward slashes. */
	file: string;
	kind: PageKind;
};

function exists(file: string): boolean {
	return fs.existsSync(path.join(CONTENT_ROOT, file));
}

/** The file that renders `<dir>/<slug>` in a locale, or undefined when there is none at all. */
export function pageSource(locale: Locale, dir: string, slug: string): PageSource | undefined {
	const own = `docs/${locale}/${dir}/${slug}.mdx`;
	const english = `docs/en/${dir}/${slug}.mdx`;
	const generated = `generated/${dir}/${slug}.mdx`;

	if (exists(own)) return { file: own, kind: locale === "en" ? "english" : "translated" };
	if (exists(generated)) return { file: generated, kind: "generated" };
	if (locale !== "en" && exists(english)) return { file: english, kind: "fallback" };
	return undefined;
}

/** The URL of a page in the English tree, without a trailing slash: /docs/<prefix or dir>/<slug>. */
export function pagePath(cat: NavCategory, slug: string): string {
	const segs = [cat.prefix ?? cat.dir, slug].filter((s) => s !== "");
	return `/docs/${segs.join("/")}`;
}

/** Every English file under content/docs/en (the source of truth), relative to content/. */
export function englishFiles(): string[] {
	const root = path.join(CONTENT_ROOT, "docs", "en");
	const walk = (dir: string): string[] =>
		fs
			.readdirSync(dir, { withFileTypes: true })
			.flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]));
	return fs.existsSync(root)
		? walk(root)
				.filter((f) => f.endsWith(".mdx"))
				.map((f) => path.relative(CONTENT_ROOT, f).split(path.sep).join("/"))
				.sort()
		: [];
}

export { navigation };

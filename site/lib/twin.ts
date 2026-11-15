import type { Doc } from "@/lib/content";
import { site } from "@/lib/site";
import { mdxToMarkdown } from "@/lib/text";
import { withTrailingSlash } from "@/lib/trailing-slash";

/** The Markdown twin of a page (SDD-010 section 9). An untranslated page says so on its first line. */
export function twinOf(doc: Doc): string {
	return [
		...(doc.kind === "fallback" ? ["> Not translated yet: this is the English text.", ""] : []),
		`# ${doc.title}`,
		"",
		`> ${doc.description}`,
		"",
		`Source: ${site.url}${withTrailingSlash(doc.href)}`,
		"",
		mdxToMarkdown(doc.body),
		"",
	].join("\n");
}

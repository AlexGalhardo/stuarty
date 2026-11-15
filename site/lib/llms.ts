import { getAllDocs, getNavGroups, isTranslated } from "@/lib/content";
import { localizePath, type Locale } from "@/lib/locale";
import { site } from "@/lib/site";
import { mdxToMarkdown } from "@/lib/text";
import { withTrailingSlash } from "@/lib/trailing-slash";

const SUMMARY: Record<Locale, string> = {
	en: "Vukem Video is a minimalist, open source video editor operated by AI agents (MCP, CLI, HTTP API) and by people in a browser UI. It is in the BETA phase.",
	"pt-br":
		"O Vukem Video é um editor de vídeo minimalista e de código aberto, operado por agentes de IA (MCP, CLI, API HTTP) e por pessoas em uma interface no navegador. Está na fase BETA.",
};

const url = (path: string) => `${site.url}${withTrailingSlash(path)}`;

/** `llms.txt` of a locale (SDD-010 section 9). The Portuguese file lists untranslated pages by their English URL. */
export function llmsTxt(locale: Locale): string {
	const docs = new Map(getAllDocs(locale).map((d) => [d.href, d]));
	const lines = [
		`# ${site.name}`,
		"",
		`> ${SUMMARY[locale]}`,
		"",
		`Full documentation in one file: ${site.url}${localizePath(locale, "/llms-full.txt")}`,
		"",
		`Every page is also available as Markdown: add ".md" to its URL (${site.url}${localizePath(locale, "/docs/introduction")}.md).`,
		"",
	];
	const untranslated: string[] = [];

	for (const group of getNavGroups(locale)) {
		const own = group.items.filter((i) => docs.get(i.href) && isTranslated(docs.get(i.href)!));
		if (own.length > 0) lines.push(`## ${group.title}`, "");
		for (const item of own) lines.push(`- [${item.title}](${url(item.href)}): ${docs.get(item.href)?.description ?? ""}`);
		if (own.length > 0) lines.push("");
		for (const item of group.items) {
			const doc = docs.get(item.href);
			if (doc && !isTranslated(doc)) untranslated.push(`- [${item.title}](${url(doc.englishHref)}): ${doc.description}`);
		}
	}

	if (untranslated.length > 0) lines.push("## Untranslated", "", ...untranslated, "");
	return lines.join("\n");
}

/** `llms-full.txt`: every page of the locale that has text of its own, in sidebar order. */
export function llmsFull(locale: Locale): string {
	const parts = [`# ${site.name} documentation`, ""];
	if (locale !== "en") parts.push(`Pages not translated yet are in ${site.url}/llms-full.txt.`, "");

	for (const doc of getAllDocs(locale)) {
		if (!isTranslated(doc)) continue;
		parts.push("---", "", `# ${doc.title}`, "", `Source: ${url(doc.href)}`, "", `> ${doc.description}`, "", mdxToMarkdown(doc.body), "");
	}
	return parts.join("\n");
}

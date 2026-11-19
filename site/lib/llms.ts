import { getAllDocs, getNavGroups } from "@/lib/content";
import { site } from "@/lib/site";
import { mdxToMarkdown } from "@/lib/text";
import { withTrailingSlash } from "@/lib/trailing-slash";

const SUMMARY =
	"Stuarty is a minimalist, open source video editor operated by AI agents (MCP, CLI, HTTP API) and by people in a browser UI.";

const url = (path: string) => `${site.url}${withTrailingSlash(path)}`;

/** `llms.txt` (SDD-010 section 9). */
export function llmsTxt(): string {
	const docs = new Map(getAllDocs().map((d) => [d.href, d]));
	const lines = [
		`# ${site.name}`,
		"",
		`> ${SUMMARY}`,
		"",
		`Full documentation in one file: ${site.url}/llms-full.txt`,
		"",
		`Every page is also available as Markdown: add ".md" to its URL (${site.url}/docs/introduction.md).`,
		"",
	];

	for (const group of getNavGroups()) {
		if (group.items.length === 0) continue;
		lines.push(`## ${group.label}`, "");
		for (const item of group.items) lines.push(`- [${item.title}](${url(item.href)}): ${docs.get(item.href)?.description ?? ""}`);
		lines.push("");
	}

	return lines.join("\n");
}

/** `llms-full.txt`: every page, in sidebar order. */
export function llmsFull(): string {
	const parts = [`# ${site.name} documentation`, ""];

	for (const doc of getAllDocs()) {
		parts.push("---", "", `# ${doc.title}`, "", `Source: ${url(doc.href)}`, "", `> ${doc.description}`, "", mdxToMarkdown(doc.body), "");
	}
	return parts.join("\n");
}

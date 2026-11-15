import fs from "node:fs";
import path from "node:path";
import type { LocalizedText } from "../lib/locale.ts";

/**
 * Sidebar navigation (SDD-010 section 3). Each category maps to a folder under
 * content/docs/<locale>/ (generated sections live in content/generated/) and lists
 * its pages in display order. A page "<dir>/<slug>.mdx" is served at
 * /docs/<prefix>/<slug> (prefix defaults to the folder name; the "Getting started"
 * pages live at the root of /docs). Page titles are the front matter of each
 * locale file; only the category titles are localized here.
 */
export type NavCategory = {
	dir: string;
	prefix?: string;
	title: LocalizedText;
	pages: string[];
};

/** The generated command pages (scripts/generate-reference.ts), in alphabetical order. */
function commandPages(): string[] {
	const dir = path.join(process.cwd(), "content", "generated", "reference", "commands");
	return fs.existsSync(dir)
		? fs
				.readdirSync(dir)
				.filter((f) => f.endsWith(".mdx"))
				.map((f) => f.slice(0, -".mdx".length))
				.sort()
		: [];
}

export const navigation: NavCategory[] = [
	{
		dir: "getting-started",
		prefix: "",
		title: { en: "Getting started", "pt-br": "Primeiros passos" },
		pages: ["introduction", "installation", "quick-start", "workspace", "projects"],
	},
	{
		dir: "concepts",
		title: { en: "Concepts", "pt-br": "Conceitos" },
		pages: ["how-it-works", "time", "commands-and-revisions", "tracks-clips-items"],
	},
	{
		dir: "guide",
		title: { en: "Editing guide", "pt-br": "Guia de edição" },
		pages: ["media-library", "timeline", "editing-tools", "text-and-captions", "undo-and-history", "exporting"],
	},
	{
		dir: "agents",
		title: { en: "Agents", "pt-br": "Agentes" },
		pages: ["overview", "connect", "mcp", "cli", "verify", "batches"],
	},
	{
		dir: "reference",
		title: { en: "Reference", "pt-br": "Referência" },
		pages: ["commands", "cli", "http-api", "timeline-schema", "error-codes", "configuration", "glossary"],
	},
	{
		dir: "reference/commands",
		title: { en: "Commands", "pt-br": "Comandos" },
		pages: commandPages(),
	},
];

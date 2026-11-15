import fs from "node:fs";
import path from "node:path";

/**
 * Sidebar navigation. Each category maps to a folder in content/docs and
 * lists its pages in display order. A page "<dir>/<slug>.mdx" is served at
 * /docs/<prefix>/<slug> (prefix defaults to the folder name; the
 * "Getting started" pages live at the root of /docs).
 */
export type NavCategory = {
	title: string;
	dir: string;
	prefix?: string;
	pages: string[];
};

/** The generated command pages (scripts/generate-reference.ts), in alphabetical order. */
function commandPages(): string[] {
	const dir = path.join(process.cwd(), "content", "docs", "reference", "commands");
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
		title: "Getting started",
		dir: "getting-started",
		prefix: "",
		pages: ["introduction", "quick-start"],
	},
	{
		title: "Agents",
		dir: "agents",
		pages: ["overview", "connect"],
	},
	{
		title: "Reference",
		dir: "reference",
		pages: ["commands", "cli", "timeline-schema", "error-codes", "configuration"],
	},
	{
		title: "Commands",
		dir: "reference/commands",
		pages: commandPages(),
	},
];

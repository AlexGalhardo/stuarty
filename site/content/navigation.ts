import fs from "node:fs";
import path from "node:path";

/**
 * Sidebar navigation (SDD-010 section 3). Each category maps to a folder under
 * content/docs/en/ (generated sections live in content/generated/) and lists
 * its pages in display order. A page "<dir>/<slug>.mdx" is served at
 * /docs/<prefix>/<slug> (prefix defaults to the folder name; the "Getting started"
 * pages live at the root of /docs). Page titles are the front matter of each file.
 */
export type NavCategory = {
	dir: string;
	prefix?: string;
	title: string;
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
		title: "Getting started",
		pages: ["introduction", "installation", "quick-start", "workspace", "projects"],
	},
	{
		dir: "concepts",
		title: "Concepts",
		pages: ["how-it-works", "time", "commands-and-revisions", "tracks-clips-items", "render", "one-renderer", "local-and-network-mode"],
	},
	{
		dir: "guide",
		title: "Editing guide",
		pages: [
			"media-library",
			"timeline",
			"preview",
			"editing-tools",
			"clip-properties",
			"undo-and-history",
			"text-and-captions",
			"shapes",
			"audio",
			"effects-and-color",
			"transitions",
			"keyframes",
			"transcription",
			"silence-removal",
			"safe-area",
			"brand-kit",
			"templates",
			"template-vertical-cut",
			"template-product-teaser",
			"template-quote-card",
			"exporting",
			"jobs",
			"encoders-and-presets",
			"keyboard-shortcuts",
			"accessibility",
			"settings",
		],
	},
	{
		dir: "guide",
		title: "Tutorials",
		pages: ["tutorial-vertical-short", "tutorial-agent-edit", "desktop"],
	},
	{
		dir: "agents",
		title: "Agents",
		pages: ["overview", "connect", "mcp", "cli", "verify", "batches", "skill", "safety", "recipes"],
	},
	{
		dir: "reference",
		title: "Reference",
		pages: ["commands", "cli", "http-api", "timeline-schema", "error-codes", "configuration", "events", "glossary"],
	},
	{
		dir: "self-hosting",
		title: "Self-hosting",
		pages: [
			"binary",
			"docker",
			"network-mode",
			"api-keys",
			"caddy",
			"backups-and-upgrades",
			"ffmpeg-and-licences",
			"verify-a-release",
			"observability",
			"privacy",
		],
	},
	{
		dir: "troubleshooting",
		title: "Troubleshooting",
		pages: ["common-problems", "ffmpeg-missing", "no-encoder", "workspace-locked", "import-fails", "slow-export", "diagnostic-bundle"],
	},
	{
		dir: "contributing",
		title: "Contributing",
		pages: ["guide", "setup", "repository-layout", "commits-and-prs", "writing-docs", "renderer", "releases"],
	},
	{
		dir: "reference/commands",
		title: "Commands",
		pages: commandPages(),
	},
];

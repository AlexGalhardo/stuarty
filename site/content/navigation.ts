import fs from "node:fs";
import path from "node:path";

/**
 * Sidebar navigation (SDD-010 section 3). Each category maps to a folder under
 * content/docs/en/ (generated sections live in content/generated/) and lists
 * its pages in display order. A page "<dir>/<slug>.mdx" is served at
 * /docs/<prefix>/<slug> (prefix defaults to the folder name; the "Getting started"
 * pages live at the root of /docs). Page titles are the front matter of each file.
 *
 * The order is the order of the work (set up, learn by doing, concepts, edit, create, deliver, then agents,
 * operations and reference), as in the docs of other editors; see the research note of TASK-0268. Several
 * categories may share a folder (`guide`): a category is a sidebar title plus a list of pages, so the URLs
 * of the pages do not depend on how they are grouped.
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
		dir: "guide",
		title: "Tutorials",
		pages: ["tutorial-vertical-short", "tutorial-agent-edit"],
	},
	{
		dir: "concepts",
		title: "Concepts",
		pages: ["how-it-works", "time", "commands-and-revisions", "tracks-clips-items", "render", "one-renderer", "local-and-network-mode"],
	},
	{
		dir: "guide",
		title: "Core editing",
		pages: ["media-library", "timeline", "preview", "editing-tools", "clip-properties", "undo-and-history"],
	},
	{
		dir: "guide",
		title: "Creative tools",
		pages: [
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
		],
	},
	{
		dir: "guide",
		title: "Templates",
		pages: ["templates", "template-vertical-cut", "template-product-teaser", "template-quote-card"],
	},
	{
		dir: "guide",
		title: "Output",
		pages: ["exporting", "jobs", "encoders-and-presets"],
	},
	{
		dir: "guide",
		title: "The editor",
		pages: ["keyboard-shortcuts", "settings", "accessibility", "desktop"],
	},
	{
		dir: "agents",
		title: "Agents",
		pages: ["overview", "connect", "mcp", "cli", "verify", "batches", "skill", "safety", "recipes"],
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
		dir: "reference",
		title: "Reference",
		pages: ["commands", "cli", "http-api", "timeline-schema", "error-codes", "configuration", "events", "glossary"],
	},
	{
		dir: "reference/commands",
		title: "Commands",
		pages: commandPages(),
	},
	{
		dir: "glossary",
		title: "Glossary",
		pages: [
			"how-to-use",
			"shooting-and-footage",
			"editing-and-timeline",
			"cuts-and-transitions",
			"audio",
			"color",
			"text-and-captions",
			"effects-and-compositing",
			"formats-codecs-containers",
			"delivery-and-platforms",
			"industry-slang",
			"a-to-z",
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
];

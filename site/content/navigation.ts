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
/**
 * The sidebar a page belongs to: the documentation, the API reference or the MCP reference. A page shows only the
 * groups of its own section, and its previous and next links stay in it.
 */
export type NavSection = "docs" | "api" | "mcp";

export type NavCategory = {
	dir: string;
	prefix?: string;
	/** The sidebar group. */
	title: string;
	/** The name above the page title and in search results, when the group title alone is not enough. */
	label?: string;
	section?: NavSection;
	pages: string[];
};

type ReferenceNav = { api?: { tag: string; pages: string[] }[]; mcp?: { tools: string[]; operations: string[] } };

/** The order of the API reference, written by scripts/generate-reference.ts (generated/reference-nav.json). */
function referenceNav(): ReferenceNav {
	const file = path.join(process.cwd(), "generated", "reference-nav.json");
	return fs.existsSync(file) ? (JSON.parse(fs.readFileSync(file, "utf8")) as ReferenceNav) : {};
}

/** The groups of the MCP reference: the server, the tools, and the operations of the edit tool. */
function mcpCategories(): NavCategory[] {
	const nav = referenceNav().mcp;
	return [
		{ dir: "mcp", prefix: "mcp", title: "MCP server", section: "mcp", pages: ["overview", "connect", "tools"] },
		{ dir: "mcp/tools", prefix: "mcp/tools", title: "Tools", label: "MCP tools", section: "mcp", pages: nav?.tools ?? [] },
		{
			dir: "mcp/tools/edit",
			prefix: "mcp/tools/edit",
			title: "Operations of edit",
			label: "MCP: edit operations",
			section: "mcp",
			pages: nav?.operations ?? [],
		},
	];
}

/** The groups of the API reference: its introduction, then one group per resource with a page per operation. */
function apiCategories(): NavCategory[] {
	const nav = referenceNav();
	return [
		{ dir: "api", prefix: "api", title: "API reference", section: "api", pages: ["overview", "endpoints", "schemas"] },
		...(nav.api ?? []).map(
			(g): NavCategory => ({ dir: "api", prefix: "api", title: g.tag, label: `API: ${g.tag}`, section: "api", pages: g.pages }),
		),
	];
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
		pages: ["keyboard-shortcuts", "keyboard-shortcuts-list", "coming-from-capcut", "settings", "accessibility", "desktop"],
	},
	{
		dir: "agents",
		title: "Agents",
		pages: ["overview", "connect", "mcp", "cli", "verify", "batches", "skill", "safety", "recipes", "drive-the-editor"],
	},
	{
		dir: "agents",
		title: "Agents by domain",
		pages: [
			"domain-project",
			"domain-video",
			"domain-image",
			"domain-writing",
			"domain-asset",
			"domain-publish",
			"domain-provider",
			"domain-memory",
			"domain-workspace",
		],
	},
	{
		dir: "prompts",
		title: "Prompts",
		pages: ["overview", "claude-code", "codex", "opencode", "cursor", "gemini-cli", "other-clients"],
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
			"desktop-updates",
		],
	},
	{
		dir: "reference",
		title: "Reference",
		pages: ["cli", "timeline-schema", "error-codes", "configuration", "events", "glossary"],
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
	...apiCategories(),
	...mcpCategories(),
];

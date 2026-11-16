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
		pages: ["how-it-works", "time", "commands-and-revisions", "tracks-clips-items", "render", "one-renderer", "local-and-network-mode"],
	},
	{
		dir: "guide",
		title: { en: "Editing guide", "pt-br": "Guia de edição" },
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
		title: { en: "Tutorials", "pt-br": "Tutoriais" },
		pages: ["tutorial-vertical-short", "tutorial-agent-edit", "desktop"],
	},
	{
		dir: "agents",
		title: { en: "Agents", "pt-br": "Agentes" },
		pages: ["overview", "connect", "mcp", "cli", "verify", "batches", "skill", "safety", "recipes"],
	},
	{
		dir: "reference",
		title: { en: "Reference", "pt-br": "Referência" },
		pages: ["commands", "cli", "http-api", "timeline-schema", "error-codes", "configuration", "events", "glossary"],
	},
	{
		dir: "self-hosting",
		title: { en: "Self-hosting", "pt-br": "Hospedagem própria" },
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
		title: { en: "Troubleshooting", "pt-br": "Solução de problemas" },
		pages: ["common-problems", "ffmpeg-missing", "no-encoder", "workspace-locked", "import-fails", "slow-export", "diagnostic-bundle"],
	},
	{
		dir: "contributing",
		title: { en: "Contributing", "pt-br": "Contribuindo" },
		pages: ["guide", "setup", "repository-layout", "commits-and-prs", "writing-docs", "translating", "releases"],
	},
	{
		dir: "reference/commands",
		title: { en: "Commands", "pt-br": "Comandos" },
		pages: commandPages(),
	},
];

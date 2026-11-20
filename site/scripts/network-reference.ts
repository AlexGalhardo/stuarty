// The limits of the social networks (epic 40, SDD-010 section 15.3). Read from generated/networks.json, which
// `go run ./internal/tools/clidoc -networks` writes from the data the product validates a post against, so the
// numbers on the site are the numbers PublishValidate enforces, each with the official page and the date it was
// read. Node strips the types.
import fs from "node:fs";
import path from "node:path";
import { esc, page, table } from "./reference-md.ts";

export type Preset = {
	id: string;
	label: string;
	codecs: string[];
	min_duration_ms?: number;
	max_duration_ms?: number;
	min_fps?: number;
	max_fps?: number;
	min_width?: number;
	min_height?: number;
	max_width?: number;
	max_height?: number;
	max_bytes?: number;
	target_width: number;
	target_height: number;
	sources: { url: string; read: string; says: string }[];
};

export type NetworkDoc = {
	network: string;
	name: string;
	publishes: string[];
	limits: {
		title_max: number;
		description_max_bytes: number;
		tags_max_chars: number;
		file_max_bytes: number;
		thumbnail_max_bytes: number;
		formats: string[] | null;
		thumbnail_formats: string[] | null;
		visibilities: string[];
		needs_kids_declaration: boolean;
		video_codecs?: string[];
		audio_codecs?: string[];
		caption_max?: number;
		hashtags_max?: number;
		daily_posts?: number;
		source: string;
		read: string;
	};
	draft?: boolean;
	price?: { currency: string; post_micros: number; link_micros: number; source: string; read: string };
	hosts: string[];
	scopes: string[];
	guide: string;
	guide_read: string;
	ai_disclosure: boolean;
	preset?: Preset;
};

const MB = 1_000_000;
const GB = 1_000_000_000;

export function bytes(n: number): string {
	if (n >= GB) return `${+(n / GB).toFixed(1)} GB`;
	if (n >= MB) return `${+(n / MB).toFixed(1)} MB`;
	return `${n} bytes`;
}

export function duration(ms: number): string {
	if (ms % 60_000 === 0) return `${ms / 60_000} min`;
	return `${+(ms / 1000).toFixed(1)} s`;
}

/** An amount in millionths of a currency, as a person writes it. */
export const money = (micros: number, currency: string): string => `${+(micros / 1_000_000).toFixed(3)} ${currency}`;

const list = (items: string[] | null | undefined): string => (items && items.length > 0 ? items.join(", ") : "none");

/** The rows of one network: what the post may hold, then what the video may be. A limit that is not stated is left out. */
export function limitRows(n: NetworkDoc): string[][] {
	const l = n.limits;
	const p = n.preset;
	const rows: [string, string][] = [];
	if (l.title_max) rows.push(["Title", `at most ${l.title_max} characters`]);
	if (l.caption_max) rows.push(["Caption", `at most ${l.caption_max} characters`]);
	if (l.hashtags_max) rows.push(["Hashtags", `at most ${l.hashtags_max}`]);
	if (l.description_max_bytes) rows.push(["Description", `at most ${l.description_max_bytes} bytes`]);
	if (l.tags_max_chars) rows.push(["Tags", `at most ${l.tags_max_chars} characters in all`]);
	if (l.file_max_bytes) rows.push(["Video file", `at most ${bytes(l.file_max_bytes)}`]);
	rows.push(["Video formats", list(l.formats)]);
	if (l.video_codecs?.length) rows.push(["Video codecs", list(l.video_codecs)]);
	if (l.audio_codecs?.length) rows.push(["Audio codecs", list(l.audio_codecs)]);
	if (p?.min_duration_ms) rows.push(["Shortest video", duration(p.min_duration_ms)]);
	if (p?.max_duration_ms) rows.push(["Longest video", duration(p.max_duration_ms)]);
	if (p?.min_fps || p?.max_fps) rows.push(["Frame rate", `${p.min_fps || "any"} to ${p.max_fps || "any"} fps`]);
	if (p?.min_width || p?.min_height) rows.push(["Smallest picture", `${p.min_width} x ${p.min_height} pixels`]);
	if (p?.max_width) rows.push(["Widest picture", `${p.max_width} pixels`]);
	if (p?.target_width) rows.push(["Export size of the preset", `${p.target_width} x ${p.target_height}`]);
	if (l.thumbnail_max_bytes) rows.push(["Thumbnail", `at most ${bytes(l.thumbnail_max_bytes)}, ${list(l.thumbnail_formats)}`]);
	else rows.push(["Thumbnail", "not sent"]);
	rows.push(["Visibility", list(l.visibilities)]);
	if (l.daily_posts) rows.push(["Posts through the API", `${l.daily_posts} in a moving 24 hours`]);
	return rows.map(([k, v]) => [k, esc(v)]);
}

/** The "Limits" section of a network: the table, then where each number was read and when. */
export function limitsSection(n: NetworkDoc): string {
	const sources = [`- [${n.name}: the page for the post](${n.limits.source}), read on ${n.limits.read}.`];
	for (const s of n.preset?.sources ?? []) sources.push(`- [The page for the video](${s.url}), read on ${s.read}: ${esc(s.says)}.`);
	const price = n.price
		? `### The price of a post

${n.name} charges for use of its API: ${money(n.price.post_micros, n.price.currency)} for a post and ${money(n.price.link_micros, n.price.currency)} for a post with a link, on ${n.price.read} ([source](${n.price.source})). The approval of a post shows the price Stuarty has; you can correct it on the Connections page when ${n.name} changes it.`
		: "";
	return [
		`These are the limits Stuarty checks a post to ${n.name} against before anything is sent (\`publish:validate\`). A post that breaks one is refused with the limit named, and nothing leaves your machine.`,
		table(["Limit", "Value"], limitRows(n)),
		"### Where the numbers come from",
		sources.join("\n"),
		price,
		n.draft
			? `${n.name} takes the file as a draft in your own app: it is never public until you finish it there.`
			: `${n.name} publishes only after you approve the post.`,
	]
		.filter((part) => part !== "")
		.join("\n\n");
}

export function networksOverview(networks: NetworkDoc[]): string {
	const rows = networks.map((n) => [
		`[${n.name}](/docs/publish/${n.network})`,
		esc(list(n.limits.formats)),
		esc(n.limits.file_max_bytes ? bytes(n.limits.file_max_bytes) : "not stated"),
		esc(n.preset?.max_duration_ms ? duration(n.preset.max_duration_ms) : "not stated"),
		esc(n.limits.read),
	]);
	return page(
		"Limits of the networks",
		"What each social network takes in a post, each with the official page and the date it was read.",
		[
			"The product validates a post against these limits before it asks for an approval. The numbers are written once in the program and this page is generated from them.",
			table(["Network", "Formats", "Largest file", "Longest video", "Read on"], rows),
			"Each network page has the full table.",
		].join("\n\n"),
	);
}

/** One page per network. `setup` is the hand-written steps of the developer app, put before the limits. */
export function networkPage(n: NetworkDoc, setup: string | undefined): string {
	const parts = [
		`Stuarty publishes to ${n.name} through a developer app that you create and own (ADR-0035): no Stuarty account stands between you and ${n.name}, and the secret stays in your vault.`,
	];
	if (setup) parts.push("## Set up the developer app", setup.trim());
	parts.push(
		"## Permissions and hosts",
		`The sign-in asks for these permissions and no others:\n\n${n.scopes.map((s) => `- \`${s}\``).join("\n")}`,
		`The only hosts Stuarty contacts for ${n.name}: ${n.hosts.map((h) => `\`${h}\``).join(", ")}. They are contacted only after you create the connection.`,
		`The official guide is [here](${n.guide}) (read on ${n.guide_read}).`,
		"## Limits",
		limitsSection(n),
	);
	return page(`Publish to ${n.name}`, `Create your ${n.name} developer app, connect it, and publish within the limits of ${n.name}.`, parts.join("\n\n"));
}

export function readNetworks(siteDir: string): NetworkDoc[] {
	return JSON.parse(fs.readFileSync(path.join(siteDir, "generated", "networks.json"), "utf8")) as NetworkDoc[];
}

export function readSetup(siteDir: string, network: string): string | undefined {
	const file = path.join(siteDir, "content", "network-setup", `${network}.md`);
	return fs.existsSync(file) ? fs.readFileSync(file, "utf8") : undefined;
}

// Generates the reference pages under content/generated/reference/ from the sources that cannot drift
// (SDD-010 section 8): the command registry (core/schema/commands.json), the error registry
// (core/schema/errors.json and SDD-000 section 8), the timeline schema, the CLI declarations of the
// binary (go run ./internal/tools/clidoc -> generated/cli.json) and the configuration table of SDD-000
// section 10. The shortcuts page (content/generated/guide/) is read from the editor keymap (shortcuts-reference.ts). Run from site/: `bun run reference`. `bun run reference:check` fails on any difference.
// The API reference (content/generated/api/) comes from site/generated/openapi.json, which `UPDATE_OPENAPI=1 go test
// ./web/contract/ -run OpenAPI` writes from the running API. The MCP reference (content/generated/mcp/) comes from
// web/app/mcp/definitions/ and from the command registry. Both write generated/reference-nav.json, the order of
// their sidebar groups. Node strips the types.
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { apiPages, type OpenApi } from "./api-reference.ts";
import { topicPages } from "./glossary-pages.ts";
import { mcpPages } from "./mcp-reference.ts";
import { networkPage, networksOverview, readNetworks, readSetup } from "./network-reference.ts";
import { readKeymap, shortcutsPage } from "./shortcuts-reference.ts";
import { code, esc, page, sentence, table } from "./reference-md.ts";

export { esc };

const siteDir = path.join(import.meta.dirname, "..");
const root = path.join(siteDir, "..");
const outDir = path.join(siteDir, "content", "generated", "reference");
const agentsDir = path.join(siteDir, "content", "generated", "agents");
const glossaryDir = path.join(siteDir, "content", "generated", "glossary");
const apiDir = path.join(siteDir, "content", "generated", "api");
const mcpDir = path.join(siteDir, "content", "generated", "mcp");
const guideDir = path.join(siteDir, "content", "generated", "guide");
const publishDir = path.join(siteDir, "content", "generated", "publish");
const startDir = path.join(siteDir, "content", "generated", "getting-started");

type Json = Record<string, unknown>;

const readJson = <T = Json>(rel: string): T => JSON.parse(fs.readFileSync(path.join(root, rel), "utf8")) as T;

type Command = {
	name: string;
	summary: string;
	description: string;
	errors: string[];
	example: unknown;
	internal: boolean;
	scope: string;
	args_schema: Schema;
};

type Schema = {
	type?: string | string[];
	description?: string;
	properties?: Record<string, Schema>;
	required?: string[];
	enum?: unknown[];
	items?: Schema;
	minimum?: number;
	maximum?: number;
	minItems?: number;
	maxItems?: number;
	pattern?: string;
	default?: unknown;
	$ref?: string;
	oneOf?: Schema[];
	anyOf?: Schema[];
	"x-status"?: string;
};

function typeOf(s: Schema): string {
	if (s.$ref) return code(s.$ref.replace("#/$defs/", ""));
	if (s.enum) return s.enum.map((v) => code(String(v))).join(", ");
	if (s.oneOf || s.anyOf) return (s.oneOf ?? s.anyOf ?? []).map(typeOf).join(" or ");
	const t = Array.isArray(s.type) ? s.type.join(" or ") : (s.type ?? "any");
	return t === "array" && s.items ? `array of ${typeOf(s.items)}` : code(t);
}

function constraints(s: Schema): string {
	const out: string[] = [];
	if (s.minimum !== undefined) out.push(`min ${s.minimum}`);
	if (s.maximum !== undefined) out.push(`max ${s.maximum}`);
	if (s.minItems !== undefined) out.push(`at least ${s.minItems} items`);
	if (s.maxItems !== undefined) out.push(`at most ${s.maxItems} items`);
	if (s.pattern) out.push(`pattern ${code(s.pattern)}`);
	if (s.default !== undefined) out.push(`default ${code(JSON.stringify(s.default))}`);
	if (s["x-status"]) out.push(s["x-status"]);
	return out.join("; ");
}

function propertyRows(s: Schema): string[][] {
	const required = new Set(s.required ?? []);
	return Object.entries(s.properties ?? {}).map(([name, p]) => [
		code(name),
		typeOf(p),
		required.has(name) ? "yes" : "no",
		esc([p.description ?? "", constraints(p)].filter(Boolean).join(" ")),
	]);
}

type ErrorEntry = { code: string; http: number; retryable: boolean };

function sddTable(rel: string, heading: RegExp): string[][] {
	const text = fs.readFileSync(path.join(root, rel), "utf8").replace(/\r\n/g, "\n");
	const start = text.search(heading);
	if (start < 0) throw new Error(`${rel}: no section matching ${heading}`);
	const rest = text.slice(start).split("\n").slice(1);
	const rows: string[][] = [];
	let seen = false;
	for (const line of rest) {
		if (line.startsWith("|")) {
			seen = true;
			const cells = line.replace(/^\||\|$/g, "").split(/(?<!\\)\|/).map((c) => c.trim());
			if (!/^-+$/.test(cells[0] ?? "")) rows.push(cells);
		} else if (seen) break;
	}
	return rows.slice(1);
}

function errorCodes(): string {
	const registry = readJson<ErrorEntry[]>("core/schema/errors.json");
	const meaning = new Map<string, string>();
	for (const [codes, text] of sddTable("docs/sdds/000-shared-definitions.md", /^## 8\. /m)) {
		for (const c of codes.match(/E_[A-Z_]+/g) ?? []) meaning.set(c, text ?? "");
	}
	const rows = [...registry]
		.sort((a, b) => a.code.localeCompare(b.code))
		.map((e) => [code(e.code), String(e.http), e.retryable ? "yes" : "no", esc(meaning.get(e.code) ?? "")]);

	const known = new Set(registry.map((e) => e.code));
	const status = serviceStatuses();
	const serviceRows: string[][] = [];
	for (const [codes, text, retry] of sddTable("docs/sdds/000-shared-definitions.md", /^## 8\. /m)) {
		for (const c of codes.match(/E_[A-Z_]+/g) ?? []) {
			if (known.has(c)) continue;
			serviceRows.push([code(c), String(status.get(c) ?? 500), retry === "yes" ? "yes" : "no", esc(text ?? "")]);
		}
	}
	serviceRows.sort((a, b) => a[0].localeCompare(b[0]));

	const anchors = [...rows, ...serviceRows].map((r) => `<a id="${r[0].replace(/`/g, "").toLowerCase()}"></a>`).join("");
	return page(
		"Error codes",
		"Every error code, its HTTP status and whether retrying can help.",
		[
			"An error carries `code`, `message`, `hint` and `retryable`. The CLI exits with `2` for a retryable code and `1` otherwise.",
			anchors,
			"## Errors of the editing commands",
			"Raised by the command core when an operation of a batch is refused.",
			table(["Code", "HTTP", "Retryable", "Meaning"], rows),
			"## Errors of the service",
			"Raised around the commands: sign-in, imports, exports, jobs and limits. They appear in the results of the calls that raise them. A code that is not in the status table of the service answers with HTTP 500, which is the status of the failures of a job.",
			table(["Code", "HTTP", "Retryable", "Meaning"], serviceRows),
		].join("\n\n"),
	);
}

const STATUS_NUMBERS: Record<string, number> = {
	StatusBadRequest: 400,
	StatusUnauthorized: 401,
	StatusForbidden: 403,
	StatusNotFound: 404,
	StatusMethodNotAllowed: 405,
	StatusConflict: 409,
	StatusRequestEntityTooLarge: 413,
	StatusUnprocessableEntity: 422,
	StatusTooManyRequests: 429,
	StatusClientClosed: 499,
	StatusInternalServerError: 500,
	StatusBadGateway: 502,
	StatusServiceUnavailable: 503,
};

/** The HTTP status of each error code of service/error.go, read from the `HTTPStatus` switch of the service. */
export function serviceStatuses(): Map<string, number> {
	const src = fs.readFileSync(path.join(root, "service", "error.go"), "utf8").replace(/\r\n/g, "\n");
	const consts = new Map<string, string>();
	for (const m of src.matchAll(/(Code[A-Za-z]+)\s*=\s*"(E_[A-Z_]+)"/g)) consts.set(m[1], m[2]);
	const out = new Map<string, number>();
	for (const m of src.matchAll(/case ([^:]+?):\s*return (?:http\.)?(Status[A-Za-z]+)/gs)) {
		const num = STATUS_NUMBERS[m[2]];
		if (num === undefined) throw new Error(`service/error.go: status ${m[2]} is not in STATUS_NUMBERS`);
		for (const token of m[1].matchAll(/"(E_[A-Z_]+)"|(?<![A-Za-z.])(Code[A-Za-z]+)/g)) {
			const c = token[1] ?? consts.get(token[2]);
			if (c) out.set(c, num);
		}
	}
	return out;
}

type CliCommand = {
	name: string;
	summary: string;
	usage: string[];
	example?: string;
	flags?: { name: string; type: string; default?: string; usage: string; repeat?: boolean }[];
	globals: string[];
	pending?: string;
};

type CliDoc = {
	globals: { name: string; type: string; default?: string; usage: string }[];
	exit_codes: { code: number; meaning: string }[];
	commands: CliCommand[];
	process_commands: CliCommand[];
};

function cliReference(cli: CliDoc): string {
	const flagRows = (flags: NonNullable<CliDoc["commands"][number]["flags"]>) =>
		flags.map((f) => [code(`--${f.name}`), f.type + (f.repeat ? ", repeatable" : ""), f.default ? code(f.default) : "", esc(f.usage)]);
	const sections = [...cli.commands, ...cli.process_commands].map((c) => {
		const out = [`## ${code(c.name)}`, esc(sentence(c.summary)) + (c.pending ? ` Not available yet: arrives with ${c.pending}.` : "")];
		out.push("```text\n" + c.usage.join("\n") + "\n```");
		if (c.flags?.length) out.push(table(["Flag", "Type", "Default", "Meaning"], flagRows(c.flags)));
		if (c.globals.length) out.push(`Global flags: ${c.globals.map((g) => code(`--${g}`)).join(", ")}.`);
		if (c.example) out.push("Example:\n\n```bash\n" + c.example + "\n```");
		return out.join("\n\n");
	});
	return page(
		"CLI reference",
		"Every stuarty command, its flags and the exit codes.",
		[
			"The CLI is a thin adapter over the same service as the editor, the MCP server and the HTTP API. Add `--json` to any command for one JSON document on standard output.",
			"## Global flags",
			table(["Flag", "Type", "Default", "Meaning"], flagRows(cli.globals)),
			"## Exit codes",
			table(["Code", "Meaning"], cli.exit_codes.map((e) => [String(e.code), esc(e.meaning)])),
			...sections,
		].join("\n\n"),
	);
}

type Timeline = { $defs: Record<string, Schema>; properties: Record<string, Schema>; required?: string[]; title?: string };

function timelineSchema(): string {
	const t = readJson<Timeline>("core/schema/timeline.v1.schema.json");
	const sections = Object.entries(t.$defs)
		.sort(([a], [b]) => a.localeCompare(b))
		.map(([name, s]) =>
			[`## ${name}`, s.description ? esc(s.description) : "", propertyRows(s).length ? table(["Property", "Type", "Required", "Description"], propertyRows(s)) : ""]
				.filter(Boolean)
				.join("\n\n"),
		);
	return page(
		"Timeline schema",
		"The JSON Schema of the project document, version 1.",
		[
			"A project is one JSON document. Times are whole frames at the frame rate of the project. The root has these properties:",
			table(["Property", "Type", "Required", "Description"], propertyRows({ properties: t.properties, required: t.required })),
			...sections,
		].join("\n\n"),
	);
}

/** The variables SDD-000 section 10 names in prose after its table: the framework's own and those of deploy/. */
function proseVariables(): { framework: string[]; deploy: string[] } {
	const sdd = fs.readFileSync(path.join(root, "docs", "sdds", "000-shared-definitions.md"), "utf8");
	const para = sdd.match(/^The limits and their errors are specified.*$/m)?.[0] ?? "";
	const names = (text: string | undefined): string[] => [...(text ?? "").matchAll(/`([A-Z][A-Z_0-9]+)`/g)].map((m) => m[1] as string);
	return {
		framework: names(para.match(/used by the product \(([^)]*)\)/)?.[1]),
		deploy: [...names(para.match(/`deploy\/\.env` \(([^)]*)\)/)?.[1]), ...names(para.match(/scripts of `deploy\/` \(([^)]*)\)/)?.[1])],
	};
}

/** The page for people who used the product under its former name (SDD-000 section 13): the only page that says it. */
function migrationPage(): string {
	const pairs = sddTable("docs/sdds/000-shared-definitions.md", /^### 13\.1 /m).map(([was, now]) => [esc(was ?? ""), esc(now ?? "")]);
	const prose = fs.readFileSync(path.join(siteDir, "content", "fragments", "migration.md"), "utf8");
	return page(
		// The title and the description show on every page (the sidebar, the search, llms.txt): only the body says the name.
		"Migrating from the former name",
		"Stuarty had another name until v0.29.0: what is adopted, what is still read, and what to rename.",
		[prose.trim(), table(["Former variable", "Variable"], pairs)].join("\n\n"),
	);
}

function configuration(): string {
	const rows = sddTable("docs/sdds/000-shared-definitions.md", /^## 10\. /m).map(([name, def, meaning]) => [
		(name ?? "").replace(/\\/g, "\\\\").replace(/\|/g, "\\|"),
		esc(def ?? ""),
		esc(meaning ?? ""),
	]);
	return page(
		"Configuration variables",
		"Every environment variable of Stuarty, with its default.",
		[
			"All variables are optional.",
			table(["Variable", "Default", "Meaning"], rows),
			"## Variables of the server and of the server package",
			`The framework the server is built on reads these, with its own names: ${proseVariables()
				.framework.map((v) => code(v))
				.join(", ")}. \`APP_KEY\` is required in network mode (32 bytes or more, base64).`,
			`The scripts and the compose file of \`deploy/\` read these, never the program itself: ${proseVariables()
				.deploy.map((v) => code(v))
				.join(", ")}.`,
			"## Names before the rename",
			"The product has had another name. The variables of that name are still read for a while: see [Migrating from the former name](/docs/migration).",
		].join("\n\n"),
	);
}


type Term = { id: string; en: string; note: string };

function glossary(): string {
	const terms = JSON.parse(fs.readFileSync(path.join(siteDir, "content", "glossary.json"), "utf8")) as Term[];
	return page(
		"Product terms",
		"The product terms the documentation and the editor use.",
		[
			"Command, field and error code names are written exactly as the program prints them. This table is for prose and labels.",
			table(["Term", "Note"], terms.map((t) => [esc(t.en), esc(t.note)])),
		].join("\n\n"),
	);
}

/** Escapes the text of a Markdown line outside inline code for MDX (no pipe: the skill has no tables). */
function escProse(text: string): string {
	const map: Record<string, string> = { "\\": "\\\\", "{": "\\{", "}": "\\}", "<": "&lt;", ">": "&gt;" };
	return text
		.split(/(`[^`]*`)/)
		.map((part, i) => (i % 2 === 1 ? part : part.replace(/[\\{}<>]/g, (c) => map[c] as string)))
		.join("");
}

/**
 * The agent skill page (SDD-010 section 7.4, SDD-003 section 8.3): the body of web/app/agentdocs/out/SKILL.md
 * without its front matter, its generated comment and its H1 (the page title is the H1), so the text is
 * written once, by the Go generator, and never committed twice by hand.
 */
function skillPage(): string {
	const raw = fs.readFileSync(path.join(root, "web", "app", "agentdocs", "out", "SKILL.md"), "utf8").replace(/\r\n/g, "\n");
	const body = raw
		.replace(/^---\n[\s\S]*?\n---\n/, "")
		.replace(/^<!--.*-->\n/m, "")
		.replace(/^# .*\n/m, "");
	let fence = false;
	const lines = body.split("\n").map((line) => {
		if (/^\s*```/.test(line)) {
			fence = !fence;
			return line;
		}
		return fence ? line : escProse(line);
	});
	return page(
		"The agent skill",
		"The skill file that teaches an agent the edit loop, the rules, the commands and the errors, as shipped in the release archives.",
		[
			"This page is the skill file `agent/skills/stuarty/SKILL.md` of every release archive. Write it into an agent's skills folder with `stuarty agent:docs --write <dir>`, or print it with `stuarty agent:docs --print skill`.",
			lines.join("\n"),
		].join("\n\n"),
	);
}

/**
 * The guide of the agents, one page per domain (SDD-003 section 8.1, epic 13): the body of
 * web/app/agentdocs/out/guide/<domain>.md without its generated comment and its H1, as the skill page is.
 */
export function domainPages(): { file: string; text: string }[] {
	const dir = path.join(root, "web", "app", "agentdocs", "out", "guide");
	if (!fs.existsSync(dir)) return [];
	return fs
		.readdirSync(dir)
		.filter((f) => f.endsWith(".md"))
		.sort()
		.map((f) => {
			const domain = f.replace(/\.md$/, "");
			const raw = fs.readFileSync(path.join(dir, f), "utf8").replace(/\r\n/g, "\n");
			const body = raw.replace(/^<!--.*-->\n/m, "").replace(/^# .*\n/m, "");
			const lines = body.split("\n").map((line) => escProse(line));
			const intro = `This page is generated from the registry. Print it with \`stuarty agent:docs --print guide:${domain}\`; \`stuarty ${domain}\` lists the commands of the domain.`;
			return {
				file: `domain-${domain}.mdx`,
				text: page(
					`The ${domain} domain`,
					`How an agent reaches the ${domain} domain: its MCP tools, its CLI commands and its edit operations.`,
					[intro, lines.join("\n")].join("\n\n"),
				),
			};
		});
}

function write(file: string, text: string, expected: Set<string>, base = outDir) {
	const target = path.join(base, file);
	fs.mkdirSync(path.dirname(target), { recursive: true });
	fs.writeFileSync(target, text);
	expected.add(path.resolve(target));
}

async function main() {
	const cliJson = execFileSync("go", ["run", "./internal/tools/clidoc"], { cwd: root, encoding: "utf8", maxBuffer: 1 << 26 });
	fs.mkdirSync(path.join(siteDir, "generated"), { recursive: true });
	fs.writeFileSync(path.join(siteDir, "generated", "cli.json"), cliJson);

	const networksJson = execFileSync("go", ["run", "./internal/tools/clidoc", "-networks"], { cwd: root, encoding: "utf8", maxBuffer: 1 << 26 });
	fs.writeFileSync(path.join(siteDir, "generated", "networks.json"), networksJson);

	const written = new Set<string>();
	const commands = readJson<Command[]>("core/schema/commands.json")
		.filter((c) => !c.internal)
		.sort((a, b) => a.name.localeCompare(b.name));

	write("cli.mdx", cliReference(JSON.parse(cliJson) as CliDoc), written);
	write("timeline-schema.mdx", timelineSchema(), written);
	write("error-codes.mdx", errorCodes(), written);
	write("configuration.mdx", configuration(), written);
	write("glossary.mdx", glossary(), written);
	write("migration.mdx", migrationPage(), written, startDir);
	const networks = readNetworks(siteDir);
	write("network-limits.mdx", networksOverview(networks), written);
	for (const n of networks) write(`${n.network}.mdx`, networkPage(n, readSetup(siteDir, n.network)), written, publishDir);
	write("skill.mdx", skillPage(), written, agentsDir);
	for (const p of domainPages()) write(p.file, p.text, written, agentsDir);

	const api = apiPages(readJson<OpenApi>("site/generated/openapi.json"), root, new Map(commands.map((c) => [c.name, c.example])));
	for (const [file, text] of api.files) write(file, text, written, apiDir);
	const mcp = mcpPages(root, commands);
	for (const [file, text] of mcp.files) write(file, text, written, mcpDir);
	fs.writeFileSync(
		path.join(siteDir, "generated", "reference-nav.json"),
		JSON.stringify({ api: api.nav, mcp: { tools: mcp.tools, operations: mcp.operations } }, null, 2) + "\n",
	);
	const { keymap, labels } = await readKeymap(root);
	write("keyboard-shortcuts-list.mdx", shortcutsPage(keymap, labels), written, guideDir);
	for (const p of topicPages()) write(p.file, p.text, written, glossaryDir);

	// A command that left the registry leaves its page too.
	const walk = (dir: string): string[] =>
		fs.existsSync(dir)
			? fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]))
			: [];
	for (const dir of [outDir, agentsDir, glossaryDir, apiDir, mcpDir, guideDir, publishDir, startDir]) for (const file of walk(dir)) if (!written.has(path.resolve(file))) fs.rmSync(file);

	console.log(`reference: ${written.size} pages`);
}

if (import.meta.main) await main();

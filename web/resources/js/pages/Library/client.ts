import { fromResponse, networkError } from "../../editor/api/errors";
import { type Transport, TransportError } from "../../editor/api/transport";

// The client of the library operations (SDD-018 section 14): memory notes, skills and prompts. The page
// computes nothing: it sends what the person wrote and draws what the service answers, the lint, the
// rendering of a prompt and the review included.

export type LibraryKind = "memory" | "skill" | "prompt";

export interface LibraryEntry {
	kind: LibraryKind;
	name: string;
	description: string;
	scope?: string;
	origin?: string;
	reviewed: boolean;
	kept: boolean;
	actor?: string;
	tags?: string[];
	state: "ok" | "invalid";
	error?: string;
	lint_errors?: number;
	lint_warnings?: number;
	has_scripts?: boolean;
	updated_at: string;
	warning?: string;
}

export interface LibraryFinding {
	code: string;
	severity: "error" | "warning";
	path?: string;
	line?: number;
	message: string;
}

export interface LibraryListResult {
	kind: LibraryKind;
	entries: LibraryEntry[];
}

export interface MemoryGetResult {
	entry: LibraryEntry;
	body: string;
}

export interface MemoryWriteBody {
	name?: string;
	description: string;
	scope?: string;
	tags?: string[];
	body?: string;
	keep?: boolean;
}

export interface SkillFileView {
	path: string;
	bytes: number;
	script?: boolean;
	binary?: boolean;
	text?: string;
}

export interface SkillGetResult {
	entry: LibraryEntry;
	license?: string;
	compatibility?: string;
	allowed_tools?: string;
	source?: string;
	body: string;
	files?: SkillFileView[];
	findings?: LibraryFinding[];
}

export interface SkillWriteBody {
	description: string;
	license?: string;
	body: string;
	// Null keeps the files the skill has.
	files: null;
}

export interface SkillWriteResult {
	entry: LibraryEntry;
	findings?: LibraryFinding[];
}

export interface SkillImportBody {
	path: string;
	source?: string;
	replace?: boolean;
}

export interface SkillImportResult {
	entry: LibraryEntry;
	findings?: LibraryFinding[];
	files: number;
}

export interface SkillExportResult {
	names: string[];
	path?: string;
}

export interface PromptGetResult {
	entry: LibraryEntry;
	body: string;
	variables?: string[];
	defaults?: { [variable: string]: string };
}

export interface PromptWriteBody {
	description: string;
	tags?: string[];
	defaults?: { [variable: string]: string };
	body: string;
}

export interface PromptRenderResult {
	name: string;
	rendered: string;
}

export interface LibraryHit extends LibraryEntry {
	snippet: string;
}

export interface LibrarySearchResult {
	hits: LibraryHit[];
	total: number;
	truncated?: boolean;
}

export interface LibraryReindexResult {
	memory: number;
	skills: number;
	prompts: number;
	invalid: number;
	ignored?: string[];
}

const TIMEOUT_MS = 20_000;

const FOLDER: { [kind in LibraryKind]: string } = {
	memory: "memory",
	skill: "skills",
	prompt: "prompts",
};

export interface LibraryClient {
	list(kind: LibraryKind): Promise<LibraryListResult>;
	search(kind: LibraryKind, query: string): Promise<LibrarySearchResult>;
	getMemory(name: string): Promise<MemoryGetResult>;
	writeMemory(body: MemoryWriteBody): Promise<{ entry: LibraryEntry }>;
	removeMemory(name: string): Promise<unknown>;
	getSkill(name: string): Promise<SkillGetResult>;
	writeSkill(name: string, body: SkillWriteBody): Promise<SkillWriteResult>;
	importSkill(body: SkillImportBody): Promise<SkillImportResult>;
	reviewSkill(name: string): Promise<{ entry: LibraryEntry }>;
	exportSkills(names: readonly string[]): Promise<SkillExportResult>;
	getPrompt(name: string): Promise<PromptGetResult>;
	writePrompt(
		name: string,
		body: PromptWriteBody,
	): Promise<{ entry: LibraryEntry; variables?: string[] }>;
	renderPrompt(
		name: string,
		values: { [variable: string]: string },
	): Promise<PromptRenderResult>;
	reindex(): Promise<LibraryReindexResult>;
}

export function libraryClient(transport: Transport): LibraryClient {
	async function call<T>(
		method: "GET" | "POST" | "PUT" | "DELETE",
		path: string,
		body?: unknown,
	): Promise<T> {
		let response: Awaited<ReturnType<Transport["send"]>>;
		try {
			response = await transport.send({
				method,
				path,
				body,
				timeoutMs: TIMEOUT_MS,
			});
		} catch (error) {
			if (error instanceof TransportError) throw networkError(error.message);
			throw error;
		}
		if (response.status < 200 || response.status > 299) {
			throw fromResponse(response.status, response.body);
		}
		return response.body as T;
	}

	const one = (name: string): string => encodeURIComponent(name);

	return {
		// A person sees everything: the notes nobody kept and the skills nobody reviewed are listed
		// with their state (an agent is not offered them).
		list: (kind) =>
			call("GET", `/library/${FOLDER[kind]}?include_unreviewed=true`),
		search: (kind, query) =>
			call(
				"GET",
				`/library/search?query=${encodeURIComponent(query)}&kinds=${kind}&include_unreviewed=true`,
			),
		getMemory: (name) => call("GET", `/library/memory/${one(name)}`),
		writeMemory: (body) => call("POST", "/library/memory", body),
		removeMemory: (name) => call("DELETE", `/library/memory/${one(name)}`),
		getSkill: (name) => call("GET", `/library/skills/${one(name)}`),
		writeSkill: (name, body) =>
			call("PUT", `/library/skills/${one(name)}`, body),
		importSkill: (body) => call("POST", "/library/skills/import", body),
		reviewSkill: (name) =>
			call("POST", `/library/skills/${one(name)}/review`, {}),
		exportSkills: (names) =>
			call("POST", "/library/skills/export", { names: [...names] }),
		getPrompt: (name) => call("GET", `/library/prompts/${one(name)}`),
		writePrompt: (name, body) =>
			call("PUT", `/library/prompts/${one(name)}`, body),
		renderPrompt: (name, values) =>
			call("POST", `/library/prompts/${one(name)}/render`, { values }),
		reindex: () => call("POST", "/library/reindex", {}),
	};
}

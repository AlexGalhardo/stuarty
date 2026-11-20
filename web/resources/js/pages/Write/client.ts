import { fromResponse, networkError } from "../../editor/api/errors";
import { type Transport, TransportError } from "../../editor/api/transport";

// The client of the writing operations (SDD-019 section 7): the scripts, the copy and the captions of a
// production. The page computes nothing: the counts, the problems, the estimates and the differences are the
// core's, and this file only sends what the person wrote and hands back what the service answered.

export type TextKind = "script" | "copy" | "caption" | "post";

// The check of a blog post (SDD-019 section 13): the issues, the images the post uses and the body the service read
// after the front matter. The page draws them; the rules are the core's.
export interface PostIssue {
	path: string;
	code: string;
	message: string;
}

export interface PostImage {
	asset_id: string;
	role: "cover" | "body";
	name: string;
	found: boolean;
	bytes?: number;
	content_type?: string;
}

export interface PostCheckResult {
	project_id: string;
	item_id: string;
	slug: string;
	ok: boolean;
	issues: PostIssue[];
	images: PostImage[];
	words: number;
	markdown: string;
}

export interface Money {
	micros: number;
	currency: string;
}

export interface TextItem {
	id: string;
	project_id: string;
	project_name?: string;
	title: string;
	text_kind: TextKind;
	state: "active" | "trashed";
	words: number;
	revision: number;
	updated_by?: string;
	updated_at: string;
}

export interface TextRevision {
	revision: number;
	actor: string;
	source: "created" | "write" | "generated" | "revert";
	reverted_to?: number;
	provider?: string;
	model?: string;
	cost?: Money;
	job_id?: string;
	created_at: string;
}

export interface TextListResult {
	texts: TextItem[];
	next_cursor?: string;
}

export interface TextGetResult {
	project_id: string;
	item: TextItem;
	revision: TextRevision;
	body: string;
}

export interface TextWriteResult {
	project_id: string;
	item: TextItem;
	revision: TextRevision;
	created: boolean;
	changed: boolean;
}

export interface Hunk {
	op: "equal" | "removed" | "added";
	text: string;
}

export interface TextRevisionsResult {
	project_id: string;
	item_id: string;
	revisions: TextRevision[];
	next_before?: number;
	diff?: { hunks: Hunk[]; coarse: boolean };
	diff_from?: number;
	diff_to?: number;
}

export interface CaptionProblem {
	code: string;
	message: string;
	limit: number;
	actual: number;
	unit?: string;
}

export interface CaptionVariantReport {
	network: string;
	// The name of the network in the table of limits; absent for the common variant.
	name?: string;
	unit?: string;
	max_length: number;
	counts: { length: number; hashtags: number; mentions: number; links: number };
	links_clickable: boolean;
	problems: CaptionProblem[];
	ok: boolean;
	words: number;
}

export interface CaptionValidateResult {
	variants: CaptionVariantReport[];
	ok: boolean;
}

export interface ScriptEstimateResult {
	words_per_minute: number;
	words: number;
	on_screen_words: number;
	duration_ms: number;
	plain: boolean;
	scenes: {
		title: string;
		words: number;
		on_screen_words: number;
		duration_ms: number;
	}[];
}

export interface DraftRequest {
	connection_id: string;
	model: string;
	instruction: string;
	network?: string;
	max_tokens?: number;
	dry_run?: boolean;
}

// The part of a job the draft panel reads: the estimate that comes first, the state, and the text result.
export interface DraftJob {
	job_id: string;
	state: string;
	estimate?: Money;
	result?: {
		text?: {
			provider: string;
			model: string;
			item_id?: string;
			revision?: number;
			cost: Money;
		};
	};
	error?: { code: string; message: string; hint?: string };
	approval_id?: string;
}

const TIMEOUT_MS = 20_000;

export interface TextListFilter {
	project_id?: string;
	text_kind?: TextKind;
	state?: "active" | "trashed";
	cursor?: string;
}

export interface WritingClient {
	list(filter: TextListFilter): Promise<TextListResult>;
	get(
		projectId: string,
		itemId: string,
		revision?: number,
	): Promise<TextGetResult>;
	create(
		projectId: string,
		body: { title: string; text_kind: TextKind; text: string },
	): Promise<TextWriteResult>;
	write(
		projectId: string,
		itemId: string,
		body: { title?: string; text: string; base_revision?: number },
	): Promise<TextWriteResult>;
	revisions(
		projectId: string,
		itemId: string,
		range?: { from: number; to: number },
	): Promise<TextRevisionsResult>;
	revert(
		projectId: string,
		itemId: string,
		revision: number,
	): Promise<TextWriteResult>;
	draft(
		projectId: string,
		itemId: string,
		request: DraftRequest,
	): Promise<DraftJob>;
	job(jobId: string): Promise<DraftJob>;
	validateCaption(
		body: string,
		projectId: string,
	): Promise<CaptionValidateResult>;
	estimateScript(
		body: string,
		projectId: string,
		wordsPerMinute?: number,
	): Promise<ScriptEstimateResult>;
	checkPost(projectId: string, itemId: string): Promise<PostCheckResult>;
}

export function writingClient(transport: Transport): WritingClient {
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

	const one = (id: string): string => encodeURIComponent(id);
	const path = (projectId: string, itemId: string, suffix = ""): string =>
		`/projects/${one(projectId)}/texts/${one(itemId)}${suffix}`;

	return {
		list: (filter) => {
			const query = new URLSearchParams();
			for (const [key, value] of Object.entries(filter)) {
				if (value !== undefined && value !== "") query.set(key, value);
			}
			const text = query.toString();
			return call("GET", `/texts${text === "" ? "" : `?${text}`}`);
		},
		get: (projectId, itemId, revision) =>
			call(
				"GET",
				path(projectId, itemId) +
					(revision === undefined ? "" : `?revision=${revision}`),
			),
		create: (projectId, body) =>
			call("POST", `/projects/${one(projectId)}/texts`, body),
		write: (projectId, itemId, body) =>
			call("PUT", path(projectId, itemId), body),
		revisions: (projectId, itemId, range) =>
			call(
				"GET",
				path(projectId, itemId, "/revisions") +
					(range === undefined ? "" : `?from=${range.from}&to=${range.to}`),
			),
		revert: (projectId, itemId, revision) =>
			call("POST", path(projectId, itemId, "/revert"), { revision }),
		draft: (projectId, itemId, request) =>
			call("POST", path(projectId, itemId, "/draft"), request),
		job: (jobId) => call("GET", `/jobs/${one(jobId)}`),
		validateCaption: (body, projectId) =>
			call("POST", "/captions/validate", { body, project_id: projectId }),
		estimateScript: (body, projectId, wordsPerMinute) =>
			call("POST", "/scripts/estimate", {
				text: body,
				project_id: projectId,
				words_per_minute: wordsPerMinute,
			}),
		checkPost: (projectId, itemId) =>
			call("GET", path(projectId, itemId, "/post")),
	};
}

import { fromResponse, networkError } from "./errors";
import { type RetryDeps, withRetry } from "./retry";
import { type Transport, TransportError } from "./transport";
import type {
	AuditListResult,
	AuditQuery,
	CommandHelpResult,
	EditRequest,
	EditResult,
	FontListResult,
	HistoryListResult,
	HistoryQuery,
	Job,
	JobListResult,
	LintResult,
	MediaImportResult,
	MediaInspectQuery,
	MediaInspectResult,
	MediaListResult,
	MediaStateResult,
	PlanWindow,
	PlanWindowHead,
	ProjectGetResult,
	SnapPointsQuery,
	SnapPointsResult,
	SystemQuery,
	SystemResult,
} from "./types";

// Timeouts of SDD-012 section 4.3 item 4.
const READ_TIMEOUT_MS = 10_000;
const EDIT_TIMEOUT_MS = 30_000;

const defaultDeps: RetryDeps = {
	sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
	random: Math.random,
};

export interface ApiClient {
	listMedia(projectId: string): Promise<MediaListResult>;
	importMedia(projectId: string, paths: string[]): Promise<MediaImportResult>;
	getLint(projectId: string): Promise<LintResult>;
	startExport(projectId: string): Promise<Job>;
	getJob(jobId: string): Promise<Job>;
	cancelJob(jobId: string): Promise<Job>;
	listJobs(projectId: string): Promise<JobListResult>;
	getProject(projectId: string): Promise<ProjectGetResult>;
	edit(projectId: string, request: EditRequest): Promise<EditResult>;
	undo(projectId: string): Promise<EditResult>;
	redo(projectId: string): Promise<EditResult>;
	getPlan(
		projectId: string,
		from: number,
		to: number,
		revision: number,
	): Promise<PlanWindow>;
	getCommands(): Promise<CommandHelpResult>;
	getCommand(name: string): Promise<CommandHelpResult>;
	getHistory(
		projectId: string,
		query?: HistoryQuery,
	): Promise<HistoryListResult>;
	inspectMedia(
		projectId: string,
		assetId: string,
		query?: MediaInspectQuery,
	): Promise<MediaInspectResult>;
	trashMedia(projectId: string, assetId: string): Promise<MediaStateResult>;
	restoreMedia(projectId: string, assetId: string): Promise<MediaStateResult>;
	getPlanHead(
		projectId: string,
		from: number,
		to: number,
		revision: number,
	): Promise<PlanWindowHead>;
	getSnapPoints(
		projectId: string,
		query?: SnapPointsQuery,
	): Promise<SnapPointsResult>;
	getFonts(): Promise<FontListResult>;
	getSystem(query?: SystemQuery): Promise<SystemResult>;
	listAudit(query?: AuditQuery): Promise<AuditListResult>;
}

// One typed function per endpoint the editor uses (SDD-012 section 4.2). The
// transient-failure retry lives here, so every caller gets the same policy.
export function createClient(
	transport: Transport,
	deps: RetryDeps = defaultDeps,
): ApiClient {
	async function call<T>(
		method: "GET" | "POST",
		path: string,
		timeoutMs: number,
		body?: unknown,
	): Promise<T> {
		return withRetry(async () => {
			let response: Awaited<ReturnType<Transport["send"]>>;
			try {
				response = await transport.send({ method, path, body, timeoutMs });
			} catch (error) {
				if (error instanceof TransportError) throw networkError(error.message);
				throw error;
			}
			if (response.status < 200 || response.status > 299) {
				throw fromResponse(response.status, response.body);
			}
			return response.body as T;
		}, deps);
	}

	const project = (id: string): string => `/projects/${encodeURIComponent(id)}`;

	const query = (
		params: Record<string, string | number | undefined>,
	): string => {
		const search = new URLSearchParams();
		for (const [key, value] of Object.entries(params)) {
			if (value !== undefined && value !== "") search.set(key, String(value));
		}
		const text = search.toString();
		return text === "" ? "" : `?${text}`;
	};

	return {
		listMedia: (id) => call("GET", `${project(id)}/media`, READ_TIMEOUT_MS),
		importMedia: (id, paths) =>
			call("POST", `${project(id)}/media`, READ_TIMEOUT_MS, {
				files: paths.map((path) => ({ path })),
			}),
		getLint: (id) => call("GET", `${project(id)}/lint`, READ_TIMEOUT_MS),
		startExport: (id) =>
			call("POST", `${project(id)}/jobs`, READ_TIMEOUT_MS, { kind: "export" }),
		getJob: (jobId) =>
			call("GET", `/jobs/${encodeURIComponent(jobId)}`, READ_TIMEOUT_MS),
		cancelJob: (jobId) =>
			call(
				"POST",
				`/jobs/${encodeURIComponent(jobId)}/cancel`,
				EDIT_TIMEOUT_MS,
				{},
			),
		listJobs: (id) => call("GET", `${project(id)}/jobs`, READ_TIMEOUT_MS),
		getProject: (id) =>
			call("GET", `${project(id)}?detail=document`, READ_TIMEOUT_MS),
		edit: (id, request) =>
			call("POST", `${project(id)}/edit`, EDIT_TIMEOUT_MS, request),
		undo: (id) => call("POST", `${project(id)}/undo`, EDIT_TIMEOUT_MS, {}),
		redo: (id) => call("POST", `${project(id)}/redo`, EDIT_TIMEOUT_MS, {}),
		getPlan: (id, from, to, revision) =>
			call(
				"GET",
				`${project(id)}/plan?from=${from}&until=${to}&rev=${revision}`,
				READ_TIMEOUT_MS,
			),
		getCommands: () => call("GET", "/commands", READ_TIMEOUT_MS),
		getCommand: (name) =>
			call("GET", `/commands/${encodeURIComponent(name)}`, READ_TIMEOUT_MS),
		getHistory: (id, q = {}) =>
			call(
				"GET",
				`${project(id)}/history${query({ actor: q.actor, cursor: q.cursor, limit: q.limit })}`,
				READ_TIMEOUT_MS,
			),
		inspectMedia: (id, asset, q = {}) =>
			call(
				"GET",
				`${project(id)}/media/${encodeURIComponent(asset)}${query({
					sections: q.sections?.join(","),
					min_silence: q.minSilence,
					cursor: q.cursor,
					limit: q.limit,
				})}`,
				READ_TIMEOUT_MS,
			),
		trashMedia: (id, asset) =>
			call(
				"POST",
				`${project(id)}/media/${encodeURIComponent(asset)}/trash`,
				EDIT_TIMEOUT_MS,
				{},
			),
		restoreMedia: (id, asset) =>
			call(
				"POST",
				`${project(id)}/media/${encodeURIComponent(asset)}/restore`,
				EDIT_TIMEOUT_MS,
				{},
			),
		getPlanHead: (id, from, to, revision) =>
			call(
				"GET",
				`${project(id)}/plan?from=${from}&until=${to}&rev=${revision}`,
				READ_TIMEOUT_MS,
			),
		getSnapPoints: (id, q = {}) =>
			call(
				"GET",
				`${project(id)}/snap-points${query({ rev: q.revision, words: q.words === undefined ? undefined : q.words ? 1 : 0 })}`,
				READ_TIMEOUT_MS,
			),
		getFonts: () => call("GET", "/fonts", READ_TIMEOUT_MS),
		getSystem: (q = {}) =>
			call(
				"GET",
				`/system${query({ full: q.full ? 1 : undefined, verify: q.verify ? 1 : undefined })}`,
				READ_TIMEOUT_MS,
			),
		listAudit: (q = {}) =>
			call(
				"GET",
				`/audit${query({ project_id: q.projectId, cursor: q.cursor, limit: q.limit })}`,
				READ_TIMEOUT_MS,
			),
	};
}

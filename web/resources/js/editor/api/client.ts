import { fromResponse, networkError } from "./errors";
import { type RetryDeps, withRetry } from "./retry";
import { type Transport, TransportError } from "./transport";
import type {
	EditRequest,
	EditResult,
	Job,
	JobListResult,
	LintResult,
	MediaImportResult,
	MediaListResult,
	PlanWindow,
	PlanWindowHead,
	ProjectGetResult,
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
	getPlanHead(
		projectId: string,
		from: number,
		to: number,
		revision: number,
	): Promise<PlanWindowHead>;
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
				`${project(id)}/plan?from=${from}&to=${to}&rev=${revision}`,
				READ_TIMEOUT_MS,
			),
		getPlanHead: (id, from, to, revision) =>
			call(
				"GET",
				`${project(id)}/plan?from=${from}&to=${to}&rev=${revision}`,
				READ_TIMEOUT_MS,
			),
	};
}

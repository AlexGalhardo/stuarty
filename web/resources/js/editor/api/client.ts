import { fromResponse, networkError } from "./errors";
import { type RetryDeps, withRetry } from "./retry";
import { type Transport, TransportError } from "./transport";
import type {
	ApprovalListResult,
	ApprovalRequest,
	ApprovalRule,
	ApprovalRuleListResult,
	ApprovalRuleSetRequest,
	AssetListResult,
	AuditListResult,
	AuditQuery,
	BrandKit,
	BrandKitBody,
	BriefResult,
	BundleImportResult,
	CommandHelpResult,
	Connection,
	ConnectionCreateRequest,
	ConnectionListResult,
	ConnectionTestResult,
	CostCeilingSetRequest,
	CostSummary,
	EditRequest,
	EditResult,
	ExportBatchItem,
	ExportCheckResult,
	ExportOptions,
	ExportPresetListResult,
	FontImportResult,
	FontListResult,
	HistoryListResult,
	HistoryQuery,
	InstallToolRequest,
	InstallToolResult,
	Job,
	JobListResult,
	KeyCreateRequest,
	KeyCreateResult,
	KeyListResult,
	KeyRevokeResult,
	LintResult,
	MediaImportResult,
	MediaInspectQuery,
	MediaInspectResult,
	MediaListResult,
	MediaStateResult,
	PlanWindow,
	PlanWindowHead,
	ProjectCreateResult,
	ProjectGetResult,
	ProjectItemAddRequest,
	ProjectItemResult,
	ProjectListResult,
	ProjectStateResult,
	SessionResult,
	SnapPointsQuery,
	SnapPointsResult,
	SystemQuery,
	SystemResult,
	TemplateInstantiateRequest,
	TemplateInstantiateResult,
	TemplateListResult,
	TemplateResult,
	TemplateSaveRequest,
	ToolsStatusResult,
	UserListResult,
} from "./types";

// Timeouts of SDD-012 section 4.3 item 4.
const READ_TIMEOUT_MS = 10_000;
const EDIT_TIMEOUT_MS = 30_000;
// A bundle call may hold for up to 25 s (wait_ms) before the job is returned.
const BUNDLE_WAIT_MS = 20_000;
const BUNDLE_TIMEOUT_MS = 60_000;

const defaultDeps: RetryDeps = {
	sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
	random: Math.random,
};

export interface ApiClient {
	listMedia(projectId: string): Promise<MediaListResult>;
	importMedia(projectId: string, paths: string[]): Promise<MediaImportResult>;
	getLint(projectId: string): Promise<LintResult>;
	// `options` are the export arguments the server schema exposes (format only: fps
	// and size are project settings); `dryRun` runs the lint gate and the limits and queues nothing.
	startExport(
		projectId: string,
		options?: ExportOptions,
		dryRun?: boolean,
	): Promise<Job>;
	// A .vvbundle of the project (SDD-005 section 16); the job is polled with getJob.
	exportBundle(projectId: string, includeLog?: boolean): Promise<Job>;
	// `path` is absolute on the machine of the service; `asNew` makes a new id.
	importBundle(path: string, asNew: boolean): Promise<BundleImportResult>;
	// A transcription of one asset (SDD-012 section 12.4); E_TRANSCRIBE_UNAVAILABLE
	// answers at once when no provider can run.
	startTranscribe(projectId: string, assetId: string): Promise<Job>;
	// The platform presets, and with `codecs` which codecs this machine writes.
	getExportPresets(codecs?: boolean): Promise<ExportPresetListResult>;
	// What does not fit a preset, before anything is encoded.
	checkExport(
		projectId: string,
		preset: string,
		range?: { start?: string; end?: string },
	): Promise<ExportCheckResult>;
	// Several exports of the project, one after the other, as one job; the core
	// queues them behind the exports that are running.
	startExportBatch(
		projectId: string,
		items: readonly ExportBatchItem[],
	): Promise<Job>;
	getJob(jobId: string): Promise<Job>;
	cancelJob(jobId: string): Promise<Job>;
	listJobs(projectId: string): Promise<JobListResult>;
	getProject(projectId: string): Promise<ProjectGetResult>;
	edit(projectId: string, request: EditRequest): Promise<EditResult>;
	undo(projectId: string): Promise<EditResult>;
	redo(projectId: string): Promise<EditResult>;
	// `width` is the pixel width the preview draws at (16 to 4096): the bitmaps
	// of the plan come rasterized at that scale.
	getPlan(
		projectId: string,
		from: number,
		to: number,
		revision: number,
		width?: number,
		signal?: AbortSignal,
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
	// The ready files of the workspace by content hash (VV-117); `kind` keeps one kind.
	listAssets(kind?: "image" | "video" | "audio"): Promise<AssetListResult>;
	// `path` is an absolute path to a font file on the machine of the service.
	importFont(path: string): Promise<FontImportResult>;
	getBrandKit(): Promise<BrandKit>;
	// Replaces the whole kit (SDD-005 section 5.11).
	setBrandKit(kit: BrandKitBody): Promise<BrandKit>;
	// The OpenAPI document lives at the root, not under /api/v1: build the client
	// on a transport with an empty base.
	getOpenApi(): Promise<unknown>;
	getSystem(query?: SystemQuery): Promise<SystemResult>;
	listAudit(query?: AuditQuery): Promise<AuditListResult>;
	getTools(): Promise<ToolsStatusResult>;
	installTool(request: InstallToolRequest): Promise<InstallToolResult>;
	createKey(request: KeyCreateRequest): Promise<KeyCreateResult>;
	listKeys(): Promise<KeyListResult>;
	revokeKey(keyId: string): Promise<KeyRevokeResult>;
	listConnections(): Promise<ConnectionListResult>;
	createConnection(request: ConnectionCreateRequest): Promise<Connection>;
	removeConnection(id: string): Promise<Connection>;
	testConnection(id: string): Promise<ConnectionTestResult>;
	// The approval queue (SDD-015). Deciding, the rules and the ceilings are for a person in a
	// browser session; any other caller gets E_PRIVILEGED_SESSION.
	listApprovals(state?: string): Promise<ApprovalListResult>;
	decideApproval(
		id: string,
		decision: "approve" | "reject",
		reason?: string,
	): Promise<ApprovalRequest>;
	listApprovalRules(): Promise<ApprovalRuleListResult>;
	setApprovalRule(request: ApprovalRuleSetRequest): Promise<ApprovalRule>;
	removeApprovalRule(id: string): Promise<ApprovalRule>;
	getCostSummary(): Promise<CostSummary>;
	setCostCeiling(request: CostCeilingSetRequest): Promise<CostSummary>;
	// An administrator session; any other caller gets E_FORBIDDEN.
	listUsers(): Promise<UserListResult>;
	listTrashedProjects(): Promise<ProjectListResult>;
	createProject(name: string, preset: string): Promise<ProjectCreateResult>;
	duplicateProject(
		projectId: string,
		name?: string,
	): Promise<ProjectCreateResult>;
	addProjectItem(
		projectId: string,
		request: ProjectItemAddRequest,
	): Promise<ProjectItemResult>;
	moveProjectItem(
		projectId: string,
		itemId: string,
		position: number,
	): Promise<ProjectItemResult>;
	renameProjectItem(
		projectId: string,
		itemId: string,
		title: string,
	): Promise<ProjectItemResult>;
	trashProjectItem(
		projectId: string,
		itemId: string,
	): Promise<ProjectItemResult>;
	restoreProjectItem(
		projectId: string,
		itemId: string,
	): Promise<ProjectItemResult>;
	setBrief(projectId: string, text: string): Promise<BriefResult>;
	answerBriefing(
		projectId: string,
		answers: Record<string, string>,
		replace: boolean,
	): Promise<BriefResult>;
	trashProject(projectId: string): Promise<ProjectStateResult>;
	restoreProject(projectId: string): Promise<ProjectStateResult>;
	// The two routes of network mode live at the root, not under /api/v1: a client
	// for them is built on a transport with an empty base.
	login(email: string, password: string): Promise<SessionResult>;
	logout(): Promise<void>;
	listTemplates(): Promise<TemplateListResult>;
	instantiateTemplate(
		templateId: string,
		request: TemplateInstantiateRequest,
	): Promise<TemplateInstantiateResult>;
	// `path` is an absolute path on the machine of the service (SDD-007 section 7.6).
	importTemplate(path: string): Promise<TemplateResult>;
	saveTemplate(request: TemplateSaveRequest): Promise<TemplateResult>;
}

// One typed function per endpoint the editor uses (SDD-012 section 4.2). The
// transient-failure retry lives here, so every caller gets the same policy.
export function createClient(
	transport: Transport,
	deps: RetryDeps = defaultDeps,
): ApiClient {
	async function call<T>(
		method: "GET" | "POST" | "PUT" | "DELETE",
		path: string,
		timeoutMs: number,
		body?: unknown,
		signal?: AbortSignal,
	): Promise<T> {
		return withRetry(async () => {
			let response: Awaited<ReturnType<Transport["send"]>>;
			try {
				response = await transport.send({
					method,
					path,
					body,
					timeoutMs,
					signal,
				});
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
		startExport: (id, options = {}, dryRun = false) => {
			const args = Object.fromEntries(
				Object.entries(options).filter(([, value]) => value !== undefined),
			);
			return call("POST", `${project(id)}/jobs`, READ_TIMEOUT_MS, {
				kind: "export",
				...(Object.keys(args).length === 0 ? {} : { args }),
				...(dryRun ? { dry_run: true } : {}),
			});
		},
		exportBundle: (id, includeLog) =>
			call("POST", `${project(id)}/bundle`, BUNDLE_TIMEOUT_MS, {
				wait_ms: BUNDLE_WAIT_MS,
				...(includeLog === undefined ? {} : { include_log: includeLog }),
			}),
		importBundle: (path, asNew) =>
			call("POST", "/projects/import", BUNDLE_TIMEOUT_MS, {
				path,
				as_new: asNew,
			}),
		startTranscribe: (id, assetId) =>
			call("POST", `${project(id)}/jobs`, READ_TIMEOUT_MS, {
				kind: "transcribe",
				args: { asset_id: assetId },
			}),
		getExportPresets: (codecs) =>
			call(
				"GET",
				`/export-presets${codecs === true ? "?codecs=true" : ""}`,
				READ_TIMEOUT_MS * 3,
			),
		checkExport: (id, preset, range = {}) => {
			const query = new URLSearchParams({ preset });
			if (range.start !== undefined) query.set("start", range.start);
			if (range.end !== undefined) query.set("end", range.end);
			return call(
				"GET",
				`${project(id)}/export-check?${query.toString()}`,
				READ_TIMEOUT_MS,
			);
		},
		startExportBatch: (id, items) =>
			call("POST", `${project(id)}/export-batches`, READ_TIMEOUT_MS, {
				items: items.map((item) =>
					Object.fromEntries(
						Object.entries(item).filter(([, value]) => value !== undefined),
					),
				),
			}),
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
		getPlan: (id, from, to, revision, width, signal) =>
			call(
				"GET",
				`${project(id)}/plan?from=${from}&until=${to}&rev=${revision}${
					width === undefined ? "" : `&width=${width}`
				}`,
				READ_TIMEOUT_MS,
				undefined,
				signal,
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
		listAssets: (kind) =>
			call("GET", `/assets${query({ kind })}`, READ_TIMEOUT_MS),
		importFont: (path) => call("POST", "/fonts", EDIT_TIMEOUT_MS, { path }),
		getBrandKit: () => call("GET", "/brand-kit", READ_TIMEOUT_MS),
		setBrandKit: (kit) => call("PUT", "/brand-kit", EDIT_TIMEOUT_MS, kit),
		getOpenApi: () => call("GET", "/openapi.json", READ_TIMEOUT_MS),
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
		getTools: () => call("GET", "/system/tools", READ_TIMEOUT_MS),
		installTool: (request) =>
			call("POST", "/system/tools/install", EDIT_TIMEOUT_MS, request),
		createKey: (request) => call("POST", "/keys", EDIT_TIMEOUT_MS, request),
		listKeys: () => call("GET", "/keys", READ_TIMEOUT_MS),
		listUsers: () => call("GET", "/users", READ_TIMEOUT_MS),
		listConnections: () => call("GET", "/connections", READ_TIMEOUT_MS),
		createConnection: (request) =>
			call("POST", "/connections", EDIT_TIMEOUT_MS, request),
		removeConnection: (id) =>
			call("DELETE", `/connections/${encodeURIComponent(id)}`, EDIT_TIMEOUT_MS),
		testConnection: (id) =>
			call(
				"POST",
				`/connections/${encodeURIComponent(id)}/test`,
				EDIT_TIMEOUT_MS,
				{},
			),
		listApprovals: (state) =>
			call(
				"GET",
				`/approvals?state=${encodeURIComponent(state ?? "pending")}&limit=200`,
				READ_TIMEOUT_MS,
			),
		decideApproval: (id, decision, reason) =>
			call(
				"POST",
				`/approvals/${encodeURIComponent(id)}/decide`,
				EDIT_TIMEOUT_MS,
				reason === undefined || reason === ""
					? { decision }
					: { decision, reason },
			),
		listApprovalRules: () => call("GET", "/approval-rules", READ_TIMEOUT_MS),
		setApprovalRule: (request) =>
			call("POST", "/approval-rules", EDIT_TIMEOUT_MS, request),
		removeApprovalRule: (id) =>
			call(
				"DELETE",
				`/approval-rules/${encodeURIComponent(id)}`,
				EDIT_TIMEOUT_MS,
			),
		getCostSummary: () => call("GET", "/cost", READ_TIMEOUT_MS),
		setCostCeiling: (request) =>
			call("PUT", "/cost/ceilings", EDIT_TIMEOUT_MS, request),
		revokeKey: (keyId) =>
			call("DELETE", `/keys/${encodeURIComponent(keyId)}`, EDIT_TIMEOUT_MS),
		listTrashedProjects: () =>
			call("GET", "/projects?state=trashed", READ_TIMEOUT_MS),
		createProject: (name, preset) =>
			call("POST", "/projects", EDIT_TIMEOUT_MS, { name, preset }),
		duplicateProject: (id, name) =>
			call(
				"POST",
				`${project(id)}/duplicate`,
				EDIT_TIMEOUT_MS,
				name === undefined ? {} : { name },
			),
		addProjectItem: (id, request) =>
			call("POST", `${project(id)}/items`, EDIT_TIMEOUT_MS, request),
		moveProjectItem: (id, itemId, position) =>
			call(
				"POST",
				`${project(id)}/items/${encodeURIComponent(itemId)}/move`,
				EDIT_TIMEOUT_MS,
				{ position },
			),
		renameProjectItem: (id, itemId, title) =>
			call(
				"POST",
				`${project(id)}/items/${encodeURIComponent(itemId)}/rename`,
				EDIT_TIMEOUT_MS,
				{ title },
			),
		trashProjectItem: (id, itemId) =>
			call(
				"POST",
				`${project(id)}/items/${encodeURIComponent(itemId)}/trash`,
				EDIT_TIMEOUT_MS,
				{},
			),
		restoreProjectItem: (id, itemId) =>
			call(
				"POST",
				`${project(id)}/items/${encodeURIComponent(itemId)}/restore`,
				EDIT_TIMEOUT_MS,
				{},
			),
		setBrief: (id, text) =>
			call("PUT", `${project(id)}/brief`, EDIT_TIMEOUT_MS, { text }),
		answerBriefing: (id, answers, replace) =>
			call("PUT", `${project(id)}/briefing`, EDIT_TIMEOUT_MS, {
				answers,
				replace,
			}),
		trashProject: (id) =>
			call("POST", `${project(id)}/trash`, EDIT_TIMEOUT_MS, {}),
		restoreProject: (id) =>
			call("POST", `${project(id)}/restore`, EDIT_TIMEOUT_MS, {}),
		login: (email, password) =>
			call("POST", "/auth/login", EDIT_TIMEOUT_MS, { email, password }),
		logout: async () => {
			await call("POST", "/auth/logout", EDIT_TIMEOUT_MS, {});
		},
		listTemplates: () => call("GET", "/templates", READ_TIMEOUT_MS),
		instantiateTemplate: (templateId, request) =>
			call(
				"POST",
				`/templates/${encodeURIComponent(templateId)}/instantiate`,
				EDIT_TIMEOUT_MS,
				request,
			),
		importTemplate: (path) =>
			call("POST", "/templates/import", EDIT_TIMEOUT_MS, { path }),
		saveTemplate: (request) =>
			call("POST", "/templates", EDIT_TIMEOUT_MS, request),
	};
}

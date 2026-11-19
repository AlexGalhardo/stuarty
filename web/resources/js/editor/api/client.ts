import { fromResponse, networkError } from "./errors";
import { type RetryDeps, withRetry } from "./retry";
import { type Transport, TransportError } from "./transport";
import type {
	AgentProfile,
	AgentProfileListResult,
	ApprovalListResult,
	ApprovalRequest,
	ApprovalRule,
	ApprovalRuleListResult,
	ApprovalRuleSetRequest,
	AssetInfoBody,
	AssetListResult,
	AssetQuery,
	AssetResult,
	AudioMeasureQuery,
	AudioMeasureResult,
	AuditListResult,
	AuditQuery,
	BrandKit,
	BrandKitBody,
	BriefResult,
	BundleImportResult,
	ChatSession,
	ChatSessionListResult,
	ChatTranscriptResult,
	Checkpoint,
	CheckpointListResult,
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
	HistoryDiffResult,
	HistoryListResult,
	HistoryQuery,
	ImageGenerateRequest,
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
	ProviderListResult,
	ProviderModelsResult,
	ProviderPriceSetRequest,
	ProviderPricesResult,
	PublishRequest,
	PublishValidateResult,
	RevertRequest,
	SessionResult,
	SnapPointsQuery,
	SnapPointsResult,
	SocialAccount,
	SocialAccountListResult,
	SocialCompleteRequest,
	SocialConnectRequest,
	SocialConnectResult,
	SocialNetworkListResult,
	SocialSelectRequest,
	SystemQuery,
	SystemResult,
	TemplateInstantiateRequest,
	TemplateInstantiateResult,
	TemplateListResult,
	TemplateResult,
	TemplateSaveRequest,
	ToolsStatusResult,
	UserListResult,
	VoiceGenerateRequest,
	VoiceListResult,
} from "./types";

// Timeouts of SDD-012 section 4.3 item 4.
const READ_TIMEOUT_MS = 10_000;
const EDIT_TIMEOUT_MS = 30_000;
// Starting an agent waits for its process to start and open the protocol (SDD-017 section 4).
const CHAT_START_TIMEOUT_MS = 45_000;
// A bundle call may hold for up to 25 s (wait_ms) before the job is returned.
const BUNDLE_WAIT_MS = 20_000;
const BUNDLE_TIMEOUT_MS = 60_000;
// The sound of up to 600 s is decoded and mixed twice.
const MEASURE_TIMEOUT_MS = 120_000;

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
	// The level of the sound before and after the audio processors (SDD-004 section 7.1).
	measureAudio(
		projectId: string,
		query?: AudioMeasureQuery,
	): Promise<AudioMeasureResult>;
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
	// Revert, the diff of two revisions and checkpoints (epic 36). The core words
	// every change; a revert with `dry_run` returns the diff it would make.
	getCheckpoints(projectId: string): Promise<CheckpointListResult>;
	createCheckpoint(
		projectId: string,
		body: { name: string; note?: string; revision?: number },
	): Promise<Checkpoint>;
	removeCheckpoint(projectId: string, name: string): Promise<Checkpoint>;
	getHistoryDiff(
		projectId: string,
		range: { from: number; until?: number },
	): Promise<HistoryDiffResult>;
	revertTo(
		projectId: string,
		request: RevertRequest,
	): Promise<HistoryRevertResult>;
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
	// The asset library with its filters (SDD-011 section 14.4), and one entry with the projects that use it.
	queryAssets(query: AssetQuery): Promise<AssetListResult>;
	getAsset(hash: string): Promise<AssetResult>;
	setAssetInfo(hash: string, info: AssetInfoBody): Promise<AssetResult>;
	tagAsset(hash: string, add: string[], remove: string[]): Promise<AssetResult>;
	// Uses a file of the library in a project: no copy, no ingest (SDD-011 section 14.3).
	useAsset(projectId: string, hash: string): Promise<MediaImportResult>;
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
	// The social networks and publishing (SDD-020): a publication waits for a person to approve it.
	listSocialNetworks(): Promise<SocialNetworkListResult>;
	connectSocial(request: SocialConnectRequest): Promise<SocialConnectResult>;
	completeSocial(request: SocialCompleteRequest): Promise<SocialAccount>;
	listSocialAccounts(): Promise<SocialAccountListResult>;
	selectSocialAccount(request: SocialSelectRequest): Promise<SocialAccount>;
	validatePublish(request: PublishRequest): Promise<PublishValidateResult>;
	publishNow(request: PublishRequest): Promise<Job>;
	listActiveProjects(): Promise<ProjectListResult>;
	createConnection(request: ConnectionCreateRequest): Promise<Connection>;
	removeConnection(id: string): Promise<Connection>;
	testConnection(id: string): Promise<ConnectionTestResult>;
	// The AI providers (SDD-016). Setting a price is for a person in a browser session; any other
	// caller gets E_PRIVILEGED_SESSION.
	listProviders(): Promise<ProviderListResult>;
	listProviderModels(
		connectionId: string,
		refresh?: boolean,
	): Promise<ProviderModelsResult>;
	setProviderPrice(
		request: ProviderPriceSetRequest,
	): Promise<ProviderPricesResult>;
	// Image and voice generation (SDD-016 section 6.1): each answers the job with its estimate, which waits
	// for a person; dry_run answers the estimate and writes nothing.
	generateImage(request: ImageGenerateRequest): Promise<Job>;
	generateVoice(request: VoiceGenerateRequest): Promise<Job>;
	listVoices(connectionId: string): Promise<VoiceListResult>;
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
	// The hosted agent chat (SDD-017). Everything that drives a session, and the profiles, are for
	// a person in a browser session; any other caller gets E_PRIVILEGED_SESSION.
	listAgentProfiles(): Promise<AgentProfileListResult>;
	setAgentProfile(
		name: string,
		command: string,
		args: string[],
	): Promise<AgentProfile>;
	removeAgentProfile(name: string): Promise<AgentProfile>;
	startChatSession(projectId: string, profile: string): Promise<ChatSession>;
	sendChatMessage(sessionId: string, text: string): Promise<ChatSession>;
	cancelChat(sessionId: string, end: boolean): Promise<ChatSession>;
	answerChatPermission(
		sessionId: string,
		requestId: string,
		answer: "allow" | "reject" | "cancel",
	): Promise<ChatSession>;
	listChatSessions(projectId: string): Promise<ChatSessionListResult>;
	getChatTranscript(
		sessionId: string,
		options?: { after?: number; format?: "json" | "markdown" },
	): Promise<ChatTranscriptResult>;
	deleteChatSession(sessionId: string): Promise<ChatSession>;
	// An administrator session; any other caller gets E_FORBIDDEN.
	listUsers(): Promise<UserListResult>;
	listTrashedProjects(): Promise<ProjectListResult>;
	listProjects(): Promise<ProjectListResult>;
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
		measureAudio: (id, q = {}) => {
			const params = new URLSearchParams();
			if (q.itemId !== undefined) params.set("item_id", q.itemId);
			if (q.start !== undefined) params.set("start", q.start);
			if (q.end !== undefined) params.set("end", q.end);
			const suffix = params.size === 0 ? "" : `?${params.toString()}`;
			return call(
				"GET",
				`${project(id)}/audio-measure${suffix}`,
				MEASURE_TIMEOUT_MS,
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
		getCheckpoints: (id) =>
			call("GET", `${project(id)}/checkpoints`, READ_TIMEOUT_MS),
		createCheckpoint: (id, body) =>
			call("POST", `${project(id)}/checkpoints`, EDIT_TIMEOUT_MS, body),
		removeCheckpoint: (id, name) =>
			call(
				"DELETE",
				`${project(id)}/checkpoints/${encodeURIComponent(name)}`,
				EDIT_TIMEOUT_MS,
			),
		getHistoryDiff: (id, range) =>
			call(
				"GET",
				`${project(id)}/diff${query({ from: range.from, until: range.until })}`,
				READ_TIMEOUT_MS,
			),
		revertTo: (id, request) =>
			call("POST", `${project(id)}/revert`, EDIT_TIMEOUT_MS, request),
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
		queryAssets: (q) =>
			call(
				"GET",
				`/assets${query({ ...q, reusable: q.reusable === undefined ? undefined : String(q.reusable) })}`,
				READ_TIMEOUT_MS,
			),
		getAsset: (hash) =>
			call("GET", `/assets/${encodeURIComponent(hash)}`, READ_TIMEOUT_MS),
		setAssetInfo: (hash, info) =>
			call("PUT", `/assets/${encodeURIComponent(hash)}`, EDIT_TIMEOUT_MS, info),
		tagAsset: (hash, add, remove) =>
			call(
				"POST",
				`/assets/${encodeURIComponent(hash)}/tags`,
				EDIT_TIMEOUT_MS,
				{
					add,
					remove,
				},
			),
		useAsset: (id, hash) =>
			call("POST", `${project(id)}/media`, EDIT_TIMEOUT_MS, {
				files: [{ hash }],
			}),
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
		listSocialNetworks: () => call("GET", "/social/networks", READ_TIMEOUT_MS),
		connectSocial: (request) =>
			call("POST", "/social/connect", EDIT_TIMEOUT_MS, request),
		completeSocial: (request) =>
			call("POST", "/social/complete", EDIT_TIMEOUT_MS, request),
		listSocialAccounts: () => call("GET", "/social/accounts", READ_TIMEOUT_MS),
		selectSocialAccount: (request) =>
			call("POST", "/social/select", EDIT_TIMEOUT_MS, request),
		validatePublish: (request) =>
			call("POST", "/publish/validate", EDIT_TIMEOUT_MS, request),
		publishNow: (request) => call("POST", "/publish", EDIT_TIMEOUT_MS, request),
		listActiveProjects: () => call("GET", "/projects", READ_TIMEOUT_MS),
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
		listAgentProfiles: () => call("GET", "/agents/profiles", READ_TIMEOUT_MS),
		setAgentProfile: (name, command, args) =>
			call(
				"PUT",
				`/agents/profiles/${encodeURIComponent(name)}`,
				EDIT_TIMEOUT_MS,
				{ command, args },
			),
		removeAgentProfile: (name) =>
			call(
				"DELETE",
				`/agents/profiles/${encodeURIComponent(name)}`,
				EDIT_TIMEOUT_MS,
			),
		startChatSession: (projectId, profile) =>
			call(
				"POST",
				`${project(projectId)}/chat/sessions`,
				CHAT_START_TIMEOUT_MS,
				{ profile },
			),
		sendChatMessage: (sessionId, text) =>
			call(
				"POST",
				`/chat/sessions/${encodeURIComponent(sessionId)}/messages`,
				EDIT_TIMEOUT_MS,
				{ text },
			),
		cancelChat: (sessionId, end) =>
			call(
				"POST",
				`/chat/sessions/${encodeURIComponent(sessionId)}/cancel`,
				EDIT_TIMEOUT_MS,
				{ end },
			),
		answerChatPermission: (sessionId, requestId, answer) =>
			call(
				"POST",
				`/chat/sessions/${encodeURIComponent(sessionId)}/permissions/${encodeURIComponent(requestId)}`,
				EDIT_TIMEOUT_MS,
				{ answer },
			),
		listChatSessions: (projectId) =>
			call("GET", `${project(projectId)}/chat/sessions`, READ_TIMEOUT_MS),
		getChatTranscript: (sessionId, options) =>
			call(
				"GET",
				`/chat/sessions/${encodeURIComponent(sessionId)}/transcript?format=${options?.format ?? "json"}&after=${options?.after ?? 0}`,
				READ_TIMEOUT_MS,
			),
		deleteChatSession: (sessionId) =>
			call(
				"DELETE",
				`/chat/sessions/${encodeURIComponent(sessionId)}`,
				EDIT_TIMEOUT_MS,
			),
		listProviders: () => call("GET", "/providers", READ_TIMEOUT_MS),
		listProviderModels: (connectionId, refresh) =>
			call(
				"GET",
				`/connections/${encodeURIComponent(connectionId)}/models${refresh === true ? "?refresh=true" : ""}`,
				EDIT_TIMEOUT_MS,
			),
		setProviderPrice: (request) =>
			call("PUT", "/providers/prices", EDIT_TIMEOUT_MS, request),
		generateImage: (request) =>
			call("POST", "/image/generate", EDIT_TIMEOUT_MS, request),
		generateVoice: (request) =>
			call("POST", "/voice/generate", EDIT_TIMEOUT_MS, request),
		listVoices: (connectionId) =>
			call(
				"GET",
				`/connections/${encodeURIComponent(connectionId)}/voices`,
				READ_TIMEOUT_MS,
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
		listProjects: () => call("GET", "/projects", READ_TIMEOUT_MS),
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

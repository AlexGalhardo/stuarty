// Wire types of the HTTP API the editor reads and the event stream. They
// mirror SDD-003 section 2 (results), SDD-002 section A.5 (the diff), SDD-006
// section 3 (events) and SDD-004 section 10 (the plan window). The document
// types come from the generated contracts and are never rewritten here.
// SDD-012 section 3.2 plans generated/api.ts and generated/events.ts for
// these; until the OpenAPI and event schemas are generated they live here.

import type { ProjectDocument } from "@/generated/timeline";
import type { ProjectSummary } from "@/types";

export type ProjectState = "active" | "trashed";

export interface TimeValue {
	frame: number;
	seconds: number;
}

export interface ProjectCounts {
	tracks: number;
	items: number;
	assets: number;
	caption_words: number;
}

// GET /projects/{id}?detail=document (SDD-003 section 2.3).
export interface ProjectGetResult {
	project_id: string;
	name: string;
	revision: number;
	state: ProjectState;
	width: number;
	height: number;
	fps: string;
	duration_frames: number;
	duration: TimeValue;
	counts: ProjectCounts;
	next_cursor: string | null;
	document?: ProjectDocument;
	text?: string;
}

export type JsonValue =
	| null
	| boolean
	| number
	| string
	| JsonValue[]
	| { [key: string]: JsonValue };

// One entry of a diff (SDD-002 section A.5). `updated` maps each changed
// top-level field to [before, after]; an absent side is null.
export type Change =
	| {
			kind:
				| "asset.added"
				| "asset.removed"
				| "track.added"
				| "track.removed"
				| "item.added"
				| "item.removed"
				| "font.added"
				| "font.removed";
			id: string;
			object: { [key: string]: JsonValue };
	  }
	| {
			kind:
				| "settings.updated"
				| "track.updated"
				| "item.updated"
				| "project.updated";
			id: string;
			changes: { [field: string]: [JsonValue, JsonValue] };
	  };

export interface OpEffect {
	index: number;
	op: string;
	effective: { [key: string]: JsonValue };
	created?: string[];
}

export interface LintFinding {
	code: string;
	severity: string;
	message: string;
	item?: string;
	[key: string]: unknown;
}

// A primitive an undo or redo left alone (SDD-005 section 12.3).
export interface SkippedOp {
	reason: string;
	fields?: string[];
	by_revision?: number;
	by_actor?: string;
}

// POST /projects/{id}/edit, undo and redo (SDD-003 sections 2.4 and 2.5).
export interface EditResult {
	project_id: string;
	revision: number;
	base_revision: number;
	applied: boolean;
	dry_run: boolean;
	replayed: boolean;
	actor: string;
	created: string[];
	effective: OpEffect[];
	diff: Change[];
	findings: LintFinding[];
	lint: { errors: number; warnings: number };
	description: string;
	kind?: "undo" | "redo";
	skipped?: SkippedOp[];
}

export interface EditRequest {
	operations: { op: string; args: { [key: string]: JsonValue } }[];
	base_revision: number;
	idempotency_key: string;
	dry_run?: boolean;
}

// The five events of SDD-006 section 3. `changed` replaces `diff` when the
// diff did not fit in one event.
export type ProjectEvent =
	| {
			name: "batch.committed";
			project_id: string;
			revision: number;
			base_revision: number;
			kind: "edit" | "undo" | "redo" | "migrate" | "import";
			actor: string;
			at: string;
			description: string;
			diff?: Change[];
			diff_truncated?: boolean;
			changed?: { kind: string; id: string }[];
			changed_more?: number;
			target_revision?: number;
			skipped_count?: number;
	  }
	| {
			name: "project.updated";
			project_id: string;
			state: ProjectState;
			at: string;
	  }
	| {
			name: "asset.updated";
			project_id: string;
			asset_id: string;
			state: "pending" | "ingesting" | "ready" | "failed";
			progress: number;
	  }
	| {
			name: "job.updated";
			project_id: string;
			job_id: string;
			kind: string;
			state: string;
			progress: number;
			message?: string;
	  }
	| {
			name: "resync.required";
			project_id: string;
			revision: number;
			reason: string;
	  };

export const EVENT_NAMES = [
	"batch.committed",
	"project.updated",
	"asset.updated",
	"job.updated",
	"resync.required",
] as const;

// The part of PlanWindow (SDD-004 section 10) the timeline needs: the end of
// the project as the core computed it.
export interface PlanWindowHead {
	revision: number;
	from: number;
	to: number;
	end_frame: number;
}

// One value per frame from `first` to `last`, or a single number when the
// value is constant over them (SDD-004 section 10).
export type PerFrame = number | number[];

interface PlanLayerBase {
	item: string;
	first: number;
	last: number;
	cx: PerFrame;
	cy: PerFrame;
	w: PerFrame;
	h: PerFrame;
	rotation: PerFrame;
	// The effective scale and opacity, keyframes and presets applied.
	scale: PerFrame;
	opacity: PerFrame;
}

export interface PlanMediaLayer extends PlanLayerBase {
	kind: "media";
	asset: string;
	asset_kind: "video" | "image";
	source_seconds: number[];
	crop: [number, number, number, number];
	color_matrix: number[] | null;
}

export interface PlanBitmapLayer extends PlanLayerBase {
	kind: "bitmap";
	bitmaps: { keys: string[]; index: number[] };
	natural: [number, number];
}

export type PlanLayer = PlanMediaLayer | PlanBitmapLayer;

export interface PlanAudio {
	item: string;
	asset: string;
	first: number;
	last: number;
	source_seconds: number;
	speed: number;
	gain: PerFrame;
}

// GET /projects/{id}/plan (SDD-004 section 10).
export interface PlanWindow extends PlanWindowHead {
	width: number;
	height: number;
	background: string;
	safe_area: [number, number, number, number];
	layers: PlanLayer[];
	audio: PlanAudio[];
}

// The envelope of SDD-002 section A.8, plus the HTTP status.
export interface ApiErrorBody {
	code: string;
	message: string;
	hint?: string;
	retryable: boolean;
	op_index?: number;
	path?: string;
	details?: { [key: string]: JsonValue };
	request_id?: string;
}

// Media, jobs and export (SDD-003 sections 2.6 and 2.8).
export type AssetState = "pending" | "ingesting" | "ready" | "failed";

export interface MediaEntry {
	asset_id: string;
	name: string;
	kind: "video" | "audio" | "image";
	state: AssetState;
	progress: number;
	duration_frames: number | null;
	duration_seconds: number | null;
	width?: number;
	height?: number;
	has_audio: boolean;
	trashed: boolean;
	strip?: {
		tile_w: number;
		tile_h: number;
		columns: number;
		count: number;
		interval_ms: number;
	};
	error?: ApiErrorBody;
}

export interface MediaListResult {
	project_id: string;
	assets: MediaEntry[];
	next_cursor: string | null;
	text: string;
}

export interface ImportedAsset {
	asset_id?: string;
	name: string;
	kind?: string;
	state: AssetState;
	progress: number;
	job_id?: string;
	deduplicated: boolean;
	error?: ApiErrorBody;
}

export interface MediaImportResult {
	project_id: string;
	revision: number;
	assets: ImportedAsset[];
	failed: number;
	text: string;
}

export type JobState =
	| "queued"
	| "running"
	| "succeeded"
	| "failed"
	| "cancelled";

export interface ExportResult {
	revision: number;
	path?: string;
	download_url: string;
	bytes: number;
	sha256?: string;
	duration: TimeValue;
	width: number;
	height: number;
}

export interface Job {
	job_id: string;
	kind: string;
	project_id: string | null;
	state: JobState;
	progress: number;
	message?: string;
	result?: {
		export?: ExportResult;
		transcribe?: { asset_id: string; language: string; words: number };
	};
	error?: ApiErrorBody;
	lint?: { errors: number; warnings: number };
	actor: string;
	created_at: string;
	text: string;
}

export interface JobListResult {
	project_id: string;
	jobs: Job[];
	next_cursor: string | null;
	text: string;
}

export interface LintResult {
	project_id: string;
	revision: number;
	findings: LintFinding[];
	text: string;
	[key: string]: unknown;
}

// GET /commands and GET /commands/{name} (SDD-000 CommandHelp). The list has
// the summaries, one command has its schema, example and error codes.
export interface CommandSummary {
	name: string;
	phase: "prototype" | "alpha" | "beta";
	scope: string;
	summary: string;
	internal?: boolean;
}

export interface CommandDetail extends CommandSummary {
	args_schema: Record<string, unknown>;
	description: string;
	errors: string[];
	example: Record<string, unknown>;
}

export interface CommandHelpResult {
	commands?: CommandSummary[];
	command?: CommandDetail;
	text: string;
}

// GET /projects/{id}/history (SDD-003 section 2.4).
export interface HistoryEntry {
	revision: number;
	actor: string;
	at: string;
	kind: string;
	changes: number;
	ops: string[];
	undo_of: number | null;
	undone_by: number | null;
	description: string;
	// True on the one step Undo or Redo would take for the caller.
	undoable_by_me: boolean;
	redoable_by_me: boolean;
}

export interface HistoryListResult {
	project_id: string;
	revision: number;
	entries: HistoryEntry[];
	next_cursor: string | null;
	text: string;
}

export interface HistoryQuery {
	actor?: string;
	cursor?: string;
	limit?: number;
}

// GET /projects/{id}/media/{asset_id}: the facts and the text of one asset.
export interface MediaInspectResult {
	asset_id: string;
	kind: string;
	state: AssetState;
	progress: number;
	available: string[];
	probe?: Record<string, unknown>;
	loudness?: Record<string, unknown>;
	silences?: { start: TimeValue; end: TimeValue }[];
	strip?: {
		tile_w: number;
		tile_h: number;
		columns: number;
		count: number;
		interval_ms: number;
	};
	next_cursor: string | null;
	text: string;
}

export interface MediaInspectQuery {
	sections?: string[];
	minSilence?: string;
	cursor?: string;
	limit?: number;
}

// POST .../trash and .../restore.
export interface MediaStateResult {
	asset_id: string;
	state: string;
	text: string;
}

// GET /projects/{id}/snap-points (SDD-012 section 7.8).
export interface SnapPoint {
	frame: number;
	kind: "timeline_start" | "item_start" | "item_end" | "caption_word";
	item?: string;
	track?: string;
}

export interface SnapPointsResult {
	revision: number;
	points: SnapPoint[];
	// Per item id, in frames.
	limits: Record<string, { trim_start_min: number; trim_end_max: number }>;
}

export interface SnapPointsQuery {
	revision?: number;
	words?: boolean;
}

// GET /fonts: the bundled families and their weights.
export interface FontFamily {
	family: string;
	weights: number[];
}

export interface FontListResult {
	fonts: FontFamily[];
}

// GET /system (Doctor).
export interface DoctorCheck {
	name: string;
	ok: boolean;
	detail: string;
	hint?: string;
}

export interface SystemResult {
	version: string;
	api_version: string;
	mode: string;
	pid: number;
	started_at: string;
	workspace?: string;
	checks?: DoctorCheck[];
	text: string;
}

export interface SystemQuery {
	full?: boolean;
	verify?: boolean;
}

// GET /audit (admin scope).
export interface AuditEntry {
	id: number;
	at: string;
	actor: string;
	action: string;
	project_id?: string;
	subject_id?: string;
	details?: unknown;
}

export interface AuditListResult {
	entries: AuditEntry[];
	next_cursor: string | null;
}

export interface AuditQuery {
	projectId?: string;
	cursor?: string;
	limit?: number;
}

// GET /system/tools and POST /system/tools/install (admin scope).
export interface InstalledTool {
	name: string;
	kind: string;
	model?: string;
	version: string;
	sha256: string;
	path: string;
}

export interface ToolsStatusResult {
	installed: InstalledTool[];
	state: "idle" | "running" | "failed";
	done_bytes: number;
	total_bytes: number;
	error?: ApiErrorBody;
	text: string;
}

export interface InstallToolRequest {
	name: string;
	model?: string;
}

export interface InstallToolResult {
	state: string;
	files: { name: string; bytes: number; host: string }[];
	download_bytes: number;
	text: string;
}

// /keys: a browser session in network mode only.
export interface ApiKey {
	key_id: string;
	name: string;
	scopes: string[];
	project_id: string | null;
	created_at: string;
	last_used_at: string | null;
	expires_at: string | null;
	revoked_at: string | null;
}

export interface KeyCreateRequest {
	name: string;
	scopes: string[];
	project_id?: string;
	expires_in_days?: number;
}

export interface KeyCreateResult {
	key: ApiKey;
	secret: string;
}

export interface KeyListResult {
	keys: ApiKey[];
}

export type KeyRevokeResult = ApiKey;

// GET /users: an administrator session only (SDD-007 section 4.1 item 2). The
// editor reads it once, to show the name of the actors `user:<id>`.
export interface UserSummary {
	user_id: string;
	email: string;
	role: string;
	disabled: boolean;
}

export interface UserListResult {
	users: UserSummary[];
}

// /templates (SDD-003 section 2.9). Every text of a template is untrusted: an
// imported file can carry anything.
export interface SlotInfo {
	name: string;
	kind: "media" | "text" | "caption";
	required: boolean;
	description: string;
	duration_policy?: string;
}

export interface TemplateSummary {
	template_id: string;
	name: string;
	description: string;
	slots: SlotInfo[];
	colors?: { [name: string]: string };
	width: number;
	height: number;
	duration: TimeValue;
}

export interface TemplateListResult {
	templates: TemplateSummary[];
	next_cursor: string | null;
	text: string;
}

export interface TemplateResult {
	template: TemplateSummary;
	text: string;
}

export interface TemplateInstantiateRequest {
	name: string;
	fill?: { [slot: string]: { asset_id: string } | { text: string } };
}

export interface TemplateInstantiateResult {
	project_id: string;
	revision: number;
	unfilled: string[];
	findings: LintFinding[];
	text: string;
}

// GET /projects?state=trashed and POST /projects/{id}/trash or /restore.
export interface ProjectListResult {
	projects: ProjectSummary[];
	next_cursor: string | null;
	text: string;
}

export interface ProjectStateResult {
	project_id: string;
	state: ProjectState;
	text: string;
}

// POST /auth/login and /auth/logout (network mode, SDD-007 section 4.1).
export interface SessionResult {
	user_id: number;
	email: string;
	role: string;
	scopes: string[];
}

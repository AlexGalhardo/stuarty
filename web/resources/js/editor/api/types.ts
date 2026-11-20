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
	  }
	| {
			// A timeline this one uses as a composition changed (SDD-006 section
			// 3.3.1): `timeline_id` and `revision` are those of that timeline.
			name: "nested.updated";
			project_id: string;
			timeline_id: string;
			revision: number;
			at: string;
	  };

export const EVENT_NAMES = [
	"batch.committed",
	"project.updated",
	"asset.updated",
	"job.updated",
	"resync.required",
	"nested.updated",
] as const;

// The event of GET /images/{id}/events (SDD-006 section 12); `resync.required` is the one above.
export const IMAGE_EVENT_NAMES = ["image.updated"] as const;

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
	// The timeline whose assets table names this layer's asset, for a layer that comes from a composition
	// (SDD-004 section 3.16); absent for a layer of the timeline being edited.
	timeline?: string;
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
	// The mix of SDD-004 section 3.8, constant over the item; absent is normal.
	blend?: PlanBlend;
	// The mask of SDD-004 section 3.9, constant over the item; absent without one.
	mask?: PlanMask;
	// The mask of each frame from `first` (a mask or null) when a mask property
	// is animated; it then replaces `mask`.
	masks?: (PlanMask | null)[];
	// The mirror of SDD-004 section 3.10, constant over the item; absent is off.
	flip_x?: boolean;
	flip_y?: boolean;
	// The pixel effects of SDD-004 section 3.13, constant over the item, in the
	// order of the item; absent without any. A shake is not here: the plan has
	// already moved the layer.
	effects?: PlanEffect[];
	// A whip, dip or wipe (SDD-004 section 3.2): one entry per frame from
	// `first`, null where there is none; a frame past the end has none.
	transition?: (PlanTransition | null)[];
	// The drop shadow and the outline of SDD-004 section 3.15, constant over
	// the item; absent without one.
	shadow?: PlanShadow;
	outline?: PlanOutline;
}

export interface PlanEffect {
	kind: "motion_blur" | "glow" | "vignette" | "grain" | "rgb_split";
	amount?: number;
	angle?: number;
	radius?: number;
	threshold?: number;
	softness?: number;
	size?: number;
	seed?: number;
}

export interface PlanTransition {
	kind: "whip" | "dip" | "wipe";
	edge: "in" | "out";
	// The progress as 16.16 fixed point, strictly between 0 and 65536.
	p: number;
	// A dip: `#RRGGBB`, black when absent.
	color?: string;
	// A whip or a wipe: `left` when absent.
	direction?: "left" | "right" | "up" | "down";
}

export interface PlanMask {
	shape: "rect" | "ellipse";
	x: number;
	y: number;
	width: number;
	height: number;
	// The plan leaves out a feather of 0.
	feather?: number;
	invert?: boolean;
}

// The drop shadow of SDD-004 section 3.15. Lengths are project pixels.
export interface PlanShadow {
	color: string;
	opacity: number;
	dx: number;
	dy: number;
	blur: number;
}

// The outline of SDD-004 section 3.15. `width` is project pixels.
export interface PlanOutline {
	color: string;
	width: number;
}

export type PlanBlend = "normal" | "multiply" | "screen" | "overlay" | "add";

export interface PlanMediaLayer extends PlanLayerBase {
	kind: "media";
	asset: string;
	asset_kind: "video" | "image";
	source_seconds: number[];
	crop: [number, number, number, number];
	// The crop of each frame from `first` when a crop side is animated; it then
	// replaces `crop`.
	crops?: [number, number, number, number][];
	color_matrix: number[] | null;
	// The matrix of each frame from `first` (a matrix or null) when a colour
	// property is animated; it then replaces `color_matrix` (SDD-004 section 3.5).
	color_matrices?: (number[] | null)[];
	// The advanced colour of SDD-004 section 3.14, constant over the item.
	grade?: PlanGrade;
	// The lookup table of SDD-004 section 3.6; absent when its intensity is 0.
	lut?: PlanLut;
	// The filter of SDD-004 section 3.6 as a 3x4 matrix, and the blur as the
	// standard deviation in project pixels; both constant over the item.
	filter?: number[] | null;
	blur?: number;
	// The key of SDD-004 section 3.7, constant over the item; absent without one.
	chroma_key?: PlanChromaKey;
}

export type PlanHslRange =
	| "red"
	| "orange"
	| "yellow"
	| "green"
	| "cyan"
	| "blue"
	| "purple"
	| "magenta";

// The parameters are -1 to 1; the plan leaves out a 0.
export interface PlanHslBand {
	range: PlanHslRange;
	hue?: number;
	saturation?: number;
	lightness?: number;
}

export interface PlanWheel {
	// Degrees, 0 red, 120 green, 240 blue.
	hue: number;
	amount: number;
	luma: number;
}

export interface PlanCurvePoint {
	x: number;
	y: number;
}

export interface PlanGrade {
	hsl?: PlanHslBand[];
	wheels?: {
		shadows?: PlanWheel;
		midtones?: PlanWheel;
		highlights?: PlanWheel;
	};
	curves?: {
		master?: PlanCurvePoint[];
		red?: PlanCurvePoint[];
		green?: PlanCurvePoint[];
		blue?: PlanCurvePoint[];
	};
}

export interface PlanLut {
	asset: string;
	intensity: number;
}

export interface PlanChromaKey {
	color: string;
	similarity: number;
	smoothness: number;
	spill: number;
}

export interface PlanBitmapLayer extends PlanLayerBase {
	kind: "bitmap";
	bitmaps: { keys: string[]; index: number[] };
	natural: [number, number];
}

export type PlanLayer = PlanMediaLayer | PlanBitmapLayer;

// A composition drawn as one unit (SDD-004 section 3.16, SDD-013 section 3.12):
// the placement and look of a media layer of a picture of `group.width` by
// `group.height`, whose `crop` is the whole nested frame, and the layers of
// that frame, bottom first, with the series of the frames of the window.
export interface PlanGroupLayer extends PlanLayerBase {
	kind: "group";
	crop: [number, number, number, number];
	group: {
		width: number;
		height: number;
		layers: (PlanLayer | PlanGroupLayer)[];
	};
}

// What a window's `layers` holds: the layers an executor draws itself and the
// groups it draws through a surface of their own.
export type PlanWindowLayer = PlanLayer | PlanGroupLayer;

export interface PlanAudio {
	// As on a layer: the timeline that holds `asset` when the sound comes from a composition.
	timeline?: string;
	item: string;
	asset: string;
	first: number;
	last: number;
	// at the entry's first frame (SDD-004 section 10); see audioSourceSeconds
	source_seconds: number;
	// The mean speed when `speed_curve` is set (SDD-004 section 3.11).
	speed: number;
	speed_curve?: PlanSpeedRamp;
	// The item reads its source backwards (SDD-004 section 3.12).
	reverse?: boolean;
	gain: PerFrame;
	// The file of the item's processed sound (SDD-004 section 7.1): its key, the
	// timeline frame of its first sample and its length in frames.
	processed?: { key: string; start: number; frames: number };
}

// The ramp of a speed curve as the plan carries it: the points (`t` a fraction
// of the item, `speed` a fraction), the item's length in frames and the item
// frame at which the window starts.
export interface PlanSpeedRamp {
	points: { t: number; speed: { num: number; den: number } }[];
	item_frames: number;
	first_frame: number;
}

// GET /projects/{id}/plan (SDD-004 section 10).
export interface PlanWindow extends PlanWindowHead {
	width: number;
	height: number;
	background: string;
	// True when settings.background is "transparent": `background` is then
	// #000000 and uncovered pixels stay clear (SDD-001).
	transparent?: boolean;
	safe_area: [number, number, number, number];
	layers: PlanWindowLayer[];
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
	| "waiting_approval"
	| "queued"
	| "waiting_approval"
	| "running"
	| "succeeded"
	| "failed"
	| "cancelled";

export interface ExportResult {
	revision: number;
	format?: string;
	path?: string;
	download_url: string;
	bytes: number;
	sha256?: string;
	duration: TimeValue;
	width: number;
	height: number;
	// The preset the export was made for, and the codec the file holds. When the
	// preset wanted a codec this machine cannot write, codec_fallback is the
	// codec it wanted and notes say so (SDD-004 section 8.3).
	preset?: string;
	video_codec?: string;
	codec_fallback?: string;
	notes?: string[];
}

// One export of a batch, read from its child job (SDD-004 section 8.4).
export interface BatchItem {
	index: number;
	preset?: string;
	format: string;
	codec?: string;
	job_id?: string;
	state: JobState;
	progress: number;
	result?: ExportResult;
	error?: ApiErrorBody;
}

export interface BatchResult {
	total: number;
	succeeded: number;
	failed: number;
	cancelled: number;
	items: BatchItem[];
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
		batch?: BatchResult;
		transcribe?: { asset_id: string; language: string; words: number };
		generated?: GeneratedResult;
		publish?: PublishResult;
	};
	error?: ApiErrorBody;
	lint?: { errors: number; warnings: number };
	// A job of a kind that needs approval carries its approval request and, from the moment it exists, the
	// estimate the service made (SDD-016 section 6).
	approval_id?: string;
	estimate?: Money;
	actor: string;
	created_at: string;
	text: string;
}

// The files an image.generate or voice.generate job stored as assets (SDD-016 section 6.1).
export interface GeneratedItem {
	asset_id: string;
	hash: string;
	kind: string;
	bytes: number;
	state: string;
	revised_prompt?: string;
}

export interface GeneratedResult {
	provider: string;
	model: string;
	items: GeneratedItem[];
	requested: number;
	delivered: number;
	notes?: string[];
	cost: Money;
}

// The bodies of POST /image/generate and POST /voice/generate.
export interface ImageGenerateRequest {
	project_id: string;
	connection_id: string;
	model: string;
	prompt: string;
	size?: string;
	count?: number;
	wait_ms?: number;
	dry_run?: boolean;
}

export interface VoiceGenerateRequest {
	project_id: string;
	connection_id: string;
	model: string;
	text: string;
	voice: string;
	language?: string;
	format?: string;
	wait_ms?: number;
	dry_run?: boolean;
}

// The body of POST /video/generate (SDD-016 section 6.4).
export interface VideoGenerateRequest {
	project_id: string;
	connection_id: string;
	model: string;
	prompt: string;
	seconds?: number;
	aspect?: string;
	first_frame?: string;
	wait_ms?: number;
	dry_run?: boolean;
}

export interface VoiceView {
	id: string;
	name: string;
	language?: string;
	source: "provider" | "documented";
}

export interface VoiceListResult {
	connection_id: string;
	provider: string;
	voices: VoiceView[];
}

export interface JobListResult {
	project_id: string;
	jobs: Job[];
	next_cursor: string | null;
	text: string;
}

// GET /projects/{id}/describe (SDD-003): the items of a production; the editor reads the timelines of it, to name
// the timeline a composition uses (SDD-001 section 3.5.1).
export interface ProjectDescribeResult {
	project_id: string;
	items: {
		id: string;
		kind: string;
		title: string;
		timeline_id?: string;
		state: string;
	}[];
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

// Revert, the diff of two revisions and checkpoints (SDD-003 section 2.5.1). The
// core classifies and words every change: `line` is shown as it comes.
export interface Checkpoint {
	name: string;
	note: string;
	revision: number;
	actor: string;
	at: string;
	auto: boolean;
}

export interface CheckpointListResult {
	project_id: string;
	revision: number;
	checkpoints: Checkpoint[];
	text: string;
}

export interface DiffEntry {
	verb: "added" | "removed" | "moved" | "trimmed" | "changed";
	noun: string;
	line: string;
}

export interface HistoryDiffResult {
	project_id: string;
	revision: number;
	from: number;
	to: number;
	count: number;
	entries: DiffEntry[];
	text: string;
}

export interface HistoryRevertResult extends Omit<EditResult, "kind"> {
	kind: "revert";
	target: number;
	same: boolean;
	text: string;
}

export interface RevertRequest {
	revision?: number;
	checkpoint?: string;
	// The revision the caller read; a change by another actor since then to what
	// the revert changes is E_REVISION_CONFLICT (SDD-002 A.7).
	base_revision?: number;
	dry_run?: boolean;
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

// A font file of the font store, named by the hash `font.add` takes (VV-094).
export interface ImportedFont {
	hash: string;
	bytes: number;
}

export interface FontListResult {
	fonts: FontFamily[];
	imported: ImportedFont[];
}

// GET /assets: one file of the workspace by content (VV-117).
export interface LibraryAsset {
	hash: string;
	name: string;
	kind: string;
	bytes: number;
	width?: number;
	height?: number;
	duration_ms?: number;
	origin?: AssetOrigin;
	description?: string;
	reusable?: boolean;
	tags?: string[];
	created_by?: string;
	created_at?: string;
	projects: number;
	// Where a picture of the file is served from: a timeline that holds it and its id there.
	poster?: AssetPoster;
	uses?: AssetUse[];
}

export type OriginKind =
	| "imported"
	| "generated"
	| "fetched_by_agent"
	| "rendered_externally";

// SDD-011 section 14.2. source, prompt and rights_note are untrusted text.
export interface AssetOrigin {
	kind: OriginKind;
	source?: string;
	provider?: string;
	model?: string;
	prompt?: string;
	cost_micros?: number;
	rights_note?: string;
}

export interface AssetPoster {
	timeline_id: string;
	asset_id: string;
}

export interface AssetUse {
	project_id: string;
	name: string;
	timelines: string[];
	asset_id: string;
}

export interface AssetListResult {
	assets: LibraryAsset[];
	total: number;
	next_cursor: string | null;
	text: string;
}

// The filters of GET /assets (SDD-011 section 14.4).
export interface AssetQuery {
	section?: "visual" | "video";
	kind?: "image" | "video" | "audio" | "lut";
	tag?: string;
	origin?: OriginKind;
	reusable?: boolean;
	text?: string;
	project?: string;
	cursor?: string;
	limit?: number;
}

// GET /assets/{hash}, PUT /assets/{hash} and POST /assets/{hash}/tags answer one entry.
export type AssetResult = LibraryAsset & { text: string };

export interface AssetInfoBody {
	description?: string;
	rights_note?: string;
	reusable?: boolean;
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

// /connections (SDD-014): the destinations the user connected. A result never carries a
// credential, only secret_set.
export interface Connection {
	id: string;
	kind: string;
	label: string;
	endpoint: string;
	hosts: string[];
	accepts: string[];
	created_at: string;
	last_used_at: string | null;
	secret_set: boolean;
}

export interface ConnectionListResult {
	connections: Connection[];
}

export interface ConnectionCreateRequest {
	kind: string;
	label: string;
	endpoint: string;
	hosts?: string[];
	accepts: string[];
	secret: string;
}

export interface ConnectionTestResult {
	id: string;
	ok: boolean;
	status: number;
	message: string;
}

// The AI providers (SDD-016): the providers, the models each connection offers, and the price table a
// person edits. A result never carries a key. An amount is an integer in millionths of the currency
// unit for a million tokens; the page formats it and decides nothing.
export interface ProviderPrice {
	currency: string;
	input_micros_per_mtok: number;
	output_micros_per_mtok: number;
	// image, mchar or second for the price of one image, of a million characters of speech or of one second
	// of video; absent for tokens.
	unit?: "image" | "mchar" | "second";
	unit_micros?: number;
}

export interface PriceEntry extends ProviderPrice {
	provider: string;
	model: string;
}

export interface ProviderView {
	id: string;
	name: string;
	capabilities: string[];
	credential_kinds: string[];
	default_endpoint: string;
	docs_url: string;
	connections: string[];
}

export interface ProviderListResult {
	providers: ProviderView[];
	prices: PriceEntry[];
}

export type PriceSource = "user" | "catalogue" | "none";

export interface ModelView {
	id: string;
	name: string;
	context_tokens: number;
	output_tokens: number;
	price: ProviderPrice | null;
	price_source: PriceSource;
	// A hint of the adapter, never a rule: it orders what a picker shows.
	capability?: "text" | "image" | "voice" | "video";
}

export interface ProviderModelsResult {
	connection_id: string;
	provider: string;
	models: ModelView[];
	fetched_at: string;
	cached: boolean;
}

export interface ProviderPriceSetRequest {
	provider: string;
	model: string;
	currency?: string;
	input_micros_per_mtok?: number;
	output_micros_per_mtok?: number;
	unit?: "image" | "mchar" | "second";
	unit_micros?: number;
}

export interface ProviderPricesResult {
	prices: PriceEntry[];
}

// The sign-in to an AI provider in the browser (SDD-016 section 12). A result never carries a token.
export interface ProviderSignInRequest {
	provider: string;
	label?: string;
	connection_id?: string;
	redirect_uri: string;
}

export interface ProviderSignInResult {
	connection_id: string;
	authorize_url: string;
}

export interface ProviderSignInCompleteRequest {
	state: string;
	code?: string;
	error?: string;
	client_id?: string;
}

export interface ProviderSignIn {
	connection_id: string;
	provider: string;
	label: string;
	account?: string;
	state: "ready" | "needs_sign_in";
}

export interface ProviderSignInListResult {
	sign_ins: ProviderSignIn[];
}

// The approval queue, its rules and the cost ceilings (SDD-015). An amount is an integer in
// millionths of the currency unit; the page formats it and adds nothing.
export interface Money {
	micros: number;
	currency: string;
}

export type ApprovalState = "pending" | "approved" | "rejected" | "expired";

export interface ApprovalRequest {
	id: string;
	actor: string;
	kind: string;
	// Text an agent or a job kind wrote: shown as text, never as markup.
	summary: string;
	connection_id: string | null;
	project_id: string | null;
	job_id: string | null;
	payload_ref: string;
	estimate: Money;
	state: ApprovalState;
	decided_by: string | null;
	rule_id: string | null;
	reason?: string;
	created_at: string;
	decided_at: string | null;
	expires_at: string;
}

export interface ApprovalListResult {
	requests: ApprovalRequest[];
	next_cursor: string | null;
	total: number;
	text: string;
}

export interface ApprovalRule {
	id: string;
	kind: string;
	connection_id: string | null;
	project_id: string | null;
	currency?: string;
	per_item_micros: number | null;
	per_day_micros: number | null;
	enabled: boolean;
	created_by: string;
	created_at: string;
}

export interface ApprovalRuleListResult {
	rules: ApprovalRule[];
}

export interface ApprovalRuleSetRequest {
	id?: string;
	kind: string;
	connection_id?: string;
	project_id?: string;
	currency?: string;
	per_item_micros?: number;
	per_day_micros?: number;
	enabled?: boolean;
}

export interface CostCeiling {
	project_id: string | null;
	currency: string;
	per_item_micros: number | null;
	per_day_micros: number | null;
	updated_by: string;
	updated_at: string;
}

export interface CostLine {
	scope: "workspace" | "project";
	project_id?: string;
	currency: string;
	micros: number;
}

export interface CostTotal {
	day: string;
	connection_id?: string;
	project_id?: string;
	currency: string;
	micros: number;
}

export interface CostSummary {
	ceilings: CostCeiling[];
	today: CostLine[];
	day: string;
	days: CostTotal[];
}

export interface CostCeilingSetRequest {
	project_id?: string;
	currency: string;
	per_item_micros?: number;
	per_day_micros?: number;
}

// The events of GET /approvals/events: the data of each is an ApprovalRequest.
export const APPROVAL_EVENT_NAMES = [
	"approval.requested",
	"approval.decided",
] as const;

// The event of GET /publications/events: the data is a publication with its targets (SDD-020 section 29).
export const PUBLICATION_EVENT_NAMES = ["publication.updated"] as const;

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
	// True when an import found the same content already stored (VV-128).
	deduplicated?: boolean;
	text: string;
}

export interface TemplateSaveRequest {
	project_id: string;
	name: string;
	description?: string;
}

export interface TemplateInstantiateRequest {
	name: string;
	fill?: { [slot: string]: { asset_id: string } | { text: string } };
	// Names of template colours that take the brand kit colour, and kit families
	// added to the fonts of the project (SDD-005 section 5.11).
	brand_colors?: string[];
	brand_fonts?: string[];
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

// POST /projects.
export interface ProjectCreateResult {
	project_id: string;
	name: string;
	text: string;
}

// An item of a project as a production (SDD-005 section 16.1): the commands and the answers are the service's.
export type ProjectItemKind = "brief" | "text" | "timeline" | "output";

export interface ProjectItem {
	id: string;
	kind: ProjectItemKind;
	title: string;
	position: number;
	state: "active" | "trashed";
	timeline_id?: string;
	job_id?: string;
	format?: string;
	text?: string;
	// A text item: script, copy or caption (SDD-019).
	text_kind?: "script" | "copy" | "caption";
	updated_at: string;
}

export interface ProjectItemResult {
	project_id: string;
	item: ProjectItem;
	text: string;
}

export interface ProjectItemAddRequest {
	kind: "text" | "timeline";
	title: string;
	text?: string;
	preset?: string;
}

export interface BriefResult {
	project_id: string;
	item_id?: string;
	text: string;
}

// The guided briefing (SDD-005 section 5.12.1): the questions are the core's, the page only draws them.
export interface BriefingQuestion {
	id: string;
	heading: string;
	text: string;
	kind: "text" | "choice" | "seconds" | "date";
	required: boolean;
	options?: string[];
	max_chars?: number;
	min?: number;
	max?: number;
}

export interface BriefingAnswer {
	id: string;
	value: string;
}

export interface BriefingQuestionsResult {
	project_id?: string;
	questions: BriefingQuestion[];
	has_brief: boolean;
	guided: boolean;
	answers?: BriefingAnswer[];
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

// The optional Google sign-in of network mode (SDD-007 section 4.7, SDD-014 section 12.4).
export interface GoogleLinked {
	email: string;
	linked_ms: number;
}

export interface GoogleStatus {
	enabled: boolean;
	linked: GoogleLinked | null;
}

export interface GoogleStart {
	authorize_url: string;
}

export interface GoogleComplete {
	user?: SessionResult;
	linked?: GoogleLinked;
}

export interface GoogleSettingsRequest {
	enabled: boolean;
	connection_id?: string;
	allowed_emails?: string[];
}

export interface GoogleSettings {
	enabled: boolean;
	connection_id: string;
	allowed_emails: string[];
	redirect_url: string;
}

// The brand kit and the font import (SDD-005 section 5.11, SDD-007 section 7.7).
export interface BrandFontFile {
	weight: number;
	hash: string;
}

export interface BrandFont {
	files: BrandFontFile[];
}

export interface BrandKitBody {
	logo?: string;
	colors?: { [name: string]: string };
	fonts?: { [family: string]: BrandFont };
}

export interface BrandKit extends BrandKitBody {
	updated_ms?: number;
	text: string;
}

export interface FontImportResult {
	hash: string;
	bytes: number;
	deduplicated: boolean;
	text: string;
}

// The export options of JobStart (SDD-003 section 2.8, P3-SRV-005): which of
// them the server accepts is read from its schema, never assumed.
export interface ExportOptions {
	format?: string;
	// A time with a unit (`120f`): the part of the timeline to render.
	start?: string;
	end?: string;
	// The time of the frame of a png export (`120f`).
	at?: string;
	// h264, hevc or av1, for an mp4 export; absent is the choice of the preset or H.264.
	codec?: string;
}

// One platform preset of GET /export-presets (SDD-004 section 8.2): the limits are
// the core's, the browser only shows them.
export interface ExportPreset {
	id: string;
	network: string;
	placement: string;
	label: string;
	container: string;
	codecs: string[];
	limits: {
		min_duration_ms?: number;
		max_duration_ms?: number;
		min_fps?: number;
		max_fps?: number;
		min_width?: number;
		min_height?: number;
		max_width?: number;
		max_height?: number;
		aspect_ratio?: string;
		max_bytes?: number;
		max_video_kbps?: number;
	};
	targets: {
		width: number;
		height: number;
		video_kbps: number;
		audio_kbps: number;
	};
	sources: { url: string; read: string; says: string }[];
}

export interface CodecAvailability {
	codec: string;
	available: boolean;
	encoder?: string;
	reason?: string;
}

export interface ExportPresetListResult {
	presets: ExportPreset[];
	codecs?: CodecAvailability[];
	text: string;
}

export interface ExportFinding {
	code: string;
	severity: string;
	message: string;
	limit: string;
	got: string;
}

// GET /projects/{id}/audio-measure: the sound of an item or a range, before and after its audio processors
// (SDD-004 section 7.1). A level is null when the audio has none (silence, or under 0.4 s).
export interface AudioLevels {
	integrated_lufs: number | null;
	true_peak_dbtp: number | null;
	noise_floor_dbfs: number;
	speech_to_noise_db: number | null;
}

export interface AudioMeasureResult {
	project_id: string;
	revision: number;
	item_id?: string;
	start_frame: number;
	end_frame: number;
	seconds: number;
	before: AudioLevels;
	after: AudioLevels;
	text: string;
}

export interface AudioMeasureQuery {
	itemId?: string;
	start?: string;
	end?: string;
}

// GET /projects/{id}/export-check: what does not fit a preset, with the exact limit.
export interface ExportCheckResult {
	project_id: string;
	revision: number;
	preset: string;
	ok: boolean;
	findings: ExportFinding[];
	duration_ms: number;
	estimated_bytes: number;
	text: string;
}

// One export of a batch request.
export interface ExportBatchItem {
	preset?: string;
	format?: string;
	codec?: string;
	start?: string;
	end?: string;
}

export interface ExportCapabilities {
	formats: string[];
	// The schema lists `start` and `end`, so the dialog may export a range.
	range: boolean;
}

// POST /projects/import answers 201 with the project it made.
export interface BundleImportResult {
	project_id: string;
	name: string;
	revision: number;
	text: string;
}

// The hosted agent chat (SDD-017). Every text in these types was written by an agent or by a
// person and is shown as text, never as markup.
export type ChatState =
	| "starting"
	| "idle"
	| "running"
	| "awaiting_permission"
	| "ended";

export interface ChatSession {
	id: string;
	project_id: string;
	profile: string;
	state: ChatState;
	end_reason?: string;
	title: string;
	created_at: string;
	updated_at: string;
	ended_at?: string;
	event_count: number;
	live: boolean;
}

export interface ChatToolCall {
	tool_call_id: string;
	title: string;
	tool_kind?: string;
	status: "pending" | "in_progress" | "completed" | "failed";
	output?: string;
}

export interface ChatPermissionOption {
	option_id: string;
	name: string;
	kind: "allow_once" | "allow_always" | "reject_once" | "reject_always";
}

export interface ChatPermission {
	request_id: string;
	tool_call_id?: string;
	title?: string;
	tool_kind?: string;
	options?: ChatPermissionOption[];
	answer?: "allow" | "reject" | "cancel";
}

export type ChatEventKind =
	| "user_message"
	| "agent_message"
	| "agent_thought"
	| "tool_call"
	| "permission_request"
	| "permission_answer"
	| "plan"
	| "notice"
	| "state";

// One entry of a transcript and one frame of GET /chat/sessions/{id}/events. A frame with
// `append` continues the text of the event with the same `seq`.
export interface ChatEvent {
	seq: number;
	kind: ChatEventKind;
	at: string;
	text?: string;
	append?: boolean;
	tool?: ChatToolCall;
	permission?: ChatPermission;
	state?: { state: ChatState; end_reason?: string };
}

export interface ChatTranscriptResult {
	session: ChatSession;
	events?: ChatEvent[];
	next_after?: number;
	markdown?: string;
}

export interface ChatSessionListResult {
	sessions: ChatSession[];
}

export interface AgentProfile {
	name: string;
	command: string;
	args: string[];
	created_by: string;
	created_at: string;
	updated_at: string;
}

export interface AgentSuggestion {
	name: string;
	title: string;
	executables: string[];
	args: string[];
	install: string;
	found: boolean;
	found_path?: string;
}

export interface AgentProfileListResult {
	profiles: AgentProfile[];
	suggestions: AgentSuggestion[];
	can_write: boolean;
	available: boolean;
}

// The event of GET /chat/sessions/{id}/events: the data is a ChatEvent.
export const CHAT_EVENT_NAMES = ["chat.event"] as const;

// The social networks and publishing (SDD-020). No result carries a credential.
export interface SocialLimits {
	title_max: number;
	description_max_bytes: number;
	tags_max_chars: number;
	file_max_bytes: number;
	formats: string[];
	// What the network takes besides the video; the dialog asks only for these (SDD-020 section 18).
	thumbnail_formats?: string[];
	visibilities?: string[];
	needs_kids_declaration?: boolean;
	source: string;
	read: string;
}

export interface SocialNetwork {
	network: string;
	name: string;
	publishes: string[];
	limits: SocialLimits;
	// The network takes a flag that says the video has synthetic content.
	ai_disclosure?: boolean;
	// What a post costs on a network that charges for one (X), as the service has it.
	price?: SocialPrice;
	// The network takes the file as a draft to be finished in its own app (TikTok): it ends sent_as_draft.
	draft?: boolean;
	endpoint: string;
	hosts: string[];
	scopes: string[];
	guide: string;
	guide_read: string;
}

// Amounts in millionths of the currency, with the page they were read from, the date, and the date a person
// edited them (SDD-020 section 35).
export interface SocialPrice {
	currency: string;
	post_micros: number;
	link_micros: number;
	source: string;
	read: string;
	edited?: string;
}

export interface SocialPriceRequest {
	connection_id: string;
	post_micros: number;
	link_micros: number;
}

export interface SocialNetworkListResult {
	networks: SocialNetwork[];
	redirect_path: string;
}

export interface SocialConnectRequest {
	network: string;
	label?: string;
	client_id?: string;
	client_secret?: string;
	connection_id?: string;
	redirect_uri: string;
}

export interface SocialConnectResult {
	connection_id: string;
	authorize_url: string;
}

export interface SocialCompleteRequest {
	state: string;
	code?: string;
	error?: string;
}

export interface SocialAccount {
	connection_id: string;
	network: string;
	label: string;
	account_id?: string;
	account_title?: string;
	// needs_account: the sign-in reached several accounts and a person has not chosen one (SDD-020 section 17).
	state: "ready" | "needs_sign_in" | "needs_account";
	choices?: SocialChoice[];
	price?: SocialPrice;
}

export interface SocialChoice {
	id: string;
	title: string;
}

export interface SocialSelectRequest {
	connection_id: string;
	account_id: string;
}

export interface SocialAccountListResult {
	accounts: SocialAccount[];
}

export interface PublishRequest {
	project_id: string;
	connection_id: string;
	export: string;
	thumbnail?: string;
	title: string;
	description?: string;
	tags?: string[];
	visibility?: string;
	made_for_kids?: boolean;
	ai_disclosure?: boolean;
}

export interface PublishIssue {
	path: string;
	code: string;
	message: string;
}

export interface PublishValidateResult {
	network: string;
	state: "prepared" | "invalid";
	issues: PublishIssue[];
	summary?: string;
}

export interface PublishResult {
	network: string;
	connection_id: string;
	remote_id: string;
	url: string;
	state: "processing" | "published";
	thumbnail_error?: string;
}

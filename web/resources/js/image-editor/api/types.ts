import type { Checkpoint, DiffEntry } from "../../editor/api/types";

// The shapes of the image operations of SDD-021 as the HTTP API answers them. They mirror the Go types of
// `service` (image_types.go, image_view.go) and `core/image` (document.go); the server wins on any difference.

export type LayerKind = "image" | "text" | "shape" | "fill" | "group";

export interface Crop {
	x: number;
	y: number;
	w: number;
	h: number;
}

// The text style keys of a text layer (the same as a video text item).
export interface TextStyle {
	font_family?: string;
	font_weight?: number;
	size?: number;
	color?: string;
	stroke_color?: string;
	stroke_width?: number;
	background_color?: string;
	align?: "left" | "center" | "right";
	line_height?: number;
	uppercase?: boolean;
	[key: string]: unknown;
}

export interface CurvePoint {
	x: number;
	y: number;
}

// The adjustments of a layer that differ from neutral (SDD-021 section 3.1): whole numbers, so the server is
// the only place that turns them into pixels.
export interface Adjust {
	brightness?: number;
	contrast?: number;
	saturation?: number;
	temperature?: number;
	tint?: number;
	hue?: number;
	exposure?: number;
	levels?: { black: number; white: number; gamma: number };
	curve?: CurvePoint[];
	// The colour lookup table of the layer: the content hash of a lut file and the per cent of it mixed in.
	lut?: { asset: string; intensity: number };
}

export type FilterKind =
	| "grayscale"
	| "sepia"
	| "invert"
	| "warm"
	| "cool"
	| "blur"
	| "sharpen"
	| "vignette"
	| "grain"
	| "shadow"
	| "glow"
	| "stroke";

// A filter of a layer: its kind and every parameter of the kind (SDD-021 section 3.1).
export interface Filter {
	kind: FilterKind;
	amount?: number;
	radius?: number;
	softness?: number;
	size?: number;
	seed?: number;
	opacity?: number;
	dx?: number;
	dy?: number;
	blur?: number;
	width?: number;
	color?: string;
}

export type MaskKind = "rect" | "rounded_rect" | "ellipse" | "layer";

// The mask of a layer: a shape in thousandths of its box, or the alpha of the layer named by `source`.
export interface Mask {
	kind: MaskKind;
	x?: number;
	y?: number;
	w?: number;
	h?: number;
	radius?: number;
	feather?: number;
	invert?: boolean;
	source?: string;
}

export interface Layer {
	id: string;
	kind: LayerKind;
	name: string;
	visible: boolean;
	locked?: boolean;
	opacity: number;
	x: number;
	y: number;
	w: number;
	h: number;
	rotation?: number;
	slot?: string;
	asset?: string;
	crop?: Crop;
	text?: string;
	style?: TextStyle;
	shape?: "rect" | "rounded_rect" | "ellipse";
	fill?: string;
	stroke_color?: string;
	stroke_width?: number;
	radius?: number;
	adjust?: Adjust;
	filters?: Filter[];
	mask?: Mask;
	blend?: string;
	children?: Layer[];
}

export interface Finding {
	code: string;
	severity: "error" | "warning";
	item?: string;
	message: string;
	hint: string;
}

export interface LintTotals {
	errors: number;
	warnings: number;
}

export interface ImageDocument {
	image_id: string;
	revision: number;
	name: string;
	width: number;
	height: number;
	background: string;
	template: boolean;
	layers: Layer[];
	findings: Finding[];
	lint: LintTotals;
	text: string;
}

export interface ImageSummary {
	image_id: string;
	name: string;
	template: boolean;
	revision: number;
	width: number;
	height: number;
	layers: number;
	updated_ms: number;
}

export interface ImageListResult {
	images: ImageSummary[];
	next_cursor: string | null;
}

export interface ImageCreateRequest {
	name?: string;
	preset?: string;
	width?: number;
	height?: number;
	background?: string;
	template?: boolean;
	template_id?: string;
	values?: { [slot: string]: { text?: string; asset?: string } };
}

export interface ImageCreateResult {
	image_id: string;
	revision: number;
	width: number;
	height: number;
}

// POST /images/import (SDD-021 section 10.1): a file of the server (`path`) or of the workspace (`hash`) becomes
// a new document, or one more layer of `image_id`.
export interface ImageImportRequest {
	path?: string;
	hash?: string;
	image_id?: string;
	name?: string;
	preset?: string;
	width?: number;
	height?: number;
	wait_ms?: number;
	base_revision?: number;
}

export interface ImageImportResult {
	state: "ready" | "pending";
	image_id?: string;
	revision?: number;
	created: boolean;
	layer_id?: string;
	hash?: string;
	name: string;
	width?: number;
	height?: number;
	job_id?: string;
	applied: boolean;
	text: string;
}

// The events of GET /images/{id}/events (SDD-006 section 12). The diff is the one of the batch, or, past 64 KiB,
// only the objects that changed.
export interface ImageUpdatedEvent {
	name: "image.updated";
	image_id: string;
	revision: number;
	base_revision: number;
	kind: "edit" | "undo" | "redo";
	actor: string;
	at: string;
	diff?: { kind: string; id: string }[];
	diff_truncated?: boolean;
	changed?: { kind: string; id: string }[];
}

export interface ImageResyncEvent {
	name: "resync.required";
	image_id: string;
	revision: number;
	reason: "gap_too_large" | "client_ahead";
}

export type ImageStreamEvent = ImageUpdatedEvent | ImageResyncEvent;

export interface ImageOperation {
	op: string;
	args?: { [key: string]: unknown };
}

export interface Skipped {
	primitive_index: number;
	reason: string;
	fields?: string[];
	by_revision?: number;
	by_actor?: string;
}

export interface ImageEditResult {
	image_id: string;
	revision: number;
	base_revision: number;
	applied: boolean;
	actor: string;
	created: string[];
	findings: Finding[];
	lint: LintTotals;
	description: string;
	skipped?: Skipped[];
	kind?: "undo" | "redo";
	step?: number;
}

export interface ImageExportOptions {
	format?: "png" | "jpeg" | "webp";
	quality?: number;
	preset?: string;
}

export interface ImageExportResult {
	image_id: string;
	revision: number;
	format: string;
	path: string;
	bytes: number;
	sha256: string;
	width: number;
	height: number;
	text: string;
}

export interface ImageHistoryEntry {
	revision: number;
	kind: "edit" | "undo" | "redo";
	actor: string;
	at: string;
	ops: string[];
	changes: number;
	// What the step did, in words, from the core (SDD-021 section 4).
	description?: string;
	undo_of?: number;
	undone_by?: number;
	skipped?: number;
	mine: boolean;
}

export interface ImageHistoryResult {
	image_id: string;
	revision: number;
	entries: ImageHistoryEntry[];
}

// The revert, the diff and the named versions of an image (SDD-003 section 2.5.1, SDD-021 section 4): the shapes
// of the timeline's, with the id of the image.
export interface ImageCheckpointListResult {
	image_id: string;
	revision: number;
	checkpoints: Checkpoint[];
	text: string;
}

export interface ImageDiffResult {
	image_id: string;
	revision: number;
	from: number;
	to: number;
	count: number;
	entries: DiffEntry[];
	text: string;
}

export interface ImageRevertRequest {
	revision?: number;
	checkpoint?: string;
	// The revision the caller read; a change by another actor since then to a layer the revert changes is
	// E_REVISION_CONFLICT (SDD-002 A.7).
	base_revision?: number;
	dry_run?: boolean;
}

export interface ImageRevertResult
	extends Omit<ImageEditResult, "kind" | "step"> {
	kind: "revert";
	target: number;
	same: boolean;
	dry_run: boolean;
	text: string;
}

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

import { callJson, type Transport } from "@/lib/api";
import type { Checkpoint } from "../../editor/api/types";
import type {
	ImageCheckpointListResult,
	ImageCreateRequest,
	ImageCreateResult,
	ImageDiffResult,
	ImageDocument,
	ImageEditResult,
	ImageExportOptions,
	ImageExportResult,
	ImageHistoryResult,
	ImageImportRequest,
	ImageImportResult,
	ImageListResult,
	ImageOperation,
	ImageRevertRequest,
	ImageRevertResult,
} from "./types";

// Timeouts of SDD-012 section 4.3 item 4. An export renders, so it gets the longest.
const READ_TIMEOUT_MS = 10_000;
const EDIT_TIMEOUT_MS = 30_000;
const EXPORT_TIMEOUT_MS = 60_000;
// An import waits for the ingest of the file, up to 25 seconds on the server (SDD-021 section 10.1).
const IMPORT_TIMEOUT_MS = 40_000;

export const API_BASE = "/api/v1";

// One typed function per call of the image editor (SDD-012 section 24.3). The only place of the image editor
// that reaches the network, through the Transport of the video editor.
export interface ImageApi {
	list(templates?: "only" | "none"): Promise<ImageListResult>;
	create(request: ImageCreateRequest): Promise<ImageCreateResult>;
	describe(id: string, signal?: AbortSignal): Promise<ImageDocument>;
	edit(
		id: string,
		baseRevision: number,
		operations: readonly ImageOperation[],
	): Promise<ImageEditResult>;
	undo(id: string): Promise<ImageEditResult>;
	redo(id: string): Promise<ImageEditResult>;
	exportImage(
		id: string,
		options: ImageExportOptions,
	): Promise<ImageExportResult>;
	history(id: string, limit?: number): Promise<ImageHistoryResult>;
	// Revert, the diff and the named versions (epic 50): the core says what changes, in words; a revert with
	// `dry_run` returns the diff it would make and writes nothing.
	checkpoints(id: string): Promise<ImageCheckpointListResult>;
	createCheckpoint(id: string, name: string): Promise<Checkpoint>;
	removeCheckpoint(id: string, name: string): Promise<Checkpoint>;
	diff(id: string, from: number, until?: number): Promise<ImageDiffResult>;
	revert(id: string, request: ImageRevertRequest): Promise<ImageRevertResult>;
	importImage(request: ImageImportRequest): Promise<ImageImportResult>;
	// A file chosen in the browser, sent as the body of POST /images/import/file (SDD-021 section 10.2); `file.name`
	// is the label it travels under, and the members of the request are the query.
	importFile(
		file: File,
		request: Omit<ImageImportRequest, "path" | "hash">,
	): Promise<ImageImportResult>;
	// Open or create a standalone image document from an existing workspace asset hash (epic 105).
	openFromAsset(
		assetHash: string,
		options?: { name?: string; preset?: string },
	): Promise<ImageImportResult>;
	// Active production projects the user can attach an exported image to (epic 105).
	listProjects(): Promise<{ projects: { id: string; name: string }[] }>;
	// Import an exported image file into a project's media library (epic 105).
	attachExportToProject(projectId: string, path: string): Promise<unknown>;
	// The HTTP status of GET /images/{id}, or 0 when the network failed: the probe of the event stream.
	probe(id: string): Promise<number>;
}

export function createImageApi(transport: Transport): ImageApi {
	async function call<T>(
		method: "GET" | "POST" | "DELETE",
		path: string,
		timeoutMs: number,
		body?: unknown,
		signal?: AbortSignal,
		file?: Blob,
	): Promise<T> {
		return callJson<T>(
			transport,
			{ method, path, body, file, timeoutMs, signal },
			{ retry: false },
		);
	}

	const image = (id: string): string => `/images/${encodeURIComponent(id)}`;

	return {
		list: (templates) =>
			call(
				"GET",
				`/images${templates === undefined ? "" : `?templates=${templates}`}`,
				READ_TIMEOUT_MS,
			),
		create: (request) => call("POST", "/images", EDIT_TIMEOUT_MS, request),
		describe: (id, signal) =>
			call("GET", image(id), READ_TIMEOUT_MS, undefined, signal),
		edit: (id, baseRevision, operations) =>
			call("POST", `${image(id)}/edit`, EDIT_TIMEOUT_MS, {
				base_revision: baseRevision,
				operations,
			}),
		undo: (id) => call("POST", `${image(id)}/undo`, EDIT_TIMEOUT_MS, {}),
		redo: (id) => call("POST", `${image(id)}/redo`, EDIT_TIMEOUT_MS, {}),
		exportImage: (id, options) =>
			call("POST", `${image(id)}/export`, EXPORT_TIMEOUT_MS, options),
		history: (id, limit) =>
			call(
				"GET",
				`${image(id)}/history${limit === undefined ? "" : `?limit=${limit}`}`,
				READ_TIMEOUT_MS,
			),
		checkpoints: (id) =>
			call("GET", `${image(id)}/checkpoints`, READ_TIMEOUT_MS),
		createCheckpoint: (id, name) =>
			call("POST", `${image(id)}/checkpoints`, EDIT_TIMEOUT_MS, { name }),
		removeCheckpoint: (id, name) =>
			call(
				"DELETE",
				`${image(id)}/checkpoints/${encodeURIComponent(name)}`,
				EDIT_TIMEOUT_MS,
			),
		diff: (id, from, until) =>
			call(
				"GET",
				`${image(id)}/diff?from=${from}${until === undefined ? "" : `&until=${until}`}`,
				READ_TIMEOUT_MS,
			),
		revert: (id, request) =>
			call("POST", `${image(id)}/revert`, EDIT_TIMEOUT_MS, request),
		importImage: (request) =>
			call("POST", "/images/import", IMPORT_TIMEOUT_MS, request),
		importFile: (file, request) => {
			const query = new URLSearchParams({ file_name: file.name });
			for (const [key, value] of Object.entries(request)) {
				if (value !== undefined && value !== "") query.set(key, String(value));
			}
			return call(
				"POST",
				`/images/import/file?${query.toString()}`,
				IMPORT_TIMEOUT_MS,
				undefined,
				undefined,
				file,
			);
		},
		openFromAsset: (assetHash, options) =>
			call("POST", "/images/from-asset", IMPORT_TIMEOUT_MS, {
				asset_hash: assetHash,
				...(options?.name === undefined ? {} : { name: options.name }),
				...(options?.preset === undefined ? {} : { preset: options.preset }),
			}),
		listProjects: () => call("GET", "/projects", READ_TIMEOUT_MS),
		attachExportToProject: (projectId, path) =>
			call(
				"POST",
				`/projects/${encodeURIComponent(projectId)}/media`,
				IMPORT_TIMEOUT_MS,
				{ path },
			),
		async probe(id) {
			try {
				const response = await transport.send({
					method: "GET",
					path: image(id),
					timeoutMs: READ_TIMEOUT_MS,
				});
				return response.status;
			} catch {
				return 0;
			}
		},
	};
}

// The picture of a document: an `img` source, not a fetch. `revision` is only a cache key, so a new revision
// loads a new picture; the width asks the server to draw at the size the canvas shows.
export function frameUrl(id: string, revision: number, width: number): string {
	return `${API_BASE}/images/${encodeURIComponent(id)}/frame?width=${width}&rev=${revision}`;
}

// A file that an export wrote, as a link the browser downloads.
export function fileUrl(id: string, path: string): string {
	const name = path.split(/[/\\]/).pop() ?? path;
	return `${API_BASE}/images/${encodeURIComponent(id)}/exports/${encodeURIComponent(name)}`;
}

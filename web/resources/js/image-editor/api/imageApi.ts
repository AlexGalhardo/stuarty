import { fromResponse, networkError } from "../../editor/api/errors";
import { type Transport, TransportError } from "../../editor/api/transport";
import type {
	ImageCreateRequest,
	ImageCreateResult,
	ImageDocument,
	ImageEditResult,
	ImageExportOptions,
	ImageExportResult,
	ImageHistoryResult,
	ImageListResult,
	ImageOperation,
} from "./types";

// Timeouts of SDD-012 section 4.3 item 4. An export renders, so it gets the longest.
const READ_TIMEOUT_MS = 10_000;
const EDIT_TIMEOUT_MS = 30_000;
const EXPORT_TIMEOUT_MS = 60_000;

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
}

export function createImageApi(transport: Transport): ImageApi {
	async function call<T>(
		method: "GET" | "POST",
		path: string,
		timeoutMs: number,
		body?: unknown,
		signal?: AbortSignal,
	): Promise<T> {
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

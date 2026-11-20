// Binary URL builders: strings only, no call (SDD-012 section 4.2). All are
// served with HTTP Range and content-hash caching.
const BASE = "/api/v1";

const project = (id: string): string =>
	`${BASE}/projects/${encodeURIComponent(id)}`;

// A time argument is a string with a unit (rule T8): the frame number and "f".
export function frameUrl(
	projectId: string,
	frame: number,
	width: number,
	revision: number,
): string {
	return `${project(projectId)}/frame?at=${frame}f&width=${width}&rev=${revision}&timeline_id=${encodeURIComponent(projectId)}`;
}

// The current frame of a timeline, asked for without a revision, so the answer
// follows the edits of that timeline (the picture of a composition item).
export function currentFrameUrl(
	timelineId: string,
	frame: number,
	width: number,
): string {
	return `${project(timelineId)}/frame?at=${frame}f&width=${width}&timeline_id=${encodeURIComponent(timelineId)}`;
}

export function thumbnailUrl(
	projectId: string,
	assetId: string,
	width: 160 | 320 | 640,
): string {
	return `${project(projectId)}/media/${encodeURIComponent(assetId)}/thumbnail?width=${width}`;
}

export function proxyUrl(projectId: string, assetId: string): string {
	return `${project(projectId)}/media/${encodeURIComponent(assetId)}/proxy`;
}

export function chunksUrl(projectId: string, assetId: string): string {
	return `${project(projectId)}/media/${encodeURIComponent(assetId)}/chunks`;
}

export function exportFileUrl(jobId: string): string {
	return `${BASE}/jobs/${encodeURIComponent(jobId)}/file`;
}

export function templateFileUrl(templateId: string): string {
	return `${BASE}/templates/${encodeURIComponent(templateId)}/file`;
}

export function bitmapUrl(projectId: string, key: string): string {
	return `${project(projectId)}/bitmaps/${encodeURIComponent(key)}`;
}

// The processed sound of an item; `from` asks for the window that starts at
// that frame of the file, counted from its first (SDD-004 section 7.1).
export function audioPreviewUrl(
	projectId: string,
	key: string,
	from?: number,
): string {
	const url = `${project(projectId)}/audio-previews/${encodeURIComponent(key)}`;
	return from === undefined ? url : `${url}?from=${from}`;
}

export function stripUrl(projectId: string, assetId: string): string {
	return `${project(projectId)}/media/${encodeURIComponent(assetId)}/strip`;
}

export function peaksUrl(projectId: string, assetId: string): string {
	return `${project(projectId)}/media/${encodeURIComponent(assetId)}/peaks`;
}

export function lutUrl(projectId: string, assetId: string): string {
	return `${project(projectId)}/media/${encodeURIComponent(assetId)}/lut`;
}

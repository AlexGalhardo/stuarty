import { bitmapUrl } from "./urls";

// Rasterized text and shapes are fetched and decoded here, never shown through
// a blob: URL, so the CSP stays unchanged (SDD-012 section 2.2). The only
// place outside the client that calls fetch (rule R6).
export async function fetchBitmap(
	projectId: string,
	key: string,
	signal?: AbortSignal,
): Promise<ImageBitmap> {
	const response = await fetch(bitmapUrl(projectId, key), {
		credentials: "same-origin",
		signal,
	});
	if (!response.ok) throw new Error(`bitmap ${key}: HTTP ${response.status}`);
	return createImageBitmap(await response.blob());
}

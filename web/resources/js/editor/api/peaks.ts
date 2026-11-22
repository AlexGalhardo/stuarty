import { peaksUrl } from "./urls";

// The binary peaks file of an asset (SDD-011 section 4.6), read whole as an
// ArrayBuffer. Next to `fetchBitmap`, the only other place outside the client
// that calls fetch (rule R6).
// The server answers 409 (E_ASSET_NOT_READY) while a long sound's peaks are computed by a job (SDD-011 section 6.3.1).
export class PeaksPendingError extends Error {}

export async function fetchPeaks(
	projectId: string,
	assetId: string,
	signal?: AbortSignal,
): Promise<ArrayBuffer> {
	const response = await fetch(peaksUrl(projectId, assetId), {
		credentials: "same-origin",
		signal,
	});
	if (response.status === 409)
		throw new PeaksPendingError(`peaks ${assetId}: pending`);
	if (!response.ok)
		throw new Error(`peaks ${assetId}: HTTP ${response.status}`);
	return response.arrayBuffer();
}

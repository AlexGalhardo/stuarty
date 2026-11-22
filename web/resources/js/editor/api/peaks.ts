import { fromResponse } from "./errors";
import { peaksUrl } from "./urls";

// The server says a long sound's peaks are being computed by a job (SDD-011 section 6.3.1). Only that answer is
// worth asking again: the same 409 also means an asset whose ingest failed, which no wait will fix.
export class PeaksPendingError extends Error {}

// The binary peaks file of an asset (SDD-011 section 4.6), read whole as an
// ArrayBuffer. Next to `fetchBitmap`, the only other place outside the client
// that calls fetch (rule R6).
export async function fetchPeaks(
	projectId: string,
	assetId: string,
	signal?: AbortSignal,
): Promise<ArrayBuffer> {
	const response = await fetch(peaksUrl(projectId, assetId), {
		credentials: "same-origin",
		signal,
	});
	if (response.status === 409) {
		const body: unknown = await response.json().catch(() => null);
		if (fromResponse(409, body).details?.reason === "peaks_pending")
			throw new PeaksPendingError(`peaks ${assetId}: pending`);
	}
	if (!response.ok)
		throw new Error(`peaks ${assetId}: HTTP ${response.status}`);
	return response.arrayBuffer();
}

import type { MediaEntry } from "../api/types";

// An audio asset has no picture, so the core has no thumbnail to serve (404).
export function hasThumbnail(
	entry: Pick<MediaEntry, "kind" | "state">,
): boolean {
	return entry.state === "ready" && entry.kind !== "audio";
}

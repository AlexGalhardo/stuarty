import type { Job } from "../../editor/api/types";

// How an export is named in a list a person picks from: what it was made for (the preset, else the format), its
// size and when it was made. Never the id of its job (VV-325); the date comes already formatted.
export function exportLabel(job: Job, when: string): string {
	const made = job.result?.export;
	const kind = made?.preset ?? made?.format ?? "mp4";
	const size =
		made !== undefined && made.width > 0 && made.height > 0
			? ` · ${made.width}×${made.height}`
			: "";
	return `${kind}${size} · ${when}`;
}

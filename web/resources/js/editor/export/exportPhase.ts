import type { Job, LintFinding } from "../api/types";

export type ExportPhase =
	| "checking"
	| "ready"
	| "blocked"
	| "running"
	| "succeeded"
	| "failed"
	| "cancelled";

// The state of the export dialog (SDD-012 section 14.1), from what the core
// answered: the job when there is one, else the lint check.
export function exportPhase(input: {
	job: Job | undefined;
	checking: boolean;
	findings: readonly LintFinding[];
}): ExportPhase {
	const { job } = input;
	if (job !== undefined) {
		if (job.state === "queued" || job.state === "running") return "running";
		if (job.state === "succeeded") return "succeeded";
		if (job.state === "cancelled") return "cancelled";
		return "failed";
	}
	if (input.checking) return "checking";
	return input.findings.some((f) => f.severity === "error")
		? "blocked"
		: "ready";
}

// A finished export of an older revision is not what the editor sees any more,
// so the dialog starts over instead of reopening on that result.
export function exportIsStale(
	job: Job | undefined,
	revision: number | null,
): boolean {
	const exported =
		job?.result?.export?.revision ??
		job?.result?.batch?.items.find((item) => item.result !== undefined)?.result
			?.revision;
	return (
		job?.state === "succeeded" &&
		exported !== undefined &&
		revision !== null &&
		revision > exported
	);
}

export function groupFindings(findings: readonly LintFinding[]): {
	errors: LintFinding[];
	warnings: LintFinding[];
	notes: LintFinding[];
} {
	return {
		errors: findings.filter((f) => f.severity === "error"),
		warnings: findings.filter((f) => f.severity === "warning"),
		notes: findings.filter(
			(f) => f.severity !== "error" && f.severity !== "warning",
		),
	};
}

// Progress is announced at most every 10% (SDD-012 section 18).
export function announcedPercent(progress: number): number {
	return Math.floor(Math.min(Math.max(progress, 0), 1) * 10) * 10;
}

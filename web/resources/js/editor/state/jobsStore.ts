import type { Job } from "../api/types";
import { createStore, type ReadStore, readOnly } from "./createStore";

// Jobs of the project (ingest, transcribe, export), from `listJobs` at boot
// and `job.updated` events (SDD-012 section 14).
export interface JobsState {
	jobs: Readonly<Record<string, Job>>;
	// The export the dialog follows; survives closing the dialog.
	exportJobId: string | null;
	// The time of the last event for the export job, for the polling fallback.
	lastEventAt: number;
}

export interface JobsWriter {
	setAll(jobs: readonly Job[]): void;
	upsert(job: Job): void;
	progress(
		jobId: string,
		patch: { state: Job["state"]; progress: number; message?: string },
		now: number,
	): void;
	setExportJob(jobId: string | null): void;
}

export function createJobsStore(): {
	store: ReadStore<JobsState>;
	writer: JobsWriter;
} {
	const store = createStore<JobsState>({
		jobs: {},
		exportJobId: null,
		lastEventAt: 0,
	});
	return {
		store: readOnly(store),
		writer: {
			setAll: (list) =>
				store.setState((s) => {
					const jobs: Record<string, Job> = {};
					for (const job of list) jobs[job.job_id] = job;
					// A running batch is what the dialog follows; its children are
					// exports of their own that it does not show.
					const active = (kind: string): Job | undefined =>
						list.find(
							(j) =>
								j.kind === kind &&
								(j.state === "running" || j.state === "queued"),
						);
					const running = active("export_batch") ?? active("export");
					return {
						...s,
						jobs,
						exportJobId: s.exportJobId ?? running?.job_id ?? null,
					};
				}),
			upsert: (job) =>
				store.setState((s) => ({
					...s,
					jobs: { ...s.jobs, [job.job_id]: job },
				})),
			progress: (jobId, patch, now) =>
				store.setState((s) => {
					const job = s.jobs[jobId];
					if (job === undefined) return s;
					return {
						...s,
						jobs: { ...s.jobs, [jobId]: { ...job, ...patch } },
						lastEventAt: jobId === s.exportJobId ? now : s.lastEventAt,
					};
				}),
			setExportJob: (exportJobId) =>
				store.setState((s) => ({ ...s, exportJobId })),
		},
	};
}

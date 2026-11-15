import { router } from "@inertiajs/react";
import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { Job } from "../../editor/api/types";
import { exportFileUrl } from "../../editor/api/urls";

const POLL_MS = 1000;
const POLL_LIMIT = 600;

const wait = (ms: number): Promise<void> =>
	new Promise((resolve) => setTimeout(resolve, ms));

// A bundle is a job (SDD-005 section 16): the call holds for a while, and a
// larger project is polled until the job is final.
async function exportBundle(projectId: string): Promise<Job> {
	const client = createClient(new HttpTransport("/api/v1"));
	let job = await client.exportBundle(projectId);
	for (
		let polls = 0;
		(job.state === "queued" || job.state === "running") && polls < POLL_LIMIT;
		polls++
	) {
		await wait(POLL_MS);
		job = await client.getJob(job.job_id);
	}
	return job;
}

// The "Export bundle" action of a project of the list: one request starts the
// job, the result is its path on the machine of the service and a download
// link (SDD-012 section 14).
export function BundleExport({ projectId }: { projectId: string }): ReactNode {
	const t = useT();
	const [busy, setBusy] = useState(false);
	const [job, setJob] = useState<Job | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	async function run(): Promise<void> {
		setBusy(true);
		setJob(null);
		setFailure(null);
		try {
			const done = await exportBundle(projectId);
			if (done.state === "failed" && done.error !== undefined) {
				setFailure({
					code: done.error.code,
					message: done.error.message,
					hint: done.error.hint,
				});
			} else setJob(done);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	const result = job?.state === "succeeded" ? job.result?.export : undefined;
	return (
		<>
			<Button
				type="button"
				variant="outline"
				disabled={busy}
				onClick={() => void run()}
			>
				{busy
					? t("ui.projects.bundle.exporting")
					: t("ui.projects.bundle.export")}
				<span className="sr-only"> {projectId}</span>
			</Button>
			{(result !== undefined || failure !== null) && (
				<div className="basis-full text-sm" data-bundle-result={projectId}>
					{result !== undefined && job !== null && (
						<p className="flex flex-wrap items-center gap-2">
							<span>{t("ui.projects.bundle.ready")}</span>
							{result.path !== undefined && (
								<span className="font-mono text-xs break-all">
									<UntrustedText>{result.path}</UntrustedText>
								</span>
							)}
							<a
								className="text-primary underline"
								href={exportFileUrl(job.job_id)}
								download
							>
								{t("ui.projects.bundle.download")}
							</a>
						</p>
					)}
					<FailureNote failure={failure} />
				</div>
			)}
		</>
	);
}

// The import of a bundle by its path on the machine of the service, never the
// bytes (SDD-007 section 7.6). A copy gets a new id and leaves the history out,
// which is the way to import into a workspace that already has the project.
export function BundleImport(): ReactNode {
	const t = useT();
	const [path, setPath] = useState("");
	const [asNew, setAsNew] = useState(true);
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [imported, setImported] = useState<string | null>(null);

	async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		setImported(null);
		try {
			const made = await createClient(
				new HttpTransport("/api/v1"),
			).importBundle(path.trim(), asNew);
			setImported(made.name);
			setPath("");
			router.reload({ only: ["projects"] });
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<section aria-labelledby="bundle-import">
			<h2 id="bundle-import" className="text-lg font-semibold">
				{t("ui.projects.bundle.title")}
			</h2>
			<FailureNote failure={failure} />
			{imported !== null && (
				<p role="status" className="mt-2 text-sm">
					{t("ui.projects.bundle.imported")}{" "}
					<UntrustedText>{imported}</UntrustedText>
				</p>
			)}
			<form
				onSubmit={(event) => void submit(event)}
				className="mt-4 flex flex-col gap-2"
			>
				<div className="flex items-end gap-2">
					<div className="flex flex-1 flex-col gap-1">
						<Label htmlFor="bundle-import-path">
							{t("ui.projects.bundle.path")}
						</Label>
						<Input
							id="bundle-import-path"
							autoComplete="off"
							spellCheck={false}
							value={path}
							onChange={(event) => setPath(event.target.value)}
						/>
					</div>
					<Button
						type="submit"
						variant="outline"
						disabled={busy || path.trim() === ""}
					>
						{t("ui.projects.bundle.import")}
					</Button>
				</div>
				<label className="flex items-center gap-1.5 text-sm">
					<input
						type="checkbox"
						checked={asNew}
						className="accent-primary"
						onChange={(event) => setAsNew(event.target.checked)}
					/>
					{t("ui.projects.bundle.as_new")}
				</label>
			</form>
		</section>
	);
}

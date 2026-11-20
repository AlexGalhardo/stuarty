import { Link } from "@inertiajs/react";
import { Loader2, X } from "lucide-react";
import { type ReactNode, useEffect, useRef } from "react";

import { FailureNote } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import { formatMoney } from "../../dashboard/approval/money";
import { getModelJob } from "../api/modelEdit";
import { useImageStore } from "../state/context";
import { isFinal, modelJobStore, settledFor, useModelJob } from "./state";

const POLL_MS = 1500;

// The strip under the toolbar that follows the job of an operation with a model (SDD-016 section 6.5): what it is
// doing to which layer, how far it is, that it waits for a person, and, when it is done, what it did to the
// document. It reads the job and says it; when the job succeeds and changed the document, it reloads the
// document and selects the layer the result is in. The job is the service's: nothing is computed here.
export function ModelJobStrip(): ReactNode {
	const t = useT();
	const locale = useLocale();
	const store = useImageStore();
	const tracked = useModelJob((s) => s.tracked);
	const reloaded = useRef<string | null>(null);

	const jobId = tracked?.job.job_id;
	const final = tracked !== null && isFinal(tracked.job);

	useEffect(() => {
		if (jobId === undefined || final) return;
		const timer = setInterval(() => {
			void getModelJob(jobId)
				.then((job) => modelJobStore.getState().update(job))
				.catch(() => undefined);
		}, POLL_MS);
		return () => clearInterval(timer);
	}, [jobId, final]);

	// A job that ended and changed this image: the screen reads the document again and selects the layer. The
	// screen of another image opened meanwhile leaves its own document alone.
	useEffect(() => {
		const settled = settledFor(tracked, store.getState().imageId);
		if (tracked === null || settled === null) return;
		if (reloaded.current === tracked.job.job_id) return;
		reloaded.current = tracked.job.job_id;
		void store
			.getState()
			.open()
			.then(() => {
				if (settled.select !== null) store.getState().select([settled.select]);
			});
	}, [tracked, store]);

	if (tracked === null) return null;

	const { job, operation, layerName } = tracked;
	const generated = job.result?.generated;
	const percent = Math.round(job.progress * 100);

	return (
		<div
			role="status"
			aria-live="polite"
			data-testid="model-job"
			data-job-state={job.state}
			data-operation={operation}
			className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b bg-muted/40 px-3 py-1.5 text-sm"
		>
			{!final && <Loader2 aria-hidden="true" className="size-4 animate-spin" />}
			<span className="font-medium">
				{t(`ui.image.model.working.${operation}`)}
				{layerName === "" ? null : (
					<>
						{" "}
						<UntrustedText>{layerName}</UntrustedText>
					</>
				)}
			</span>
			<span>{t(`ui.image.model.state.${job.state}`)}</span>
			{job.state === "running" && (
				<span className="tabular-nums" data-testid="model-job-progress">
					{t("ui.image.model.progress", { percent })}
				</span>
			)}
			{job.state === "waiting_approval" && (
				<Link href="/approvals" className="underline">
					{t("ui.image.model.open_approvals")}
				</Link>
			)}
			{job.state === "succeeded" && generated !== undefined && (
				<>
					<span data-testid="model-job-result">
						{generated.image?.applied === true
							? t("ui.image.model.done.applied", {
									amount: formatMoney(generated.cost, locale),
								})
							: t("ui.image.model.done.kept", {
									amount: formatMoney(generated.cost, locale),
								})}
					</span>
					{(generated.notes ?? []).map((note) => (
						<span key={note} className="text-muted-foreground">
							<UntrustedText>{note}</UntrustedText>
						</span>
					))}
				</>
			)}
			{job.state === "failed" && job.error !== undefined && (
				<FailureNote
					failure={{
						code: job.error.code,
						message: job.error.message,
						hint: job.error.hint,
					}}
				/>
			)}
			{final && (
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					className="ml-auto"
					aria-label={t("ui.image.model.dismiss")}
					data-testid="model-job-dismiss"
					onClick={() => modelJobStore.getState().dismiss()}
				>
					<X aria-hidden="true" />
				</Button>
			)}
		</div>
	);
}

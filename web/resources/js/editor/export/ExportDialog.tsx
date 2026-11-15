import {
	CircleAlert,
	CircleCheck,
	CircleSlash,
	Clock,
	Copy,
	Info,
	TriangleAlert,
} from "lucide-react";
import {
	type ReactNode,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Progress } from "@/components/ui/progress";
import { useLocale, useT } from "@/lib/i18n";
import { ApiError } from "../api/errors";
import type { JsonValue, LintFinding } from "../api/types";
import { exportFileUrl } from "../api/urls";
import { useStore } from "../state/createStore";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { errorHeadline } from "../ui/errorHeadline";
import { formatDecimal, formatSeconds } from "../ui/format";
import { announcedPercent, exportPhase, groupFindings } from "./exportPhase";

const POLL_AFTER_MS = 5000;
const POLL_EVERY_MS = 2000;

interface Failure {
	code: string;
	message: string;
	hint?: string;
	details?: { [key: string]: JsonValue };
}

function FindingList({
	title,
	icon,
	findings,
	onGo,
}: {
	title: string;
	icon: ReactNode;
	findings: readonly LintFinding[];
	onGo(itemId: string): void;
}): ReactNode {
	const t = useT();
	if (findings.length === 0) return null;
	return (
		<section className="mb-3">
			<h3 className="flex items-center gap-1.5 text-sm font-semibold">
				{icon}
				{title} ({findings.length})
			</h3>
			<ul className="mt-1 flex flex-col gap-2">
				{findings.map((finding) => (
					<li
						key={`${finding.code}:${finding.item ?? ""}:${finding.message}`}
						className="text-xs"
					>
						<span className="font-mono">{finding.code}</span>{" "}
						<span lang="en">
							<UntrustedText>{finding.message}</UntrustedText>
						</span>
						{typeof finding.hint === "string" && (
							<p lang="en" className="text-muted-foreground">
								<UntrustedText>{finding.hint}</UntrustedText>
							</p>
						)}
						{finding.item !== undefined && (
							<Button
								type="button"
								variant="link"
								size="sm"
								onClick={() => onGo(finding.item as string)}
							>
								{t("ui.editor.export.go_to", { item: finding.item })}
							</Button>
						)}
					</li>
				))}
			</ul>
		</section>
	);
}

// The export dialog of SDD-012 section 14 and docs/design/alpha-editor.md
// section 5: lint findings, start, progress from events, the result with its
// path and a download link, or the failure with its code and hint. The job
// outlives the dialog; closing it keeps the export running.
export default function ExportDialog({
	open,
	onOpenChange,
}: {
	open: boolean;
	onOpenChange(open: boolean): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const runtime = useRuntime();
	const revision = useDocumentState((s) => s.revision);
	const jobId = useStore(runtime.jobs, (s) => s.exportJobId);
	const job = useStore(runtime.jobs, (s) =>
		s.exportJobId === null ? undefined : s.jobs[s.exportJobId],
	);
	const lastEventAt = useStore(runtime.jobs, (s) => s.lastEventAt);
	const [checking, setChecking] = useState(false);
	const [findings, setFindings] = useState<readonly LintFinding[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const startedAt = useRef(0);

	const check = useCallback(async (): Promise<void> => {
		setChecking(true);
		setFailure(null);
		try {
			setFindings((await runtime.client.getLint(runtime.projectId)).findings);
		} catch (error) {
			if (error instanceof ApiError) {
				setFailure({
					code: error.code,
					message: error.message,
					hint: error.hint,
				});
			}
		} finally {
			setChecking(false);
		}
	}, [runtime]);

	useEffect(() => {
		if (open && jobId === null) void check();
	}, [open, jobId, check]);

	// Without an event for the job for 5 s, poll every 2 s until one arrives.
	const running = job?.state === "running" || job?.state === "queued";
	// biome-ignore lint/correctness/useExhaustiveDependencies: lastEventAt restarts the quiet period.
	useEffect(() => {
		if (!open || !running || jobId === null) return;
		const timer = setInterval(() => {
			const quietSince = Math.max(
				runtime.jobs.getState().lastEventAt,
				startedAt.current,
			);
			if (Date.now() - quietSince < POLL_AFTER_MS) return;
			runtime.client
				.getJob(jobId)
				.then((fresh) => runtime.jobsWriter.upsert(fresh))
				.catch(() => undefined);
		}, POLL_EVERY_MS);
		return () => clearInterval(timer);
	}, [open, running, jobId, runtime, lastEventAt]);

	async function start(): Promise<void> {
		setFailure(null);
		try {
			const started = await runtime.client.startExport(runtime.projectId);
			startedAt.current = Date.now();
			runtime.jobsWriter.upsert(started);
			runtime.jobsWriter.setExportJob(started.job_id);
		} catch (error) {
			if (!(error instanceof ApiError)) throw error;
			const raw = error.details?.findings;
			if (error.code === "E_LINT_BLOCKING" && Array.isArray(raw)) {
				setFindings(raw as unknown as LintFinding[]);
				return;
			}
			setFailure({
				code: error.code,
				message: error.message,
				hint: error.hint,
				details: error.details,
			});
		}
	}

	function again(): void {
		runtime.jobsWriter.setExportJob(null);
		setFailure(null);
	}

	function goTo(itemId: string): void {
		runtime.uiWriter.select([itemId]);
		onOpenChange(false);
	}

	const phase = exportPhase({ job, checking, findings });
	const groups = groupFindings(findings);
	const percent = Math.round((job?.progress ?? 0) * 100);
	const result = job?.result?.export;
	const newer =
		result !== undefined && revision !== null && revision > result.revision;
	const shown: Failure | null = job?.error ?? failure;
	const noRetry =
		shown?.code === "E_FFMPEG_MISSING" || shown?.code === "E_NO_ENCODER";
	const checks = shown?.details?.checks;
	const showFailure = shown !== null && phase !== "running";

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-h-[calc(100vh-48px)] overflow-auto sm:max-w-[560px]">
				<DialogHeader>
					<DialogTitle>{t("ui.editor.export.title")}</DialogTitle>
					<DialogDescription>
						{revision !== null && t("ui.editor.export.revision", { revision })}
					</DialogDescription>
				</DialogHeader>

				<div aria-live="polite" data-phase={phase}>
					{phase === "checking" && (
						<p className="flex items-center gap-1.5 text-sm">
							<Clock aria-hidden="true" className="size-4" />
							{t("ui.editor.export.checking")}
						</p>
					)}

					{(phase === "ready" || phase === "blocked") && (
						<>
							{findings.length === 0 && (
								<p className="text-sm">{t("ui.editor.export.no_problems")}</p>
							)}
							<FindingList
								title={t("ui.editor.export.errors")}
								icon={
									<CircleAlert
										aria-hidden="true"
										className="size-4 text-destructive"
									/>
								}
								findings={groups.errors}
								onGo={goTo}
							/>
							<FindingList
								title={t("ui.editor.export.warnings")}
								icon={
									<TriangleAlert
										aria-hidden="true"
										className="size-4 text-warning"
									/>
								}
								findings={groups.warnings}
								onGo={goTo}
							/>
							<FindingList
								title={t("ui.editor.export.notes")}
								icon={<Info aria-hidden="true" className="size-4 text-info" />}
								findings={groups.notes}
								onGo={goTo}
							/>
							{phase === "blocked" && (
								<p className="text-sm">{t("ui.editor.export.blocked")}</p>
							)}
						</>
					)}

					{phase === "running" && (
						<div>
							<p className="text-sm">{t("ui.editor.export.running")}</p>
							<Progress
								value={percent}
								aria-label={t("ui.editor.export.progress")}
								className="my-2 h-2"
							/>
							<p
								className="text-xs [font-variant-numeric:tabular-nums]"
								data-announce={announcedPercent(job?.progress ?? 0)}
							>
								{percent}%
								{job?.message !== undefined && (
									<span lang="en">
										{" · "}
										<UntrustedText>{job.message}</UntrustedText>
									</span>
								)}
							</p>
							<p className="mt-2 text-xs text-muted-foreground">
								{t("ui.editor.export.keeps_running")}
							</p>
						</div>
					)}

					{phase === "succeeded" && result !== undefined && (
						<div className="text-sm">
							<p className="flex items-center gap-1.5 font-medium">
								<CircleCheck
									aria-hidden="true"
									className="size-4 text-success"
								/>
								{t("ui.editor.export.finished")}
							</p>
							{result.path !== undefined && (
								<p className="mt-1 flex items-center gap-1 font-mono text-xs break-all">
									<UntrustedText>{result.path}</UntrustedText>
									<Button
										type="button"
										variant="ghost"
										size="icon-sm"
										aria-label={t("ui.editor.export.copy_path")}
										onClick={() =>
											void navigator.clipboard.writeText(result.path ?? "")
										}
									>
										<Copy aria-hidden="true" />
									</Button>
								</p>
							)}
							<p className="text-xs text-muted-foreground [font-variant-numeric:tabular-nums]">
								{t("ui.editor.export.size", {
									megabytes: formatDecimal(result.bytes / 1_000_000, locale),
									duration: formatSeconds(result.duration.seconds, locale),
									revision: result.revision,
								})}
							</p>
							{newer && (
								<p
									role="status"
									className="mt-2 flex items-center gap-1.5 text-xs text-warning"
								>
									<TriangleAlert aria-hidden="true" className="size-4" />
									{t("ui.editor.export.newer", { revision: revision ?? 0 })}
								</p>
							)}
						</div>
					)}

					{phase === "cancelled" && (
						<p className="flex items-center gap-1.5 text-sm text-muted-foreground">
							<CircleSlash aria-hidden="true" className="size-4" />
							{t("ui.editor.export.cancelled")}
						</p>
					)}

					{showFailure && (
						<div role="alert" className="text-sm">
							<p className="flex items-center gap-1.5 font-medium text-destructive">
								<CircleAlert aria-hidden="true" className="size-4" />
								{errorHeadline(t, shown.code)}
							</p>
							<p lang="en">
								<UntrustedText>{shown.message}</UntrustedText>
							</p>
							{shown.hint !== undefined && (
								<p lang="en" className="text-muted-foreground">
									<UntrustedText>{shown.hint}</UntrustedText>
								</p>
							)}
							<p className="font-mono text-xs">{shown.code}</p>
							{Array.isArray(checks) && (
								<p className="text-xs">
									{t("ui.editor.export.failed_checks")} {checks.join(", ")}
								</p>
							)}
							{Array.isArray(shown.details?.running) && (
								<p className="text-xs">{t("ui.editor.export.queue_full")}</p>
							)}
						</div>
					)}
				</div>

				<DialogFooter>
					{(phase === "ready" ||
						phase === "blocked" ||
						phase === "checking") && (
						<>
							<Button
								type="button"
								variant="outline"
								disabled={phase === "checking"}
								onClick={() => void check()}
							>
								{t("ui.editor.export.recheck")}
							</Button>
							<Button
								type="button"
								disabled={phase !== "ready"}
								onClick={() => void start()}
							>
								{failure?.code === "E_QUEUE_FULL"
									? t("ui.editor.export.try_again")
									: t("ui.editor.export.start")}
							</Button>
						</>
					)}
					{phase === "running" && (
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							{t("ui.editor.export.close")}
						</Button>
					)}
					{phase === "succeeded" && result !== undefined && jobId !== null && (
						<>
							<Button
								type="button"
								variant="outline"
								onClick={() => onOpenChange(false)}
							>
								{t("ui.editor.export.close")}
							</Button>
							<Button asChild>
								<a href={exportFileUrl(jobId)} download>
									{t("ui.editor.export.download")}
								</a>
							</Button>
						</>
					)}
					{(phase === "failed" || phase === "cancelled") && (
						<>
							<Button
								type="button"
								variant="outline"
								onClick={() => onOpenChange(false)}
							>
								{t("ui.editor.export.close")}
							</Button>
							{shown !== null && (
								<Button
									type="button"
									variant="outline"
									onClick={() =>
										void navigator.clipboard.writeText(
											JSON.stringify(shown, null, 2),
										)
									}
								>
									{t("ui.editor.export.copy_details")}
								</Button>
							)}
							{!noRetry && (
								<Button type="button" onClick={again}>
									{phase === "failed"
										? t("ui.editor.export.try_again")
										: t("ui.editor.export.again")}
								</Button>
							)}
						</>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

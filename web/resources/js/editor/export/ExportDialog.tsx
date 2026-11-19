import {
	CircleAlert,
	CircleCheck,
	CircleSlash,
	Clock,
	Copy,
	FolderOpen,
	Info,
	TriangleAlert,
} from "lucide-react";
import {
	type ReactNode,
	useCallback,
	useEffect,
	useRef,
	useState,
	useSyncExternalStore,
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
import {
	canShowInFolder,
	getDesktopBridge,
	showInFolder,
	subscribeDesktopBridge,
} from "@/lib/desktopBridge";
import { type Translate, useLocale, useT } from "@/lib/i18n";
import { createClient } from "../api/client";
import { ApiError } from "../api/errors";
import { HttpTransport } from "../api/http";
import type {
	CodecAvailability,
	ExportCapabilities,
	ExportOptions,
	ExportPreset,
	Job,
	JsonValue,
	LintFinding,
} from "../api/types";
import { exportFileUrl } from "../api/urls";
import ErrorBody from "../commands/ErrorBody";
import { formatSeconds } from "../commands/units";
import { useActions } from "../commands/useActions";
import { useStore } from "../state/createStore";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { readTimeDisplay } from "../state/prefs";
import { errorHeadline } from "../ui/errorHeadline";
import { formatDecimal } from "../ui/format";
import BatchProgress from "./BatchProgress";
import ExportOptionsFields, {
	formatOptions,
	type Unsupported,
} from "./ExportOptions";
import ExportPresets, { type PresetCheck } from "./ExportPresets";
import { exportCapabilities } from "./exportCapabilities";
import {
	announcedPercent,
	exportIsStale,
	exportPhase,
	groupFindings,
} from "./exportPhase";
import { batchItems, queuedItem } from "./presetText";

const POLL_AFTER_MS = 5000;
const POLL_EVERY_MS = 2000;
// A batch job is read every second: the events of the job carry its progress,
// not the state of its children.
const BATCH_POLL_MS = 1000;

interface Failure {
	code: string;
	message: string;
	hint?: string;
	details?: { [key: string]: JsonValue };
}

// The localized name of a lint rule (ui.editor.lint.<rule>); a rule the
// catalogue does not know shows its code only.
function lintName(t: Translate, code: string): string {
	const key = `ui.editor.lint.${code}`;
	const name = t(key);
	return name === key ? code : name;
}

interface VerifyCheck {
	name: string;
	expected: string;
	got: string;
}

// details.checks of E_VERIFY_FAILED: what the verification of the file found
// (SDD-003 error table); an entry that is not that shape is left out.
function verifyChecks(raw: unknown): VerifyCheck[] {
	if (!Array.isArray(raw)) return [];
	return raw.flatMap((entry) => {
		if (typeof entry !== "object" || entry === null) return [];
		const { name, expected, got } = entry as Record<string, unknown>;
		return typeof name === "string"
			? [{ name, expected: String(expected ?? ""), got: String(got ?? "") }]
			: [];
	});
}

function FindingList({
	title,
	icon,
	findings,
	onGo,
	onResync,
}: {
	title: string;
	icon: ReactNode;
	findings: readonly LintFinding[];
	onGo(itemId: string): void;
	onResync(captionId: string): void;
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
						<span className="font-medium">{lintName(t, finding.code)}</span>{" "}
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
						{finding.code === "L_CAPTION_DESYNC" &&
							finding.item !== undefined && (
								<Button
									type="button"
									variant="link"
									size="sm"
									onClick={() => onResync(finding.item as string)}
								>
									{t("ui.editor.auto_captions.resync")}
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
	const actions = useActions();
	const revision = useDocumentState((s) => s.revision);
	const jobId = useStore(runtime.jobs, (s) => s.exportJobId);
	const job = useStore(runtime.jobs, (s) =>
		s.exportJobId === null ? undefined : s.jobs[s.exportJobId],
	);
	const range = useStore(runtime.ui, (s) => s.range);
	const playhead = useStore(runtime.ui, (s) => s.playheadFrame);
	const lastEventAt = useStore(runtime.jobs, (s) => s.lastEventAt);
	const [checking, setChecking] = useState(false);
	const [findings, setFindings] = useState<readonly LintFinding[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [cancelling, setCancelling] = useState(false);
	const bridge = useSyncExternalStore(
		subscribeDesktopBridge,
		getDesktopBridge,
		() => null,
	);
	const startedAt = useRef(0);
	const [capabilities, setCapabilities] = useState<ExportCapabilities | null>(
		null,
	);
	const [options, setOptions] = useState<ExportOptions>({});
	const [unsupported, setUnsupported] = useState<Unsupported | null>(null);
	// The platform presets the person picks (RF-087), the codec asked for ("" is
	// the preset's own preference), what each preset says about the project, and
	// the codecs this machine writes once the core has tried them.
	const [presets, setPresets] = useState<readonly ExportPreset[] | null>(null);
	const [codecInfo, setCodecInfo] = useState<
		readonly CodecAvailability[] | null
	>(null);
	const [selected, setSelected] = useState<readonly string[]>([]);
	const [codec, setCodec] = useState("");
	const [presetChecks, setPresetChecks] = useState<
		Readonly<Record<string, PresetCheck>>
	>({});
	// A plain export that found every export slot busy was queued as a batch of
	// one (VV-220): the dialog says it waits.
	const [queuedBehind, setQueuedBehind] = useState(false);

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

	// Reopening after the project changed starts over (VV-009).
	const stale = exportIsStale(job, revision);
	useEffect(() => {
		if (open && stale) {
			runtime.jobsWriter.setExportJob(null);
			setFailure(null);
		}
	}, [open, stale, runtime]);

	useEffect(() => {
		if (open && jobId === null) void check();
	}, [open, jobId, check]);

	// What the server accepts as export arguments, read once from its schema; a
	// server that does not expose them leaves the dialog as the plain export.
	useEffect(() => {
		if (!open || capabilities !== null) return;
		createClient(new HttpTransport(""))
			.getOpenApi()
			.then(
				(doc) => setCapabilities(exportCapabilities(doc)),
				() => setCapabilities({ formats: [], range: false }),
			);
	}, [open, capabilities]);

	// The platforms, then which codecs this machine writes: the second call runs
	// the test encodes the first time, so the list does not wait for it.
	useEffect(() => {
		if (!open || presets !== null) return;
		let live = true;
		runtime.client.getExportPresets().then(
			(list) => {
				if (live) setPresets(list.presets);
			},
			() => {
				if (live) setPresets([]);
			},
		);
		runtime.client.getExportPresets(true).then(
			(list) => {
				if (live && list.codecs !== undefined) setCodecInfo(list.codecs);
			},
			() => undefined,
		);
		return () => {
			live = false;
		};
	}, [open, presets, runtime]);

	// What the core says about the project against each chosen preset, again
	// when the range or the revision changes. Only the core knows the limits.
	// biome-ignore lint/correctness/useExhaustiveDependencies: revision restarts the check.
	useEffect(() => {
		if (!open || jobId !== null || selected.length === 0) return;
		let live = true;
		setPresetChecks(
			(prev) =>
				Object.fromEntries(
					selected.map((id) => [
						id,
						prev[id] === undefined ? "pending" : prev[id],
					]),
				) as Record<string, PresetCheck>,
		);
		for (const id of selected) {
			runtime.client
				.checkExport(runtime.projectId, id, {
					start: options.start,
					end: options.end,
				})
				.then(
					(result) => {
						if (live) setPresetChecks((prev) => ({ ...prev, [id]: result }));
					},
					() => {
						if (live)
							setPresetChecks((prev) => {
								const { [id]: _gone, ...rest } = prev;
								return rest;
							});
					},
				);
		}
		return () => {
			live = false;
		};
	}, [open, jobId, selected, options.start, options.end, revision, runtime]);

	// A choice the machine cannot encode is the core to say: a dry run of the
	// start returns its error and hint without queuing anything.
	useEffect(() => {
		setUnsupported(null);
		const chosen = Object.values(options).some((value) => value !== undefined);
		if (!open || jobId !== null || !chosen) return;
		let live = true;
		runtime.client
			.startExport(runtime.projectId, options, true)
			.catch((error: unknown) => {
				if (!live || !(error instanceof ApiError)) return;
				if (error.code === "E_LINT_BLOCKING") return;
				setUnsupported({
					code: error.code,
					message: error.message,
					hint: error.hint,
				});
			});
		return () => {
			live = false;
		};
	}, [open, jobId, options, runtime]);

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

	// The children of a batch change without an event for the job itself.
	const isBatch = job?.kind === "export_batch";
	useEffect(() => {
		if (!open || !running || !isBatch || jobId === null) return;
		const timer = setInterval(() => {
			runtime.client
				.getJob(jobId)
				.then((fresh) => runtime.jobsWriter.upsert(fresh))
				.catch(() => undefined);
		}, BATCH_POLL_MS);
		return () => clearInterval(timer);
	}, [open, running, isBatch, jobId, runtime]);

	async function cancel(): Promise<void> {
		if (jobId === null) return;
		setCancelling(true);
		try {
			runtime.jobsWriter.upsert(await runtime.client.cancelJob(jobId));
		} catch (error) {
			if (!(error instanceof ApiError)) throw error;
			setFailure({
				code: error.code,
				message: error.message,
				hint: error.hint,
			});
		} finally {
			setCancelling(false);
		}
	}

	async function revealExport(path: string): Promise<void> {
		try {
			await showInFolder(bridge, path);
		} catch (error) {
			setFailure({
				code: "E_DESKTOP",
				message: error instanceof Error ? error.message : String(error),
			});
		}
	}

	function begin(started: Job): void {
		startedAt.current = Date.now();
		runtime.jobsWriter.upsert(started);
		runtime.jobsWriter.setExportJob(started.job_id);
	}

	function toggle(id: string): void {
		const next = selected.includes(id)
			? selected.filter((other) => other !== id)
			: [...selected, id];
		setSelected(next);
		// A preset is an mp4: the format goes back to the video, and the range stays.
		if (next.length > 0) setOptions(formatOptions(options, "mp4", playhead));
	}

	async function start(): Promise<void> {
		setFailure(null);
		setQueuedBehind(false);
		const withCodec: ExportOptions = {
			...options,
			codec: codec === "" ? undefined : codec,
		};
		try {
			if (selected.length > 0) {
				begin(
					await runtime.client.startExportBatch(
						runtime.projectId,
						batchItems(selected, codec, options),
					),
				);
				return;
			}
			begin(await runtime.client.startExport(runtime.projectId, withCodec));
		} catch (error) {
			if (!(error instanceof ApiError)) throw error;
			// Every export slot is busy: the core queues the export behind them
			// as a batch of one, and starts it by itself (VV-220). A png is one
			// frame and has no batch.
			if (
				error.code === "E_QUEUE_FULL" &&
				selected.length === 0 &&
				withCodec.format !== "png"
			) {
				try {
					begin(
						await runtime.client.startExportBatch(runtime.projectId, [
							queuedItem(withCodec),
						]),
					);
					setQueuedBehind(true);
					return;
				} catch (again) {
					if (!(again instanceof ApiError)) throw again;
					setFailure({
						code: again.code,
						message: again.message,
						hint: again.hint,
						details: again.details,
					});
					return;
				}
			}
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
		setQueuedBehind(false);
	}

	// The clip a caption is linked to comes from the read model; the core retimes.
	async function resync(captionId: string): Promise<void> {
		const clipId =
			runtime.documents.getState().document?.items[captionId]?.linked_item;
		if (clipId === undefined) return;
		const outcome = await actions.resyncCaptions(clipId);
		if (outcome?.ok) await check();
	}

	function goTo(itemId: string): void {
		runtime.uiWriter.select([itemId]);
		onOpenChange(false);
	}

	const phase = exportPhase({ job, checking, findings });
	const groups = groupFindings(findings);
	const percent = Math.round((job?.progress ?? 0) * 100);
	const result = job?.result?.export;
	const batch = job?.result?.batch;
	const newer =
		result !== undefined && revision !== null && revision > result.revision;
	const shown: Failure | null = job?.error ?? failure;
	const noRetry =
		shown?.code === "E_FFMPEG_MISSING" || shown?.code === "E_NO_ENCODER";
	const checks = verifyChecks(shown?.details?.checks);
	// A cancelled job carries E_CANCELLED, which is the outcome asked for, not a
	// failure to report (SDD-012 section 14.1).
	const showFailure =
		shown !== null && phase !== "running" && phase !== "cancelled";

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
							{capabilities !== null && (
								<ExportOptionsFields
									capabilities={capabilities}
									value={options}
									disabled={false}
									unsupported={unsupported}
									range={range}
									playhead={playhead}
									onChange={setOptions}
								/>
							)}
							{phase === "ready" && presets === null && (
								<p className="mb-3 text-xs text-muted-foreground">
									{t("ui.editor.export.presets.loading")}
								</p>
							)}
							{phase === "ready" && presets?.length === 0 && (
								<p className="mb-3 text-xs text-muted-foreground">
									{t("ui.editor.export.presets.unavailable")}
								</p>
							)}
							{phase === "ready" && presets !== null && presets.length > 0 && (
								<ExportPresets
									presets={presets}
									selected={selected}
									checks={presetChecks}
									codecs={codecInfo}
									codec={codec}
									codecDisabled={
										selected.length === 0 &&
										options.format !== undefined &&
										options.format !== "mp4"
									}
									disabled={false}
									onToggle={toggle}
									onCodec={setCodec}
								/>
							)}
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
								onResync={(id) => void resync(id)}
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
								onResync={(id) => void resync(id)}
							/>
							<FindingList
								title={t("ui.editor.export.notes")}
								icon={<Info aria-hidden="true" className="size-4 text-info" />}
								findings={groups.notes}
								onGo={goTo}
								onResync={(id) => void resync(id)}
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
							{queuedBehind && (
								<p role="status" className="mt-1 text-xs text-muted-foreground">
									{t("ui.editor.export.batch.queued_behind")}
								</p>
							)}
							<p className="mt-2 text-xs text-muted-foreground">
								{t("ui.editor.export.keeps_running")}
							</p>
						</div>
					)}

					{batch !== undefined && phase === "failed" && (
						<p role="status" className="mt-2 text-sm font-medium">
							{t("ui.editor.export.batch.some_failed")}
						</p>
					)}
					{batch !== undefined && phase !== "ready" && (
						<BatchProgress batch={batch} presets={presets ?? []} />
					)}

					{phase === "succeeded" && batch !== undefined && (
						<p className="flex items-center gap-1.5 text-sm font-medium">
							<CircleCheck aria-hidden="true" className="size-4 text-success" />
							{t("ui.editor.export.batch.finished")}
						</p>
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
									{canShowInFolder(bridge) && (
										<Button
											type="button"
											variant="ghost"
											size="icon-sm"
											aria-label={t("ui.editor.export.show_in_folder")}
											onClick={() => void revealExport(result.path ?? "")}
										>
											<FolderOpen aria-hidden="true" />
										</Button>
									)}
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
									duration: formatSeconds(
										result.duration.seconds,
										readTimeDisplay(),
									),
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
							<ErrorBody error={shown} />
							<p className="font-mono text-xs">{shown.code}</p>
							{checks.length > 0 && (
								<div className="text-xs">
									<p>{t("ui.editor.export.failed_checks")}</p>
									<ul className="list-disc pl-4" data-verify-checks="">
										{checks.map((check) => (
											<li key={check.name}>
												<span className="font-mono">{check.name}</span>
												{" · "}
												<span lang="en">
													<UntrustedText>
														{t("ui.editor.export.check_line", {
															expected: check.expected,
															got: check.got,
														})}
													</UntrustedText>
												</span>
											</li>
										))}
									</ul>
								</div>
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
								disabled={phase !== "ready" || unsupported !== null}
								onClick={() => void start()}
							>
								{failure?.code === "E_QUEUE_FULL"
									? t("ui.editor.export.try_again")
									: selected.length > 1
										? t("ui.editor.export.presets.start_many", {
												count: selected.length,
											})
										: t("ui.editor.export.start")}
							</Button>
						</>
					)}
					{phase === "running" && (
						<>
							<Button
								type="button"
								variant="outline"
								disabled={cancelling}
								onClick={() => void cancel()}
							>
								{cancelling
									? t("ui.editor.export.cancelling")
									: t("ui.editor.export.cancel")}
							</Button>
							<Button
								type="button"
								variant="outline"
								onClick={() => onOpenChange(false)}
							>
								{t("ui.editor.export.close")}
							</Button>
						</>
					)}
					{phase === "succeeded" && batch !== undefined && (
						<>
							<Button
								type="button"
								variant="outline"
								onClick={() => onOpenChange(false)}
							>
								{t("ui.editor.export.close")}
							</Button>
							<Button type="button" variant="outline" onClick={again}>
								{t("ui.editor.export.again")}
							</Button>
						</>
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
							<Button type="button" variant="outline" onClick={again}>
								{t("ui.editor.export.again")}
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
							{shown !== null && phase === "failed" && (
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

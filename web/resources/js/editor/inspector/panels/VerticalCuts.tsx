import { Check, ScanFace } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { useT } from "@/lib/i18n";
import { ApiError } from "../../api/errors";
import type { CutsMediaFile, CutsProposal, ToolOffer } from "../../api/types";
import { useCanEdit } from "../../commands/useActions";
import { useStore } from "../../state/createStore";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import { errorHeadline } from "../../ui/errorHeadline";
import Section from "../Section";
import FaceInstallDialog from "./FaceInstallDialog";
import {
	clockLabel,
	faceOffer,
	isFaceUnavailable,
	sharePercent,
	timelineTitle,
} from "./verticalCutsModel";

const POLL_AFTER_MS = 5000;
const POLL_EVERY_MS = 2000;

type Phase =
	| { name: "idle" }
	| { name: "checking" }
	| { name: "tracking"; jobId: string }
	| { name: "proposing" }
	| { name: "review"; proposals: CutsProposal[]; mediaFile: CutsMediaFile }
	| { name: "creating" }
	| { name: "created"; count: number }
	| { name: "failed"; code: string; message: string; hint?: string };

// The Vertical cuts section (SDD-024 section 8): the face model is offered, never downloaded, until the person
// installs it; a face_track job finds the faces; the service proposes the cuts, and nothing is created until the
// person chooses some and accepts. Each accepted proposal becomes a timeline of the project that receives the
// batch of commands the service built, as it came: the browser computes no range and no framing.
export default function VerticalCuts({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const canEdit = useCanEdit();
	const assetId = useDocumentState((s) => s.document?.items[itemId]?.asset);
	const isVideo = useDocumentState((s) => {
		const asset =
			assetId === undefined ? undefined : s.document?.assets[assetId];
		return asset?.kind === "video";
	});
	const [phase, setPhase] = useState<Phase>({ name: "idle" });
	const [offer, setOffer] = useState<ToolOffer | null>(null);
	const [chosen, setChosen] = useState<ReadonlySet<number>>(new Set());
	const lastChange = useRef(0);

	const jobId = phase.name === "tracking" ? phase.jobId : null;
	const job = useStore(runtime.jobs, (s) =>
		jobId === null ? undefined : s.jobs[jobId],
	);

	async function propose(id: string): Promise<void> {
		setPhase({ name: "proposing" });
		try {
			const result = await runtime.client.getCuts(runtime.projectId, id);
			setChosen(new Set(result.proposals.map((p) => p.n)));
			setPhase({
				name: "review",
				proposals: result.proposals,
				mediaFile: result.media_file,
			});
		} catch (error) {
			fail(error);
		}
	}

	function fail(error: unknown): void {
		if (!(error instanceof ApiError)) throw error;
		failWith(error);
	}

	// A missing face set opens its install prompt; with no set to offer (a platform the catalogue has no helper
	// for), the error itself is shown, so the section never stays on "checking" (VV-422).
	function failWith(error: {
		code: string;
		message: string;
		hint?: string;
		details?: ApiError["details"];
	}): void {
		const failed: Phase = {
			name: "failed",
			code: error.code,
			message: error.message,
			hint: error.hint,
		};
		if (!isFaceUnavailable(error)) {
			setPhase(failed);
			return;
		}
		showOffer().then(
			(shown) => {
				if (!shown) setPhase(failed);
			},
			() => setPhase(failed),
		);
	}

	async function showOffer(): Promise<boolean> {
		const status = await runtime.client.getTools();
		const found = faceOffer(status);
		if (found === undefined || found.installed) return false;
		setOffer(found);
		setPhase({ name: "idle" });
		return true;
	}

	// The track job: success asks for the proposals; failure shows its error.
	// biome-ignore lint/correctness/useExhaustiveDependencies: `propose` and `failWith` only read stable runtime members.
	useEffect(() => {
		if (job === undefined || assetId === undefined) return;
		lastChange.current = Date.now();
		if (job.state === "succeeded") {
			void propose(assetId);
		} else if (job.state === "failed" || job.state === "cancelled") {
			if (job.error === undefined) {
				setPhase({ name: "idle" });
			} else {
				failWith(job.error);
			}
		}
	}, [job, assetId]);

	// Without an event for the job for 5 s, ask for it every 2 s.
	useEffect(() => {
		if (jobId === null) return;
		lastChange.current = Date.now();
		const timer = setInterval(() => {
			if (Date.now() - lastChange.current < POLL_AFTER_MS) return;
			runtime.client
				.getJob(jobId)
				.then((fresh) => runtime.jobsWriter.upsert(fresh))
				.catch(() => undefined);
		}, POLL_EVERY_MS);
		return () => clearInterval(timer);
	}, [jobId, runtime]);

	if (!isVideo || assetId === undefined) return null;

	async function find(): Promise<void> {
		if (assetId === undefined) return;
		setPhase({ name: "checking" });
		try {
			if (await showOffer()) return;
			const started = await runtime.client.startFaceTrack(
				runtime.projectId,
				assetId,
			);
			runtime.jobsWriter.upsert(started);
			setPhase({ name: "tracking", jobId: started.job_id });
		} catch (error) {
			fail(error);
		}
	}

	// Each kept proposal: a new timeline, the asset placed in it under the id the batch names (only the import
	// flow adds an asset), then the batch at the revision the placement left.
	async function accept(
		proposals: CutsProposal[],
		mediaFile: CutsMediaFile,
	): Promise<void> {
		const picked = proposals.filter((p) => chosen.has(p.n));
		if (picked.length === 0) return;
		setPhase({ name: "creating" });
		try {
			for (const proposal of picked) {
				const added = await runtime.client.addProjectItem(runtime.projectId, {
					kind: "timeline",
					title: timelineTitle(proposal),
				});
				const timelineId = added.item.timeline_id;
				if (timelineId === undefined) continue;
				const placed = await runtime.client.placeCutMedia(
					runtime.projectId,
					timelineId,
					mediaFile,
				);
				const refused = placed.assets.find((a) => a.error !== undefined)?.error;
				if (refused !== undefined) throw new ApiError(422, refused);
				await runtime.client.editTimeline(runtime.projectId, timelineId, {
					operations: proposal.batch,
					base_revision: placed.revision,
					idempotency_key: crypto.randomUUID(),
				});
			}
			setPhase({ name: "created", count: picked.length });
		} catch (error) {
			fail(error);
		}
	}

	const busy =
		phase.name === "checking" ||
		phase.name === "tracking" ||
		phase.name === "proposing" ||
		phase.name === "creating";
	const percent = Math.round((job?.progress ?? 0) * 100);

	return (
		<Section title={t("ui.editor.vertical_cuts.title")} icon={ScanFace}>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.vertical_cuts.hint")}
			</p>
			<div>
				<Button
					type="button"
					size="sm"
					disabled={!canEdit || busy}
					onClick={() => void find()}
				>
					{t("ui.editor.vertical_cuts.find")}
				</Button>
			</div>

			{phase.name === "checking" && (
				<p role="status" className="text-xs">
					{t("ui.editor.vertical_cuts.checking")}
				</p>
			)}
			{phase.name === "tracking" && (
				<div role="status">
					<p className="text-xs">{t("ui.editor.vertical_cuts.tracking")}</p>
					<Progress
						value={percent}
						aria-label={t("ui.editor.vertical_cuts.progress")}
						className="my-1 h-2"
					/>
					<p className="text-xs [font-variant-numeric:tabular-nums]">
						{percent}%
					</p>
				</div>
			)}
			{phase.name === "proposing" && (
				<p role="status" className="text-xs">
					{t("ui.editor.vertical_cuts.proposing")}
				</p>
			)}
			{phase.name === "review" && (
				<div className="flex flex-col gap-2">
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.vertical_cuts.review")}
					</p>
					{phase.proposals.length === 0 ? (
						<p role="status" className="text-xs">
							{t("ui.editor.vertical_cuts.none")}
						</p>
					) : (
						<ul
							className="flex flex-col gap-1"
							aria-label={t("ui.editor.vertical_cuts.proposals")}
						>
							{phase.proposals.map((proposal) => (
								<li
									key={proposal.n}
									className="flex items-start gap-2 rounded-lg border border-border p-2 text-xs"
								>
									<input
										type="checkbox"
										className="mt-0.5 size-4 accent-primary"
										checked={chosen.has(proposal.n)}
										aria-label={t("ui.editor.vertical_cuts.choose", {
											n: proposal.n,
										})}
										onChange={(event) => {
											const next = new Set(chosen);
											if (event.target.checked) next.add(proposal.n);
											else next.delete(proposal.n);
											setChosen(next);
										}}
									/>
									<span>
										<span className="font-mono font-medium [font-variant-numeric:tabular-nums]">
											{t("ui.editor.vertical_cuts.range", {
												start: clockLabel(proposal.start_ms),
												end: clockLabel(proposal.end_ms),
											})}
										</span>{" "}
										<span className="text-muted-foreground">
											{t("ui.editor.vertical_cuts.meta", {
												words: proposal.words,
												share: sharePercent(proposal.face_share),
											})}
										</span>
										<br />
										<span lang="en">
											<UntrustedText>{proposal.title}</UntrustedText>
										</span>
									</span>
								</li>
							))}
						</ul>
					)}
					<div className="flex gap-2">
						<Button
							type="button"
							size="sm"
							disabled={!canEdit || chosen.size === 0}
							onClick={() => void accept(phase.proposals, phase.mediaFile)}
						>
							<Check aria-hidden="true" />
							{t("ui.editor.vertical_cuts.accept", { count: chosen.size })}
						</Button>
						<Button
							type="button"
							size="sm"
							variant="outline"
							onClick={() => setPhase({ name: "idle" })}
						>
							{t("ui.editor.vertical_cuts.dismiss")}
						</Button>
					</div>
				</div>
			)}
			{phase.name === "creating" && (
				<p role="status" className="text-xs">
					{t("ui.editor.vertical_cuts.creating")}
				</p>
			)}
			{phase.name === "created" && (
				<p role="status" className="text-xs">
					{t("ui.editor.vertical_cuts.created", { count: phase.count })}
				</p>
			)}
			{phase.name === "failed" && (
				<div role="alert" className="text-xs">
					<p className="font-medium text-destructive">
						{errorHeadline(t, phase.code)}
					</p>
					<p lang="en">
						<UntrustedText>{phase.message}</UntrustedText>
					</p>
					{phase.hint !== undefined && (
						<p lang="en" className="text-muted-foreground">
							<UntrustedText>{phase.hint}</UntrustedText>
						</p>
					)}
				</div>
			)}

			{offer !== null && (
				<FaceInstallDialog
					offer={offer}
					onClose={() => setOffer(null)}
					onInstalled={() => {
						setOffer(null);
						void find();
					}}
				/>
			)}
		</Section>
	);
}

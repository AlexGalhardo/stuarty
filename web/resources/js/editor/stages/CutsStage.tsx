import {
	Check,
	CircleDashed,
	Clock,
	Hourglass,
	ScanFace,
	X,
} from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { useT } from "@/lib/i18n";
import type { CutsProposal } from "../api/types";
import { thumbnailUrl } from "../api/urls";
import FaceInstallDialog from "../inspector/panels/FaceInstallDialog";
import {
	clockLabel,
	sharePercent,
} from "../inspector/panels/verticalCutsModel";
import { useRuntime } from "../state/EditorContext";
import { errorHeadline } from "../ui/errorHeadline";
import { type CutsState, useCutsState } from "./useCuts";

// One proposal as a card (final design 27): the face badge, a vertical picture of the clip, its title, its range, its
// state and the two buttons. The picture is the thumbnail of the asset cropped to 9:16; the cut itself is made by the
// batch the service built, when the person accepts.
function ProposalCard({
	proposal,
	state,
}: {
	proposal: CutsProposal;
	state: CutsState;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const reviewing = state.reviewing?.n === proposal.n;
	return (
		<li
			data-proposal={proposal.n}
			aria-current={reviewing ? "true" : undefined}
			className={`flex flex-col gap-2 rounded-xl border bg-surface p-3 ${reviewing ? "border-accent bg-surface-selected" : "border-border"}`}
		>
			<button
				type="button"
				className="relative block aspect-[9/16] w-full overflow-hidden rounded-lg bg-surface-sunken"
				aria-label={t("ui.editor.cuts_stage.review", { n: proposal.n })}
				onClick={() => state.review(proposal.n)}
			>
				<img
					src={thumbnailUrl(runtime.projectId, state.assetId, 320)}
					alt=""
					className="size-full object-cover"
				/>
				<span className="absolute top-2 left-2 flex items-center gap-1 rounded-md bg-black/70 px-2 py-0.5 text-xs text-white">
					<ScanFace aria-hidden="true" className="size-3.5" />
					{t("ui.editor.cuts_stage.face", {
						share: sharePercent(proposal.face_share),
					})}
				</span>
			</button>
			<div className="flex flex-col gap-1">
				<span
					lang="en"
					className="text-[13px] font-medium"
					title={t("ui.editor.vertical_cuts.meta", {
						words: proposal.words,
						share: sharePercent(proposal.face_share),
					})}
				>
					<UntrustedText>{proposal.title}</UntrustedText>
				</span>
				<span className="flex items-center justify-between gap-2 text-xs">
					<span className="font-mono text-xs whitespace-nowrap text-muted-foreground tabular-nums">
						{t("ui.editor.vertical_cuts.range", {
							start: clockLabel(proposal.start_ms),
							end: clockLabel(proposal.end_ms),
						})}
					</span>
					<span
						className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-xs whitespace-nowrap ${reviewing ? "bg-accent-soft text-accent-text" : "border border-border text-muted-foreground"}`}
					>
						{reviewing ? (
							<Clock aria-hidden="true" className="size-3.5" />
						) : (
							<CircleDashed aria-hidden="true" className="size-3.5" />
						)}
						{reviewing
							? t("ui.editor.cuts_stage.in_review")
							: t("ui.editor.cuts_stage.proposed")}
					</span>
				</span>
			</div>
			<div className="grid grid-cols-2 gap-1">
				<Button
					type="button"
					variant="outline"
					className="h-8"
					onClick={() => state.refuse(proposal.n)}
				>
					<X aria-hidden="true" />
					{t("ui.editor.cuts_stage.refuse")}
				</Button>
				<Button
					type="button"
					className="h-8"
					disabled={!state.canEdit || state.busy}
					onClick={() => void state.accept(proposal.n)}
				>
					<Check aria-hidden="true" />
					{t("ui.editor.cuts_stage.accept")}
				</Button>
			</div>
		</li>
	);
}

// The vertical cuts stage (final design 27): in the place of the preview, the cuts the service proposes for the source
// video as cards, and before them the steps that find them (the face model, the face track). The agent proposes; only
// a person accepts, and nothing enters the timeline before that.
export default function CutsStage({ onClose }: { onClose(): void }): ReactNode {
	const t = useT();
	const state = useCutsState();
	if (state === null) return null;
	const { phase } = state;
	return (
		<section
			aria-label={t("ui.editor.cuts_stage.label")}
			data-stage="cuts"
			className="flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-auto p-6"
		>
			<div className="flex items-center justify-between gap-2">
				<h2 className="text-lg font-semibold">
					{phase.name === "review"
						? t("ui.editor.cuts_stage.title", { count: state.visible.length })
						: t("ui.editor.vertical_cuts.title")}
				</h2>
				<span className="flex items-center gap-2">
					{phase.name === "review" && (
						<span className="flex items-center gap-1 rounded-full bg-warning-soft px-3 py-1 text-sm text-warning-text">
							<Hourglass aria-hidden="true" className="size-4" />
							{t("ui.editor.cuts_stage.waiting")}
						</span>
					)}
					<Button type="button" variant="ghost" size="sm" onClick={onClose}>
						<X aria-hidden="true" />
						{t("ui.editor.stage.close")}
					</Button>
				</span>
			</div>

			{(phase.name === "idle" || phase.name === "failed") && (
				<div className="flex flex-col items-start gap-2">
					<p className="text-sm text-muted-foreground">
						{t("ui.editor.vertical_cuts.hint")}
					</p>
					<Button
						type="button"
						disabled={!state.canEdit || state.busy}
						onClick={() => void state.find()}
					>
						{t("ui.editor.vertical_cuts.find")}
					</Button>
				</div>
			)}
			{phase.name === "checking" && (
				<p role="status" className="text-sm">
					{t("ui.editor.vertical_cuts.checking")}
				</p>
			)}
			{phase.name === "tracking" && (
				<div role="status" className="max-w-md">
					<p className="text-sm">{t("ui.editor.vertical_cuts.tracking")}</p>
					<Progress
						value={state.percent}
						aria-label={t("ui.editor.vertical_cuts.progress")}
						className="my-1 h-2"
					/>
					<p className="text-xs [font-variant-numeric:tabular-nums]">
						{state.percent}%
					</p>
				</div>
			)}
			{phase.name === "proposing" && (
				<p role="status" className="text-sm">
					{t("ui.editor.vertical_cuts.proposing")}
				</p>
			)}
			{phase.name === "review" &&
				(state.visible.length === 0 ? (
					<p role="status" className="text-sm">
						{phase.proposals.length === 0
							? t("ui.editor.vertical_cuts.none")
							: t("ui.editor.cuts_stage.all_decided")}
					</p>
				) : (
					<ul
						className="grid grid-cols-3 gap-3"
						aria-label={t("ui.editor.vertical_cuts.proposals")}
					>
						{state.visible.map((proposal) => (
							<ProposalCard
								key={proposal.n}
								proposal={proposal}
								state={state}
							/>
						))}
					</ul>
				))}
			{state.created > 0 && (
				<p role="status" className="text-sm">
					{t("ui.editor.vertical_cuts.created", { count: state.created })}
				</p>
			)}
			{phase.name === "failed" && (
				<div role="alert" className="text-sm">
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
			{state.offer !== null && (
				<FaceInstallDialog
					offer={state.offer}
					onClose={state.closeOffer}
					onInstalled={state.installed}
				/>
			)}
		</section>
	);
}

import { type ReactNode, useState } from "react";

import { type Failure, FailureNote } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { useLocale, useT } from "@/lib/i18n";
import { localizeError } from "@/lib/localizeError";
import type {
	Recording,
	RecordingProposal,
	RecordingSourceInfo,
	RecordingSourcesResult,
} from "../api/types";
import { blockedKey, type Capability, codecKey } from "./browserCapture";
import {
	type CaptureChoice,
	endKey,
	type RecorderState,
} from "./browserRecorder";
import {
	DEFAULT_FPS,
	FPS_CHOICES,
	formatClock,
	formatClockTenths,
	formatMegabytes,
	meterValue,
	NO_EXTRAS,
	needsDecision,
	pendingProposals,
	type RecordView,
	type StartOptions,
	unavailableKey,
} from "./recordFlow";
import type { RecordFlow } from "./useRecordFlow";

// The screens of the record dialog (SDD-025 section 12). It draws what the service said and sends the commands of
// the person; the state of a recording, the elapsed time and what a capture may do are the service's.
export default function RecordPanel({
	flow,
	onClose,
}: {
	flow: RecordFlow;
	onClose: () => void;
}): ReactNode {
	return (
		<div className="flex flex-col gap-3" data-record-step={flow.view.step}>
			<Step view={flow.view} flow={flow} onClose={onClose} />
			<FailureNote failure={flow.failure as Failure | null} />
		</div>
	);
}

function Step({
	view,
	flow,
	onClose,
}: {
	view: RecordView;
	flow: RecordFlow;
	onClose: () => void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	switch (view.step) {
		case "loading":
			return (
				<p role="status" aria-busy="true" className="text-sm">
					{t("ui.editor.record.loading")}
				</p>
			);
		case "unavailable":
			return (
				<>
					<p role="status" className="text-sm" data-record-unavailable="">
						{t(unavailableKey(view.reason))}
					</p>
					<Button type="button" variant="outline" onClick={onClose}>
						{t("ui.editor.record.close")}
					</Button>
				</>
			);
		case "pick":
			return (
				<>
					<Decided notice={flow.notice} />
					<ProposalList recordings={view.proposals ?? []} flow={flow} />
					<PreparedList
						prepared={view.prepared ?? []}
						sources={view.sources}
						flow={flow}
					/>
					{view.kept.length > 0 && (
						<Kept kept={view.kept} busy={flow.busy} flow={flow} />
					)}
					<Picker
						sources={view.sources}
						busy={flow.busy}
						onStart={(kind, id, name, fps, extras) =>
							void flow.start(kind, id, name, fps, extras)
						}
					/>
				</>
			);
		case "browser":
			return (
				<>
					<Decided notice={flow.notice} />
					<ProposalList recordings={view.proposals ?? []} flow={flow} />
					{view.kept.length > 0 && (
						<Kept kept={view.kept} busy={flow.busy} flow={flow} />
					)}
					<BrowserPicker
						capability={view.capability}
						busy={flow.busy}
						onStart={(choice) => void flow.startBrowser(choice)}
						onClose={onClose}
					/>
				</>
			);
		case "browser_live":
			return (
				<BrowserLive
					state={view.state}
					busy={flow.busy}
					onStop={() => void flow.stopBrowser()}
				/>
			);
		case "interrupted":
			return (
				<>
					<p role="status" className="text-sm" data-record-interrupted="">
						{t("ui.editor.record.browser.interrupted")}
					</p>
					<p className="text-sm tabular-nums">
						{t("ui.editor.record.browser.uploaded", {
							size: formatMegabytes(view.recording.bytes),
						})}
					</p>
					<div className="flex flex-wrap gap-2">
						<Button
							type="button"
							disabled={flow.busy}
							data-record-finish-interrupted=""
							onClick={() => void flow.stop()}
						>
							{t("ui.editor.record.browser.finish")}
						</Button>
						<Button
							type="button"
							variant="ghost"
							disabled={flow.busy}
							data-record-delete=""
							onClick={() => void flow.remove()}
						>
							{t("ui.editor.record.delete")}
						</Button>
					</div>
				</>
			);
		case "recording":
			return (
				<>
					<p
						className="flex items-center gap-2 text-sm font-medium"
						role="status"
					>
						<span
							aria-hidden="true"
							className="size-2.5 rounded-full bg-destructive"
						/>
						{view.recording.state === "finishing"
							? t("ui.editor.record.finishing")
							: view.recording.state === "paused"
								? t("ui.editor.record.paused")
								: t("ui.editor.record.recording")}
						<span data-record-clock="" className="tabular-nums">
							{t("ui.editor.record.elapsed", {
								time: formatClock(view.recording.duration_ms),
							})}
						</span>
					</p>
					<Meters recording={view.recording} />
					<div className="flex flex-wrap gap-2">
						{view.recording.state === "paused" ? (
							<Button
								type="button"
								disabled={flow.busy}
								data-record-resume=""
								onClick={() => void flow.resume()}
							>
								{t("ui.editor.record.resume")}
							</Button>
						) : (
							<Button
								type="button"
								variant="outline"
								disabled={flow.busy || view.recording.state !== "recording"}
								data-record-pause=""
								onClick={() => void flow.pause()}
							>
								{t("ui.editor.record.pause")}
							</Button>
						)}
						<Button
							type="button"
							variant="destructive"
							disabled={
								flow.busy ||
								(view.recording.state !== "recording" &&
									view.recording.state !== "paused")
							}
							data-record-stop=""
							onClick={() => void flow.stop()}
						>
							{t("ui.editor.record.stop")}
						</Button>
					</div>
				</>
			);
		case "review":
			return (
				<>
					<h3 className="text-sm font-medium">
						{t("ui.editor.record.review_title")}
					</h3>
					<p className="text-sm tabular-nums">
						{t("ui.editor.record.review_length", {
							time: formatClock(view.recording.duration_ms),
						})}
					</p>
					{view.recording.dropped > 0 && (
						<p className="text-xs text-muted-foreground">
							{t("ui.editor.record.review_dropped", {
								count: view.recording.dropped,
							})}
						</p>
					)}
					<p className="text-sm" data-record-notice="">
						{t("ui.editor.record.review_notice")}
					</p>
					<div className="flex flex-wrap gap-2">
						<Button
							type="button"
							disabled={flow.busy}
							data-record-add=""
							onClick={() => void flow.confirm()}
						>
							{t("ui.editor.record.add")}
						</Button>
						<Button
							type="button"
							variant="outline"
							disabled={flow.busy}
							data-record-not-now=""
							onClick={() => void flow.dismiss()}
						>
							{t("ui.editor.record.not_now")}
						</Button>
						<Button
							type="button"
							variant="ghost"
							disabled={flow.busy}
							data-record-delete=""
							onClick={() => void flow.remove()}
						>
							{t("ui.editor.record.delete")}
						</Button>
					</div>
				</>
			);
		case "added":
			return (
				<>
					<p role="status" className="text-sm">
						{flow.placed
							? t("ui.editor.record.placed")
							: t("ui.editor.record.added")}
					</p>
					<Decided notice={flow.notice} />
					{needsDecision(view.recording) && (
						<ProposalList recordings={[view.recording]} flow={flow} />
					)}
					<div className="flex flex-wrap gap-2">
						{!flow.placed && (
							<Button
								type="button"
								disabled={flow.busy}
								data-record-place=""
								onClick={() => void flow.place()}
							>
								{t("ui.editor.record.place")}
							</Button>
						)}
						<Button type="button" variant="outline" onClick={onClose}>
							{t("ui.editor.record.close")}
						</Button>
					</div>
				</>
			);
		case "failed": {
			const recErr =
				view.recording.error !== undefined
					? localizeError(t, view.recording.error, { locale })
					: null;
			return (
				<>
					<p role="alert" className="text-sm font-medium text-destructive">
						{t("ui.editor.record.failed")}
					</p>
					{recErr !== null && (
						<p lang={recErr.localized ? undefined : "en"} className="text-xs">
							<UntrustedText>{recErr.message}</UntrustedText>
						</p>
					)}
					<div className="flex gap-2">
						<Button type="button" onClick={flow.reset}>
							{t("ui.editor.record.again")}
						</Button>
						<Button type="button" variant="outline" onClick={onClose}>
							{t("ui.editor.record.close")}
						</Button>
					</div>
				</>
			);
		}
		case "kept":
			return (
				<>
					<p role="status" className="text-sm" data-record-kept-note="">
						{t("ui.editor.record.kept")}
					</p>
					<div className="flex gap-2">
						<Button type="button" onClick={flow.reset}>
							{t("ui.editor.record.again")}
						</Button>
						<Button type="button" variant="outline" onClick={onClose}>
							{t("ui.editor.record.close")}
						</Button>
					</div>
				</>
			);
		case "gone":
			return (
				<>
					<p role="status" className="text-sm">
						{t("ui.editor.record.gone")}
					</p>
					<div className="flex gap-2">
						<Button type="button" onClick={flow.reset}>
							{t("ui.editor.record.again")}
						</Button>
						<Button type="button" variant="outline" onClick={onClose}>
							{t("ui.editor.record.close")}
						</Button>
					</div>
				</>
			);
	}
}

// The recordings a person set aside with "Not now": still out of the project and out of sight of agents, until the
// person adds or deletes them here (SDD-025 section 4).
// What the service said about the last decision on a proposal of cuts.
function Decided({ notice }: { notice: string | null }): ReactNode {
	if (notice === null) return null;
	return (
		<p role="status" className="text-sm" data-record-decided="">
			{notice}
		</p>
	);
}

// The plans an agent or a person prepared (SDD-025 section 19.1). Nothing records until a person presses Record:
// the button here is that press, and it is the only way a prepared recording starts.
// The person sees what pressing Record captures: the title the helper gives the source of the plan (never a name
// the agent wrote) and every track besides the screen.
function PreparedList({
	prepared,
	sources,
	flow,
}: {
	prepared: Recording[];
	sources: RecordingSourcesResult;
	flow: RecordFlow;
}): ReactNode {
	const t = useT();
	if (prepared.length === 0) return null;
	const titleOf = (r: Recording): string => {
		const list =
			r.source.kind === "window" ? sources.windows : sources.displays;
		return list.find((s) => s.id === r.plan?.source_id)?.name ?? r.source.kind;
	};
	const extras = (r: Recording): string[] => [
		...(r.plan?.microphone ? [t("ui.editor.record.microphone")] : []),
		...(r.plan?.camera ? [t("ui.editor.record.camera")] : []),
		...(r.plan?.system_audio ? [t("ui.editor.record.system_audio")] : []),
		...(r.plan?.events ? [t("ui.editor.record.events")] : []),
	];
	return (
		<section className="flex flex-col gap-2" data-record-prepared="">
			<h3 className="text-xs font-medium text-muted-foreground uppercase">
				{t("ui.editor.record.prepared_title")}
			</h3>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.record.prepared_note")}
			</p>
			<ul className="flex flex-col gap-2">
				{prepared.map((r) => (
					<li
						key={r.recording_id}
						className="flex flex-col gap-1 rounded-md border p-2 text-sm"
						data-record-prepared-item={r.recording_id}
					>
						<span className="min-w-0 truncate">
							<UntrustedText>{titleOf(r)}</UntrustedText>
						</span>
						{extras(r).length > 0 && (
							<ul
								className="flex flex-col text-xs"
								data-record-prepared-tracks=""
							>
								{extras(r).map((label) => (
									<li key={label}>{label}</li>
								))}
							</ul>
						)}
						<span className="text-xs text-muted-foreground">
							{t("ui.editor.record.prepared_by", { actor: r.actor })}
						</span>
						{r.plan?.notes !== undefined && r.plan.notes !== "" && (
							<p
								lang="en"
								className="text-xs whitespace-pre-line"
								data-record-prepared-notes=""
							>
								<UntrustedText>{r.plan.notes}</UntrustedText>
							</p>
						)}
						<div className="flex flex-wrap gap-2">
							<Button
								type="button"
								size="sm"
								disabled={flow.busy}
								data-record-prepared-start={r.recording_id}
								onClick={() => void flow.startPrepared(r.recording_id)}
							>
								{t("ui.editor.record.prepared_start")}
							</Button>
							<Button
								type="button"
								size="sm"
								variant="ghost"
								disabled={flow.busy}
								data-record-prepared-delete={r.recording_id}
								onClick={() => void flow.remove(r.recording_id)}
							>
								{t("ui.editor.record.delete")}
							</Button>
						</div>
					</li>
				))}
			</ul>
		</section>
	);
}

// The cuts that wait for a person (SDD-025 section 19): one card for each pending proposal.
function ProposalList({
	recordings,
	flow,
}: {
	recordings: Recording[];
	flow: RecordFlow;
}): ReactNode {
	const t = useT();
	const cards = recordings.flatMap((r) =>
		pendingProposals(r).map((p) => ({ r, p })),
	);
	if (cards.length === 0) return null;
	return (
		<section className="flex flex-col gap-2" data-record-proposals="">
			<h3 className="text-xs font-medium text-muted-foreground uppercase">
				{t("ui.editor.record.proposals_title")}
			</h3>
			{cards.map(({ r, p }) => (
				<ProposalCard
					key={p.proposal_id}
					recording={r}
					proposal={p}
					flow={flow}
				/>
			))}
		</section>
	);
}

// One proposal: every range with its reason and a yes or no of its own, and one button that sends them all. The
// choices are the person's; the service applies the accepted ones as one batch that one undo restores.
function ProposalCard({
	recording,
	proposal,
	flow,
}: {
	recording: Recording;
	proposal: RecordingProposal;
	flow: RecordFlow;
}): ReactNode {
	const t = useT();
	const [accept, setAccept] = useState<boolean[]>(() =>
		proposal.ranges.map(() => true),
	);
	const cut = accept.filter(Boolean).length;
	return (
		<div
			className="flex flex-col gap-2 rounded-md border p-2"
			data-record-proposal={proposal.proposal_id}
		>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.record.proposals_by", { actor: proposal.actor })}
			</p>
			<ul className="flex flex-col gap-1">
				{proposal.ranges.map((range, i) => (
					<li
						key={`${range.from_ms}-${range.to_ms}`}
						className="flex flex-wrap items-center gap-2 text-sm"
						data-record-range={i}
					>
						<span className="tabular-nums">
							{t("ui.editor.record.proposals_range", {
								from: formatClockTenths(range.from_ms),
								to: formatClockTenths(range.to_ms),
							})}
						</span>
						<span lang="en" className="min-w-0 truncate text-xs">
							<UntrustedText>{range.reason}</UntrustedText>
						</span>
						<Button
							type="button"
							size="sm"
							variant={accept[i] ? "default" : "outline"}
							aria-pressed={accept[i]}
							disabled={flow.busy}
							data-record-range-accept={i}
							onClick={() =>
								setAccept((a) => a.map((v, j) => (j === i ? true : v)))
							}
						>
							{t("ui.editor.record.proposals_accept")}
						</Button>
						<Button
							type="button"
							size="sm"
							variant={accept[i] ? "outline" : "default"}
							aria-pressed={!accept[i]}
							disabled={flow.busy}
							data-record-range-reject={i}
							onClick={() =>
								setAccept((a) => a.map((v, j) => (j === i ? false : v)))
							}
						>
							{t("ui.editor.record.proposals_reject")}
						</Button>
					</li>
				))}
			</ul>
			<p className="text-xs" data-record-proposal-count="">
				{t("ui.editor.record.proposals_count", {
					cut,
					total: proposal.ranges.length,
				})}
			</p>
			<Button
				type="button"
				size="sm"
				disabled={flow.busy}
				data-record-proposal-apply={proposal.proposal_id}
				onClick={() =>
					void flow.decide(recording.recording_id, proposal.proposal_id, accept)
				}
			>
				{t("ui.editor.record.proposals_apply")}
			</Button>
		</div>
	);
}

function Kept({
	kept,
	busy,
	flow,
}: {
	kept: Recording[];
	busy: boolean;
	flow: RecordFlow;
}): ReactNode {
	const t = useT();
	return (
		<section className="flex flex-col gap-2" data-record-kept="">
			<h3 className="text-xs font-medium text-muted-foreground uppercase">
				{t("ui.editor.record.kept_title")}
			</h3>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.record.review_notice")}
			</p>
			<ul className="flex flex-col gap-1">
				{kept.map((r) => (
					<li
						key={r.recording_id}
						className="flex flex-wrap items-center gap-2 text-sm"
					>
						<span className="min-w-0 truncate tabular-nums">
							{t("ui.editor.record.review_length", {
								time: formatClock(r.duration_ms),
							})}
						</span>
						<Button
							type="button"
							size="sm"
							disabled={busy}
							data-record-kept-add={r.recording_id}
							onClick={() => void flow.confirm(r.recording_id)}
						>
							{t("ui.editor.record.add")}
						</Button>
						<Button
							type="button"
							size="sm"
							variant="ghost"
							disabled={busy}
							data-record-kept-delete={r.recording_id}
							onClick={() => void flow.remove(r.recording_id)}
						>
							{t("ui.editor.record.delete")}
						</Button>
					</li>
				))}
			</ul>
		</section>
	);
}

interface Choice {
	kind: "display" | "window";
	source: RecordingSourceInfo;
}

// The picker of a recording made in the browser (SDD-025 section 18.1): what this browser can do, said before the
// person starts, and, when it cannot record here, why, with Start disabled.
function BrowserPicker({
	capability,
	busy,
	onStart,
	onClose,
}: {
	capability: Capability;
	busy: boolean;
	onStart: (choice: CaptureChoice) => void;
	onClose: () => void;
}): ReactNode {
	const t = useT();
	const [choice, setChoice] = useState<CaptureChoice>({
		microphone: true,
		camera: false,
		displayAudio: false,
	});

	if (!capability.ok) {
		return (
			<>
				<h3 className="text-sm font-medium">
					{t("ui.editor.record.browser.title")}
				</h3>
				<p
					role="alert"
					className="text-sm"
					data-record-browser-blocked={capability.reason}
				>
					{t(blockedKey(capability.reason))}
				</p>
				<div className="flex flex-wrap gap-2">
					<Button type="button" disabled data-record-start="">
						{t("ui.editor.record.start")}
					</Button>
					<Button type="button" variant="outline" onClick={onClose}>
						{t("ui.editor.record.close")}
					</Button>
				</div>
			</>
		);
	}

	return (
		<form
			className="flex flex-col gap-3"
			data-record-browser=""
			onSubmit={(event) => {
				event.preventDefault();
				onStart(choice);
			}}
		>
			<h3 className="text-sm font-medium">
				{t("ui.editor.record.browser.title")}
			</h3>
			<p className="text-sm">{t("ui.editor.record.browser.intro")}</p>
			<p className="text-sm" data-record-browser-codec={capability.label}>
				{t(codecKey(capability.label))}
			</p>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.record.browser.note_prompts")}
			</p>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.record.browser.note_audio")}
			</p>
			<fieldset className="flex flex-col gap-2">
				<legend className="text-xs font-medium text-muted-foreground uppercase">
					{t("ui.editor.record.browser.sound_title")}
				</legend>
				<label className="flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						checked={choice.microphone && capability.microphone}
						disabled={!capability.microphone}
						data-record-browser-microphone=""
						onChange={(event) =>
							setChoice({ ...choice, microphone: event.target.checked })
						}
					/>
					{t("ui.editor.record.browser.microphone")}
				</label>
				<label className="flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						checked={choice.displayAudio}
						data-record-browser-display-audio=""
						onChange={(event) =>
							setChoice({ ...choice, displayAudio: event.target.checked })
						}
					/>
					{t("ui.editor.record.browser.display_audio")}
				</label>
				<label className="flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						checked={choice.camera && capability.camera}
						disabled={!capability.camera}
						data-record-browser-camera=""
						onChange={(event) =>
							setChoice({ ...choice, camera: event.target.checked })
						}
					/>
					{t("ui.editor.record.browser.camera")}
				</label>
				{!capability.camera && (
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.record.browser.no_user_media")}
					</p>
				)}
			</fieldset>
			<Button type="submit" disabled={busy} data-record-start="">
				{busy ? t("ui.editor.record.starting") : t("ui.editor.record.start")}
			</Button>
		</form>
	);
}

// A recording this tab is making in the browser: the clock of the recorder, what the browser is sharing, how much
// the server confirmed, and, when it happens, that the connection dropped or a limit was reached.
function BrowserLive({
	state,
	busy,
	onStop,
}: {
	state: RecorderState;
	busy: boolean;
	onStop: () => void;
}): ReactNode {
	const t = useT();
	const sharing = (
		[
			["screen", true, "ui.editor.record.browser.shared_screen"],
			[
				"microphone",
				state.shared.microphone,
				"ui.editor.record.browser.shared_microphone",
			],
			[
				"display_audio",
				state.shared.displayAudio,
				"ui.editor.record.browser.shared_display_audio",
			],
			["camera", state.shared.camera, "ui.editor.record.browser.shared_camera"],
		] as const
	).filter(([, on]) => on);
	const ending = endKey(state.upload.end);
	const recording = state.phase === "recording";
	return (
		<>
			<p
				className="flex items-center gap-2 text-sm font-medium"
				role="status"
				data-record-browser-live={state.phase}
			>
				<span
					aria-hidden="true"
					className="size-2.5 rounded-full bg-destructive"
				/>
				{recording
					? t("ui.editor.record.recording")
					: t("ui.editor.record.finishing")}
				<span data-record-clock="" className="tabular-nums">
					{t("ui.editor.record.elapsed", {
						time: formatClock(state.elapsedMs),
					})}
				</span>
			</p>
			<p
				className="text-xs text-muted-foreground"
				data-record-browser-shared=""
			>
				{t("ui.editor.record.browser.shared_title")}{" "}
				{sharing.map(([key, , label], i) => (
					<span key={key} data-record-browser-shared-item={key}>
						{i > 0 ? ", " : ""}
						{t(label)}
					</span>
				))}
			</p>
			<p className="text-xs tabular-nums" data-record-browser-bytes="">
				{t("ui.editor.record.browser.uploaded", {
					size: formatMegabytes(state.upload.bytes),
				})}
			</p>
			{state.upload.phase === "retrying" && (
				<p role="status" className="text-xs" data-record-browser-retrying="">
					{t("ui.editor.record.browser.retrying")}
				</p>
			)}
			{state.stoppedBy === "browser" && (
				<p className="text-xs" data-record-browser-stopped-by-browser="">
					{t("ui.editor.record.browser.stopped_by_browser")}
				</p>
			)}
			{ending !== null && (
				<p role="alert" className="text-xs" data-record-browser-ended="">
					{t(ending)}
				</p>
			)}
			<div className="flex flex-wrap gap-2">
				<Button
					type="button"
					variant="destructive"
					disabled={busy || !recording}
					data-record-stop=""
					onClick={onStop}
				>
					{t("ui.editor.record.stop")}
				</Button>
			</div>
		</>
	);
}

// The level of each sound track while the recording runs, drawn from the peaks the service reports.
function Meters({ recording }: { recording: Recording }): ReactNode {
	const t = useT();
	const kinds = new Set((recording.tracks ?? []).map((tr) => tr.kind));
	const rows = [
		[
			"microphone",
			recording.levels?.microphone,
			"ui.editor.record.meter_microphone",
		],
		["system", recording.levels?.system, "ui.editor.record.meter_system"],
	] as const;
	const shown = rows.filter(([kind]) => kinds.has(kind));
	if (shown.length === 0) return null;
	return (
		<div className="flex flex-col gap-1" data-record-meters="">
			{shown.map(([kind, level, label]) => (
				<label key={kind} className="flex items-center gap-2 text-xs">
					<span className="w-40 shrink-0">{t(label)}</span>
					<meter
						className="h-2 w-full"
						min={0}
						max={1000}
						value={meterValue(level)}
						aria-label={t(label)}
						data-record-meter={kind}
					/>
				</label>
			))}
			{kinds.has("system") && (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.record.system_note")}
				</p>
			)}
		</div>
	);
}

function Picker({
	sources,
	busy,
	onStart,
}: {
	sources: RecordingSourcesResult;
	busy: boolean;
	onStart: (
		kind: "display" | "window",
		id: string,
		name: string,
		fps: number,
		extras: StartOptions,
	) => void;
}): ReactNode {
	const t = useT();
	const { displays, windows } = sources;
	const [extras, setExtras] = useState<StartOptions>(NO_EXTRAS);
	const choices: Choice[] = [
		...displays.map((source): Choice => ({ kind: "display", source })),
		...windows.map((source): Choice => ({ kind: "window", source })),
	];
	const [picked, setPicked] = useState<string>(
		choices.length > 0 ? `${choices[0]?.kind}:${choices[0]?.source.id}` : "",
	);
	const [fps, setFps] = useState<number>(DEFAULT_FPS);

	if (choices.length === 0) {
		return <p className="text-sm">{t("ui.editor.record.none")}</p>;
	}

	const chosen = choices.find((c) => `${c.kind}:${c.source.id}` === picked);

	return (
		<form
			className="flex flex-col gap-3"
			onSubmit={(event) => {
				event.preventDefault();
				if (chosen === undefined) return;
				onStart(
					chosen.kind,
					chosen.source.id,
					chosen.kind === "window" ? chosen.source.name : "",
					fps,
					extras,
				);
			}}
		>
			{(
				[
					["display", displays, "ui.editor.record.displays"],
					["window", windows, "ui.editor.record.windows"],
				] as const
			).map(([kind, list, label]) =>
				list.length === 0 ? null : (
					<fieldset key={kind} className="flex flex-col gap-1">
						<legend className="text-xs font-medium text-muted-foreground uppercase">
							{t(label)}
						</legend>
						{list.map((source) => {
							const value = `${kind}:${source.id}`;
							return (
								<label key={value} className="flex items-center gap-2 text-sm">
									<input
										type="radio"
										name="record-source"
										value={value}
										checked={picked === value}
										onChange={() => setPicked(value)}
									/>
									<span className="min-w-0 truncate">
										<UntrustedText>{source.name}</UntrustedText>
									</span>
									<span className="text-xs text-muted-foreground tabular-nums">
										{`${source.width}×${source.height}`}
									</span>
								</label>
							);
						})}
					</fieldset>
				),
			)}
			<div className="flex items-center gap-2">
				<Label htmlFor="record-fps">{t("ui.editor.record.fps")}</Label>
				<select
					id="record-fps"
					className="h-8 rounded-md border bg-background px-2 text-sm"
					value={fps}
					onChange={(event) => setFps(Number(event.target.value))}
				>
					{FPS_CHOICES.map((n) => (
						<option key={n} value={n}>
							{n}
						</option>
					))}
				</select>
			</div>
			<fieldset className="flex flex-col gap-2" data-record-extras="">
				<legend className="text-xs font-medium text-muted-foreground uppercase">
					{t("ui.editor.record.sound_title")}
				</legend>
				<div className="flex items-center gap-2">
					<Label htmlFor="record-microphone">
						{t("ui.editor.record.microphone")}
					</Label>
					<select
						id="record-microphone"
						className="h-8 min-w-0 rounded-md border bg-background px-2 text-sm"
						value={extras.microphone}
						onChange={(event) =>
							setExtras({ ...extras, microphone: event.target.value })
						}
					>
						<option value="">{t("ui.editor.record.microphone_none")}</option>
						{sources.microphones.length > 0 && (
							<option value="default">
								{t("ui.editor.record.microphone_default")}
							</option>
						)}
						{sources.microphones.map((m) => (
							<option key={m.id} value={m.id}>
								{m.name}
							</option>
						))}
					</select>
				</div>
				{sources.system_audio && (
					<label className="flex items-center gap-2 text-sm">
						<input
							type="checkbox"
							checked={extras.systemAudio}
							data-record-system-audio=""
							onChange={(event) =>
								setExtras({ ...extras, systemAudio: event.target.checked })
							}
						/>
						{t("ui.editor.record.system_audio")}
					</label>
				)}
				{sources.cameras.length === 0 ? (
					<p className="text-xs text-muted-foreground" data-record-no-camera="">
						{t("ui.editor.record.camera_missing")}
					</p>
				) : (
					<div className="flex items-center gap-2">
						<Label htmlFor="record-camera">
							{t("ui.editor.record.camera")}
						</Label>
						<select
							id="record-camera"
							className="h-8 min-w-0 rounded-md border bg-background px-2 text-sm"
							value={extras.camera}
							onChange={(event) =>
								setExtras({ ...extras, camera: event.target.value })
							}
						>
							<option value="">{t("ui.editor.record.camera_none")}</option>
							{sources.cameras.map((c) => (
								<option key={c.id} value={c.id}>
									{c.name}
								</option>
							))}
						</select>
					</div>
				)}
				<label className="flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						checked={extras.events}
						data-record-events=""
						onChange={(event) =>
							setExtras({ ...extras, events: event.target.checked })
						}
					/>
					{t("ui.editor.record.events")}
				</label>
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.record.events_note")}
				</p>
			</fieldset>
			<Button
				type="submit"
				disabled={busy || chosen === undefined}
				data-record-start=""
			>
				{busy ? t("ui.editor.record.starting") : t("ui.editor.record.start")}
			</Button>
		</form>
	);
}

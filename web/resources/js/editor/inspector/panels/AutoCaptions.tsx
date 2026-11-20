import { type ReactNode, useEffect, useRef, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { useLocale, useT } from "@/lib/i18n";
import { ApiError } from "../../api/errors";
import {
	captionFromTranscriptOperations,
	type Operations,
} from "../../commands/builders";
import { newId } from "../../commands/ids";
import { localizeDecimalInput, normalizeTimeInput } from "../../commands/units";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useStore } from "../../state/createStore";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import { selectOrderedTrackIds } from "../../state/selectors";
import { errorHeadline } from "../../ui/errorHeadline";
import { trackLabel } from "../../ui/trackLabel";
import Section from "../Section";
import InstallDialog from "./InstallDialog";
import {
	defaultCaptionTrack,
	hasTranscript,
	NEW_TRACK,
	type Unavailable,
	unavailableFrom,
} from "./transcriptModel";

const POLL_AFTER_MS = 5000;
const POLL_EVERY_MS = 2000;

type Phase =
	| { name: "idle" }
	| { name: "checking" }
	| { name: "transcribing"; jobId: string }
	| { name: "failed"; code: string; message: string; hint?: string };

const wholeNumber = (draft: string): number | null => {
	const text = draft.trim();
	if (!/^\d+$/.test(text)) return null;
	const value = Number(text);
	return value > 0 ? value : null;
};

// The Auto captions section of SDD-012 sections 12.4 and 12.5: Generate starts a
// transcription when the asset has no transcript, follows it from the job
// events, and sends `caption.from_transcript` once; "Re-sync captions" sends
// `caption.retime_from_clip` for the captions linked to the clip. The words come
// from the service, never from the browser.
export default function AutoCaptions({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const runtime = useRuntime();
	const actions = useActions();
	const canEdit = useCanEdit();
	const assetId = useDocumentState((s) => s.document?.items[itemId]?.asset);
	const hasAudio = useDocumentState((s) => {
		const asset =
			assetId === undefined ? undefined : s.document?.assets[assetId];
		return asset?.kind === "video" && asset.has_audio !== false;
	});
	const orderedIds = useDocumentState(selectOrderedTrackIds);
	const tracks = useDocumentState((s) => s.document?.tracks);
	const linked = useDocumentState((s) =>
		Object.values(s.document?.items ?? {}).some(
			(item) => item.type === "caption" && item.linked_item === itemId,
		),
	);
	const [chosen, setChosen] = useState<string | null>(null);
	const [maxWords, setMaxWords] = useState("5");
	const [maxChars, setMaxChars] = useState("28");
	const [maxGap, setMaxGap] = useState("0.6s");
	const [replace, setReplace] = useState(true);
	const [phase, setPhase] = useState<Phase>({ name: "idle" });
	const [unavailable, setUnavailable] = useState<Unavailable | null>(null);
	const [installOpen, setInstallOpen] = useState(false);
	const pending = useRef<Operations | null>(null);
	const lastChange = useRef(0);

	const jobId = phase.name === "transcribing" ? phase.jobId : null;
	const job = useStore(runtime.jobs, (s) =>
		jobId === null ? undefined : s.jobs[jobId],
	);

	// The job states of the transcription: success sends the captions once.
	useEffect(() => {
		if (job === undefined) return;
		lastChange.current = Date.now();
		if (job.state === "succeeded") {
			const ops = pending.current;
			pending.current = null;
			setPhase({ name: "idle" });
			if (ops !== null) void actions.generateCaptions(ops);
		} else if (job.state === "failed" || job.state === "cancelled") {
			pending.current = null;
			if (job.error?.code === "E_TRANSCRIBE_UNAVAILABLE") {
				setPhase({ name: "idle" });
				setUnavailable(unavailableFrom(job.error));
				setInstallOpen(true);
			} else if (job.error !== undefined) {
				setPhase({
					name: "failed",
					code: job.error.code,
					message: job.error.message,
					hint: job.error.hint,
				});
			} else {
				setPhase({ name: "idle" });
			}
		}
	}, [job, actions]);

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

	if (!hasAudio || assetId === undefined || tracks === undefined) return null;

	const track = chosen ?? defaultCaptionTrack(orderedIds, tracks);
	const words = wholeNumber(maxWords);
	const chars = wholeNumber(maxChars);
	const gap = normalizeTimeInput(localizeDecimalInput(maxGap, locale));
	const valid = words !== null && chars !== null && gap.ok;
	const busy = phase.name === "checking" || phase.name === "transcribing";

	async function generate(): Promise<void> {
		if (words === null || chars === null || !gap.ok || assetId === undefined) {
			return;
		}
		const ops = captionFromTranscriptOperations({
			clipId: itemId,
			trackId: track === NEW_TRACK ? null : track,
			newTrackId: () => newId("trk"),
			trackName: t("ui.editor.auto_captions.track_name"),
			maxWords: words,
			maxChars: chars,
			maxGap: gap.value,
			replace,
		});
		setUnavailable(null);
		setPhase({ name: "checking" });
		try {
			const inspected = await runtime.client.inspectMedia(
				runtime.projectId,
				assetId,
				{ sections: ["transcript"], limit: 1 },
			);
			if (hasTranscript(inspected)) {
				setPhase({ name: "idle" });
				void actions.generateCaptions(ops);
				return;
			}
			const started = await runtime.client.startTranscribe(
				runtime.projectId,
				assetId,
			);
			pending.current = ops;
			runtime.jobsWriter.upsert(started);
			setPhase({ name: "transcribing", jobId: started.job_id });
		} catch (error) {
			if (!(error instanceof ApiError)) throw error;
			if (error.code === "E_TRANSCRIBE_UNAVAILABLE") {
				setPhase({ name: "idle" });
				setUnavailable(unavailableFrom(error));
				setInstallOpen(true);
				return;
			}
			setPhase({
				name: "failed",
				code: error.code,
				message: error.message,
				hint: error.hint,
			});
		}
	}

	const percent = Math.round((job?.progress ?? 0) * 100);

	return (
		<Section title={t("ui.editor.auto_captions.title")}>
			<div className="grid grid-cols-[96px_1fr] items-center gap-x-2 gap-y-1">
				<Label htmlFor={`auto-track-${itemId}`} className="text-xs">
					{t("ui.editor.auto_captions.track")}
				</Label>
				<select
					id={`auto-track-${itemId}`}
					value={track}
					disabled={!canEdit || busy}
					className="h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
					onChange={(event) => setChosen(event.target.value)}
				>
					<option value={NEW_TRACK}>
						{t("ui.editor.auto_captions.new_track")}
					</option>
					{orderedIds.flatMap((id) => {
						const candidate = tracks[id];
						if (candidate?.kind !== "visual") return [];
						return [
							<option key={id} value={id}>
								{candidate.name === undefined || candidate.name === ""
									? trackLabel(id, orderedIds, (other) =>
											tracks[other]?.kind === "audio" ? "audio" : "visual",
										)
									: candidate.name}
							</option>,
						];
					})}
				</select>
				<Label htmlFor={`auto-words-${itemId}`} className="text-xs">
					{t("ui.editor.auto_captions.max_words")}
				</Label>
				<Input
					id={`auto-words-${itemId}`}
					value={maxWords}
					inputMode="numeric"
					autoComplete="off"
					disabled={!canEdit || busy}
					aria-invalid={words === null ? true : undefined}
					className="h-8 px-2 text-sm"
					onChange={(event) => setMaxWords(event.target.value)}
				/>
				<Label htmlFor={`auto-chars-${itemId}`} className="text-xs">
					{t("ui.editor.auto_captions.max_chars")}
				</Label>
				<Input
					id={`auto-chars-${itemId}`}
					value={maxChars}
					inputMode="numeric"
					autoComplete="off"
					disabled={!canEdit || busy}
					aria-invalid={chars === null ? true : undefined}
					className="h-8 px-2 text-sm"
					onChange={(event) => setMaxChars(event.target.value)}
				/>
				<Label htmlFor={`auto-gap-${itemId}`} className="text-xs">
					{t("ui.editor.auto_captions.max_gap")}
				</Label>
				<Input
					id={`auto-gap-${itemId}`}
					value={maxGap}
					autoComplete="off"
					spellCheck={false}
					disabled={!canEdit || busy}
					aria-invalid={gap.ok ? undefined : true}
					className="h-8 px-2 font-mono text-sm"
					onChange={(event) => setMaxGap(event.target.value)}
				/>
				<Label htmlFor={`auto-replace-${itemId}`} className="text-xs">
					{t("ui.editor.auto_captions.replace")}
				</Label>
				<input
					id={`auto-replace-${itemId}`}
					type="checkbox"
					checked={replace}
					disabled={!canEdit || busy}
					className="size-4 accent-primary"
					onChange={(event) => setReplace(event.target.checked)}
				/>
			</div>
			{!valid && (
				<p role="alert" className="text-xs text-destructive">
					{t("ui.editor.auto_captions.invalid")}
				</p>
			)}

			<div className="flex flex-wrap gap-2">
				<Button
					type="button"
					size="sm"
					disabled={!canEdit || busy || !valid}
					onClick={() => void generate()}
				>
					{t("ui.editor.auto_captions.generate")}
				</Button>
				{linked && (
					<Button
						type="button"
						size="sm"
						variant="outline"
						disabled={!canEdit}
						onClick={() => void actions.resyncCaptions(itemId)}
					>
						{t("ui.editor.auto_captions.resync")}
					</Button>
				)}
			</div>

			{phase.name === "checking" && (
				<p role="status" className="text-xs">
					{t("ui.editor.auto_captions.checking")}
				</p>
			)}
			{phase.name === "transcribing" && (
				<div role="status">
					<p className="text-xs">{t("ui.editor.auto_captions.transcribing")}</p>
					<Progress
						value={percent}
						aria-label={t("ui.editor.auto_captions.progress")}
						className="my-1 h-2"
					/>
					<p className="text-xs [font-variant-numeric:tabular-nums]">
						{percent}%
					</p>
				</div>
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
			{unavailable !== null && (
				<div role="alert" className="text-xs" data-unavailable>
					<p className="font-medium text-destructive">
						{errorHeadline(t, unavailable.code)}
					</p>
					{unavailable.hint !== undefined && (
						<p lang="en" className="text-muted-foreground">
							<UntrustedText>{unavailable.hint}</UntrustedText>
						</p>
					)}
					<Button
						type="button"
						variant="link"
						size="sm"
						onClick={() => setInstallOpen(true)}
					>
						{t("ui.editor.auto_captions.open_install")}
					</Button>
				</div>
			)}
			<InstallDialog
				unavailable={installOpen ? unavailable : null}
				onClose={() => setInstallOpen(false)}
			/>
		</Section>
	);
}

import { Pause, Play, Undo2, X } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { audioPreviewUrl, proxyUrl } from "../api/urls";
import { fpsOf } from "../commands/units";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { selectFps, selectOrderedTrackIds } from "../state/selectors";
import { trackLabel } from "../ui/trackLabel";
import { useVoiceWaves, type VoiceWaves, WAVE_COLUMNS } from "./useVoiceWaves";

// One waveform: a bar for each column, centred, drawn with the colour of the text around it.
function Wave({
	bars,
	label,
	className,
}: {
	bars: Float32Array | null;
	label: string;
	className: string;
}): ReactNode {
	return (
		<div
			role="img"
			aria-label={label}
			data-wave={bars === null ? "empty" : "drawn"}
			className={`mx-auto flex h-8 w-full max-w-[290px] items-center justify-center gap-px ${className}`}
		>
			{Array.from({ length: WAVE_COLUMNS }, (_, c) => {
				const lo = bars?.[c * 2] ?? 0;
				const hi = bars?.[c * 2 + 1] ?? 0;
				const share = Math.max(0.12, Math.min(1, Math.max(hi, -lo)));
				return (
					<span
						// biome-ignore lint/suspicious/noArrayIndexKey: a fixed row of columns, never reordered
						key={c}
						aria-hidden="true"
						className="w-[2px] shrink-0 rounded-full bg-current"
						style={{ height: `${share * 100}%` }}
					/>
				);
			})}
		</div>
	);
}

// The play button of a card: it plays the sound it shows from the start of the item, and stops at its end.
function PlayButton({
	label,
	src,
	startSeconds,
	seconds,
}: {
	label: string;
	src: string | null;
	startSeconds: number;
	seconds: number;
}): ReactNode {
	const audio = useRef<HTMLAudioElement | null>(null);
	const [playing, setPlaying] = useState(false);
	useEffect(() => {
		return () => {
			audio.current?.pause();
		};
	}, []);
	return (
		<Button
			type="button"
			variant="outline"
			size="icon-sm"
			aria-label={label}
			aria-pressed={playing}
			disabled={src === null}
			onClick={() => {
				if (src === null) return;
				if (playing) {
					audio.current?.pause();
					setPlaying(false);
					return;
				}
				const element = audio.current ?? new Audio();
				audio.current = element;
				element.src = src;
				element.currentTime = startSeconds;
				element.ontimeupdate = () => {
					if (element.currentTime >= startSeconds + seconds) {
						element.pause();
						setPlaying(false);
					}
				};
				element.onended = () => setPlaying(false);
				void element.play().then(
					() => setPlaying(true),
					() => setPlaying(false),
				);
			}}
		>
			{playing ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}
		</Button>
	);
}

// The studio voice stage (final design 30): the sound of an item before the chain and after it, one waveform each,
// with a play button for each. The inspector at the right edits the chain. The waveforms are what the service
// rendered; the stage decides nothing about the sound.
export default function VoiceStage({
	itemId,
	onClose,
}: {
	itemId: string;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const label = useDocumentState((s) => {
		const track = item?.track;
		if (track === undefined) return "";
		return trackLabel(track, selectOrderedTrackIds(s), (id) =>
			s.document?.tracks[id]?.kind === "audio" ? "audio" : "visual",
		);
	});
	const fps = fpsOf(useDocumentState(selectFps));
	const waves: VoiceWaves = useVoiceWaves(itemId);
	if (item === undefined || item.asset === undefined) return null;
	const seconds = (frames: number): number => (frames * fps.den) / fps.num;
	const name = item.name ?? t(`ui.editor.item.${item.type}`);
	return (
		<section
			aria-label={t("ui.editor.voice_stage.label")}
			data-stage="voice"
			className="flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-auto p-6"
		>
			<div className="flex items-center justify-between gap-2">
				<h2 className="text-lg font-semibold">
					{label}: <UntrustedText>{name}</UntrustedText>
				</h2>
				<Button type="button" variant="ghost" size="sm" onClick={onClose}>
					<X aria-hidden="true" />
					{t("ui.editor.stage.close")}
				</Button>
			</div>
			<div
				className="flex flex-col gap-3 rounded-xl border border-border bg-surface p-4"
				data-card="before"
			>
				<div className="flex items-center justify-between">
					<h3 className="text-sm font-semibold">
						{t("ui.editor.voice_stage.before")}
					</h3>
					<PlayButton
						label={t("ui.editor.voice_stage.play_before")}
						src={proxyUrl(runtime.projectId, item.asset)}
						startSeconds={seconds(item.source_in ?? 0)}
						seconds={seconds(item.duration)}
					/>
				</div>
				<Wave
					bars={waves.before}
					label={t("ui.editor.voice_stage.wave_before")}
					className="text-muted-foreground"
				/>
			</div>
			<div
				className="flex flex-col gap-3 rounded-xl border border-border bg-surface p-4"
				data-card="after"
			>
				<div className="flex items-center justify-between">
					<h3 className="text-sm font-semibold">
						{t("ui.editor.voice_stage.after")}
					</h3>
					<PlayButton
						label={t("ui.editor.voice_stage.play_after")}
						src={
							waves.processedKey === null
								? null
								: audioPreviewUrl(runtime.projectId, waves.processedKey)
						}
						startSeconds={0}
						seconds={seconds(item.duration)}
					/>
				</div>
				<Wave
					bars={waves.after}
					label={t("ui.editor.voice_stage.wave_after")}
					className="text-accent"
				/>
				{waves.processedKey === null && (
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.voice_stage.no_chain")}
					</p>
				)}
			</div>
			<p className="flex items-center gap-2 text-[13px] text-muted-foreground">
				<Undo2 aria-hidden="true" className="size-4" />
				{t("ui.editor.voice_stage.note")}
			</p>
		</section>
	);
}

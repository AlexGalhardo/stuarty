import { type ReactNode, useEffect, useRef, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { sizeText } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import type { LibraryAsset } from "../editor/api/types";
import { proxyUrl } from "../editor/api/urls";
import {
	columnPeaks,
	loadPeaks,
	type Peaks,
} from "../editor/timeline/items/waveform";
import { formatSeconds } from "../editor/ui/timeFormat";

const WAVE_WIDTH = 480;
const WAVE_HEIGHT = 48;

// The whole file as one min and max per pixel column, from the peaks the server serves (SDD-011 section 4.6).
// One "frame" here is one sample, so a column spans sampleCount / WAVE_WIDTH samples.
function wholeFile(peaks: Peaks): Float32Array {
	return columnPeaks(peaks, {
		sourceIn: 0,
		speed: { num: 1, den: 1 },
		fps: { num: peaks.sampleRate, den: 1 },
		pxPerFrame: WAVE_WIDTH / Math.max(1, peaks.sampleCount),
		firstColumn: 0,
		columns: WAVE_WIDTH,
	});
}

// The waveform of a sound: the browser only draws what the server computed. A file the server cannot give peaks
// for shows a quiet line and says so.
export function Waveform({
	timelineId,
	assetId,
	label,
}: {
	timelineId: string;
	assetId: string;
	label: string;
}): ReactNode {
	const t = useT();
	const canvas = useRef<HTMLCanvasElement>(null);
	const [state, setState] = useState<"loading" | "ready" | "missing">(
		"loading",
	);

	useEffect(() => {
		let live = true;
		setState("loading");
		void loadPeaks(timelineId, assetId).then((peaks) => {
			if (!live) return;
			const ctx = canvas.current?.getContext("2d");
			if (peaks === null || ctx === null || ctx === undefined) {
				setState("missing");
				return;
			}
			const columns = wholeFile(peaks);
			const mid = WAVE_HEIGHT / 2;
			ctx.clearRect(0, 0, WAVE_WIDTH, WAVE_HEIGHT);
			ctx.fillStyle = getComputedStyle(canvas.current as HTMLElement).color;
			for (let x = 0; x < WAVE_WIDTH; x++) {
				const lo = columns[x * 2] ?? 0;
				const hi = columns[x * 2 + 1] ?? 0;
				const top = mid - hi * mid;
				ctx.fillRect(x, top, 1, Math.max(1, mid - lo * mid - top));
			}
			setState("ready");
		});
		return () => {
			live = false;
		};
	}, [timelineId, assetId]);

	return (
		<div className="flex flex-col gap-1">
			<canvas
				ref={canvas}
				width={WAVE_WIDTH}
				height={WAVE_HEIGHT}
				role="img"
				aria-label={label}
				data-testid="waveform"
				data-state={state}
				className="h-12 w-full rounded bg-muted text-primary"
			/>
			{state === "missing" && (
				<span className="text-xs text-muted-foreground">
					{t("ui.assets.sound.no_waveform")}
				</span>
			)}
		</div>
	);
}

// One sound of the library: its name, tags and duration, a player and its waveform. The row opens the detail
// panel from the name; the player is a native control so the keyboard and the screen reader have it for free.
export function SoundRow({
	asset,
	selected,
	onOpen,
}: {
	asset: LibraryAsset;
	selected: boolean;
	onOpen(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const poster = asset.poster;
	return (
		<li
			className="flex flex-col gap-2 rounded-xl border bg-card p-3 text-card-foreground aria-[current=true]:border-primary"
			aria-current={selected}
			data-asset-hash={asset.hash}
			data-testid="sound-row"
		>
			<div className="flex flex-wrap items-center justify-between gap-2">
				<button
					type="button"
					onClick={onOpen}
					aria-pressed={selected}
					className="min-w-0 truncate text-left text-sm font-medium underline-offset-2 hover:underline focus-visible:outline-2"
				>
					<UntrustedText>{asset.name}</UntrustedText>
				</button>
				<span className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
					{asset.duration_ms !== undefined && (
						<span data-testid="sound-duration">
							{formatSeconds(asset.duration_ms / 1000, "timecode")}
						</span>
					)}
					<span>{sizeText(asset.bytes, locale)}</span>
					<span>
						{t(`ui.assets.origin.${asset.origin?.kind ?? "imported"}`)}
					</span>
					{asset.reusable === true && (
						<span className="rounded bg-primary/10 px-1 text-primary">
							{t("ui.assets.reusable_badge")}
						</span>
					)}
					{(asset.tags ?? []).map((tag) => (
						<span key={tag} className="rounded bg-muted px-1">
							<UntrustedText>{tag}</UntrustedText>
						</span>
					))}
				</span>
			</div>
			{poster !== undefined && (
				<>
					<Waveform
						timelineId={poster.timeline_id}
						assetId={poster.asset_id}
						label={t("ui.assets.sound.waveform", { name: asset.name })}
					/>
					{/* biome-ignore lint/a11y/useMediaCaption: a sound has no spoken track to caption; a voice-over has its text in the library */}
					<audio
						controls
						preload="none"
						src={proxyUrl(poster.timeline_id, poster.asset_id)}
						aria-label={t("ui.assets.sound.player", { name: asset.name })}
						className="w-full"
					/>
				</>
			)}
		</li>
	);
}

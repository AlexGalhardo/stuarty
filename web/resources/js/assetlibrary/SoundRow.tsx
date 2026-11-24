import { type ReactNode, useEffect, useRef, useState } from "react";

import RenameMenu from "@/components/RenameMenu";
import UntrustedText from "@/components/UntrustedText";
import { formatDate } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import type { LibraryAsset } from "../editor/api/types";
import { proxyUrl } from "../editor/api/urls";
import {
	columnPeaks,
	PEAKS_RETRY_MS,
	type Peaks,
	watchPeaks,
} from "../editor/timeline/items/waveform";
import { formatSeconds } from "../editor/ui/timeFormat";
import { AssetSize } from "./AssetSize";
import { OriginBadge, ReusableBadge, TagChip } from "./badges";
import { type Reveal, RevealMenuItem } from "./reveal";

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
		setState("loading");
		// A long sound's peaks are computed by a job: the page asks again while the server says so.
		return watchPeaks(timelineId, assetId, PEAKS_RETRY_MS, (peaks) => {
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
				className="h-12 w-full rounded-md bg-surface-hover text-accent-text"
			/>
			{state === "missing" && (
				<span className="text-sm text-text-muted">
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
	rename,
	reveal,
	onReveal,
}: {
	asset: LibraryAsset;
	selected: boolean;
	onOpen(): void;
	// Sends a new name; without it the row has no context menu.
	rename?(name: string): Promise<void>;
	// What the person may do with the folder of the file, and the call that does it (epic 94).
	reveal?: Pick<Reveal, "kind" | "disabled">;
	onReveal?(hash: string): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const poster = asset.poster;
	const header = (
		<div className="flex flex-wrap items-center justify-between gap-2">
			<button
				type="button"
				onClick={onOpen}
				aria-pressed={selected}
				className="min-w-0 truncate text-left text-sm font-medium underline-offset-2 hover:underline focus-visible:outline-2"
			>
				<UntrustedText>{asset.name}</UntrustedText>
			</button>
			<span className="flex flex-wrap items-center gap-1.5 text-sm text-text-muted">
				{asset.duration_ms !== undefined && (
					<span data-testid="sound-duration" className="font-mono tabular-nums">
						{formatSeconds(asset.duration_ms / 1000, "timecode")}
					</span>
				)}
				<AssetSize asset={asset} />
				{asset.created_at !== undefined && (
					<span data-testid="asset-added">
						{t("ui.assets.added", {
							date: formatDate(asset.created_at, locale),
						})}
					</span>
				)}
				<OriginBadge kind={asset.origin?.kind} />
				{asset.reusable === true && <ReusableBadge />}
				{(asset.tags ?? []).map((tag) => (
					<TagChip key={tag} tag={tag} />
				))}
			</span>
		</div>
	);
	return (
		<li
			className="flex flex-col gap-2 rounded-lg border border-border bg-surface p-3 text-text aria-[current=true]:border-accent aria-[current=true]:bg-accent-soft"
			aria-current={selected}
			data-asset-hash={asset.hash}
			data-testid="sound-row"
		>
			{/* The menu is on the header only: the player keeps the context menu of the browser. */}
			{rename === undefined ? (
				header
			) : (
				<RenameMenu
					name={asset.name}
					rename={rename}
					extra={
						reveal !== undefined && (
							<RevealMenuItem
								reveal={reveal}
								onSelect={() => onReveal?.(asset.hash)}
							/>
						)
					}
					returnFocus={() =>
						document.querySelector<HTMLElement>(
							`li[data-asset-hash="${CSS.escape(asset.hash)}"] button`,
						)
					}
				>
					{header}
				</RenameMenu>
			)}
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

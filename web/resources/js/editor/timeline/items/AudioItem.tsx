import { type ReactNode, useEffect, useRef, useState } from "react";

import { fpsOf } from "../../commands/units";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../../state/EditorContext";
import { selectFps } from "../../state/selectors";
import MarkerTicks from "./MarkerTicks";
import {
	columnPeaks,
	PEAKS_RETRY_MS,
	type Peaks,
	watchPeaks,
} from "./waveform";

// Columns drawn beyond each edge of the viewport, so a short scroll shows
// drawn pixels before the next redraw.
const MARGIN_PX = 256;
const MAX_CANVAS_PX = 4096;

// The waveform behind the label of an audio item, the fade ramps and the
// volume text (SDD-012 section 7.4). The canvas is as wide as the visible part
// of the item (never the whole item: an hour at 32 px per frame cannot be one
// canvas) and follows the scroll. Decoration only (section 1.3).
export default function AudioItem({ id }: { id: string }): ReactNode {
	const runtime = useRuntime();
	const item = useDocumentState((s) => s.document?.items[id]);
	const fps = useDocumentState((s) => selectFps(s));
	const ppf = useUiState((s) => s.timeline.pxPerFrame);
	const host = useRef<HTMLSpanElement>(null);
	const canvas = useRef<HTMLCanvasElement>(null);
	const [peaks, setPeaks] = useState<Peaks | null>(null);
	const assetId = item?.asset;
	const sourceIn = item?.source_in ?? 0;
	const speedNum = item?.speed?.num ?? 1;
	const speedDen = item?.speed?.den ?? 1;
	const { num: fpsNum, den: fpsDen } = fpsOf(fps);

	useEffect(() => {
		if (assetId === undefined || assetId === "ast_placeholder") return;
		return watchPeaks(runtime.projectId, assetId, PEAKS_RETRY_MS, setPeaks);
	}, [runtime.projectId, assetId]);

	useEffect(() => {
		const el = canvas.current;
		const box = host.current;
		if (el === null || box === null || peaks === null) return;
		const scroller = box.closest<HTMLElement>("[data-timeline-scroller]");
		let frame = 0;
		const draw = (): void => {
			frame = 0;
			const rect = box.getBoundingClientRect();
			const view = scroller?.getBoundingClientRect();
			const left = Math.max(
				0,
				(view?.left ?? rect.left) - rect.left - MARGIN_PX,
			);
			const right = Math.min(
				rect.width,
				(view?.right ?? rect.right) - rect.left + MARGIN_PX,
			);
			const columns = Math.min(
				MAX_CANVAS_PX,
				Math.max(0, Math.ceil(right - left)),
			);
			const first = Math.floor(left);
			const height = Math.max(1, Math.round(rect.height));
			el.width = Math.max(1, columns);
			el.height = height;
			el.style.left = `${first}px`;
			el.style.width = `${Math.max(1, columns)}px`;
			el.style.height = `${height}px`;
			const context = el.getContext("2d");
			if (context === null || columns === 0) return;
			context.clearRect(0, 0, el.width, el.height);
			context.fillStyle = getComputedStyle(el).color;
			const mid = height / 2;
			const bars = columnPeaks(peaks, {
				sourceIn,
				speed: { num: speedNum, den: speedDen },
				fps: { num: fpsNum, den: fpsDen },
				pxPerFrame: ppf,
				firstColumn: first,
				columns,
			});
			for (let c = 0; c < columns; c++) {
				const lo = bars[c * 2] as number;
				const hi = bars[c * 2 + 1] as number;
				if (lo === 0 && hi === 0) continue;
				const top = mid - hi * mid;
				context.fillRect(c, top, 1, Math.max(1, mid - lo * mid - top));
			}
			el.dataset.drawn = "true";
		};
		const schedule = (): void => {
			if (frame === 0) frame = requestAnimationFrame(draw);
		};
		draw();
		scroller?.addEventListener("scroll", schedule, { passive: true });
		const resize = new ResizeObserver(schedule);
		resize.observe(box);
		if (scroller) resize.observe(scroller);
		return () => {
			scroller?.removeEventListener("scroll", schedule);
			resize.disconnect();
			if (frame !== 0) cancelAnimationFrame(frame);
		};
	}, [peaks, sourceIn, speedNum, speedDen, fpsNum, fpsDen, ppf]);

	if (item === undefined) return null;
	return (
		<>
			<span
				ref={host}
				aria-hidden="true"
				data-waveform=""
				className="pointer-events-none absolute inset-0 overflow-hidden"
			>
				<canvas
					ref={canvas}
					className="absolute top-0 text-ed-item-audio-fg opacity-60"
				/>
				{Boolean(item.fade_in) && (
					<span
						data-fade="in"
						className="absolute inset-y-0 left-0 bg-foreground/25"
						style={{
							["--fd" as string]: item.fade_in,
							width: "calc(var(--fd) * var(--ppf) * 1px)",
							clipPath: "polygon(0 0, 100% 0, 0 100%)",
						}}
					/>
				)}
				{Boolean(item.fade_out) && (
					<span
						data-fade="out"
						className="absolute inset-y-0 right-0 bg-foreground/25"
						style={{
							["--fd" as string]: item.fade_out,
							width: "calc(var(--fd) * var(--ppf) * 1px)",
							clipPath: "polygon(0 0, 100% 0, 100% 100%)",
						}}
					/>
				)}
			</span>
			<MarkerTicks id={id} />
			{item.volume !== undefined && item.volume !== 1 && (
				<span
					data-badge="volume"
					className="relative z-10 ml-1 shrink-0 rounded bg-background/80 px-1 text-[10px] leading-4 text-foreground"
				>
					{Math.round(item.volume * 100)}%
				</span>
			)}
		</>
	);
}

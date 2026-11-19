import {
	type KeyboardEvent,
	type PointerEvent,
	type ReactNode,
	useEffect,
	useRef,
} from "react";

import { useT } from "@/lib/i18n";
import { rowFor } from "../commands/keymapStore";
import { formatTime } from "../commands/units";
import { readTimeDisplay } from "../state/prefs";
import {
	clampPlayhead,
	clientXToFrame,
	type Fps,
	formatRulerLabel,
	framesPerSecond,
	RULER_HEIGHT_PX,
	rulerStep,
} from "./geometry";

interface RulerProps {
	ppf: number;
	fps: Fps;
	scrollLeft: number;
	viewportWidth: number;
	playheadFrame: number;
	endFrame: number;
	// The frames of the document markers and the in and out points of I and O.
	markers?: readonly number[];
	range?: { in: number | null; out: number | null };
	onSeek(frame: number): void;
}

function cssVar(name: string): string {
	return getComputedStyle(document.documentElement).getPropertyValue(name);
}

// Draws the ticks and labels of the visible window only, so the cost does not
// grow with the length of the project (SDD-012 section 7.3).
function draw(
	canvas: HTMLCanvasElement,
	props: Pick<RulerProps, "ppf" | "fps" | "scrollLeft" | "viewportWidth">,
): void {
	const ctx = canvas.getContext("2d");
	if (ctx === null) return;
	const dpr = window.devicePixelRatio || 1;
	const width = props.viewportWidth;
	const height = RULER_HEIGHT_PX;
	if (canvas.width !== Math.round(width * dpr)) {
		canvas.width = Math.round(width * dpr);
		canvas.height = Math.round(height * dpr);
	}
	ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
	ctx.clearRect(0, 0, width, height);

	const step = rulerStep(props.ppf, props.fps);
	const first = Math.max(
		0,
		Math.floor(props.scrollLeft / props.ppf / step.tickEvery),
	);
	const last = Math.ceil(
		(props.scrollLeft + width) / props.ppf / step.tickEvery,
	);
	ctx.strokeStyle = cssVar("--muted-foreground");
	ctx.fillStyle = cssVar("--muted-foreground");
	ctx.font = "11px var(--font-sans, sans-serif)";
	ctx.textBaseline = "top";
	ctx.beginPath();
	for (let n = first; n <= last; n++) {
		const frame = n * step.tickEvery;
		const x = Math.round(frame * props.ppf - props.scrollLeft) + 0.5;
		const labelled = frame % step.labelEvery === 0;
		ctx.moveTo(x, height);
		ctx.lineTo(x, labelled ? height - 14 : height - 6);
		if (labelled) {
			ctx.fillText(formatRulerLabel(frame, props.fps, step.format), x + 4, 4);
		}
	}
	ctx.stroke();
}

// The ruler: ticks, labels, the playhead handle, and seeking by click or
// drag (SDD-012 sections 7.3 and 7.5). A wrapper with role="slider" carries
// the playhead for assistive technology and the keyboard.
export default function Ruler(props: RulerProps): ReactNode {
	const t = useT();
	const wrapper = useRef<HTMLDivElement>(null);
	const canvas = useRef<HTMLCanvasElement>(null);
	const dragging = useRef(false);
	const {
		ppf,
		fps,
		scrollLeft,
		viewportWidth,
		playheadFrame,
		endFrame,
		markers = [],
		range = { in: null, out: null },
		onSeek,
	} = props;

	useEffect(() => {
		const el = canvas.current;
		if (el === null || viewportWidth <= 0) return;
		const handle = requestAnimationFrame(() =>
			draw(el, { ppf, fps, scrollLeft, viewportWidth }),
		);
		return () => cancelAnimationFrame(handle);
	}, [ppf, fps, scrollLeft, viewportWidth]);

	const seekFrom = (event: PointerEvent<HTMLDivElement>): void => {
		const rect = wrapper.current?.getBoundingClientRect();
		if (!rect) return;
		onSeek(clientXToFrame(event.clientX, rect.left, scrollLeft, ppf));
	};

	// The slider keys are rows of the registry (SDD-012 section 18.9): a frame
	// or a second back and forward, the start and the end. Mod+Shift+arrows are
	// the markers' and are read by the timeline.
	const onKeyDown = (event: KeyboardEvent<HTMLDivElement>): void => {
		const second = framesPerSecond(fps);
		switch (rowFor(event.nativeEvent, ["timeline"])) {
			case "step_back":
				onSeek(clampPlayhead(playheadFrame - 1));
				break;
			case "step_forward":
				onSeek(clampPlayhead(playheadFrame + 1));
				break;
			case "step_second_back":
				onSeek(clampPlayhead(playheadFrame - second));
				break;
			case "step_second_forward":
				onSeek(clampPlayhead(playheadFrame + second));
				break;
			case "go_start":
				onSeek(0);
				break;
			case "go_end":
				onSeek(clampPlayhead(endFrame));
				break;
			default:
				return;
		}
		event.preventDefault();
	};

	return (
		<div
			ref={wrapper}
			role="slider"
			tabIndex={0}
			aria-label={t("ui.editor.timeline.playhead")}
			aria-orientation="horizontal"
			aria-valuemin={0}
			aria-valuemax={endFrame}
			aria-valuenow={playheadFrame}
			aria-valuetext={formatTime(playheadFrame, fps, readTimeDisplay())}
			title={t("ui.editor.timeline.ruler")}
			className="relative h-full cursor-pointer touch-none overflow-hidden border-b border-ed-track-line bg-ed-panel select-none"
			onPointerDown={(event) => {
				dragging.current = true;
				event.currentTarget.setPointerCapture(event.pointerId);
				seekFrom(event);
			}}
			onPointerMove={(event) => {
				if (dragging.current) seekFrom(event);
			}}
			onPointerUp={(event) => {
				dragging.current = false;
				event.currentTarget.releasePointerCapture(event.pointerId);
			}}
			onPointerCancel={() => {
				dragging.current = false;
			}}
			onKeyDown={onKeyDown}
		>
			<canvas
				ref={canvas}
				className="absolute inset-0 h-full"
				style={{ width: viewportWidth }}
			/>
			{range.in !== null || range.out !== null ? (
				<div
					aria-hidden="true"
					data-testid="ruler-range"
					className="pointer-events-none absolute top-0 bottom-0 bg-primary/25"
					style={{
						left: (range.in ?? 0) * ppf - scrollLeft,
						width:
							range.in !== null && range.out !== null
								? (range.out - range.in) * ppf
								: undefined,
						right: range.in !== null && range.out !== null ? undefined : 0,
					}}
				/>
			) : null}
			{markers.map((frame) => (
				<span
					key={frame}
					aria-hidden="true"
					data-testid="ruler-marker"
					data-frame={frame}
					className="pointer-events-none absolute bottom-0 size-0 border-x-[5px] border-b-[8px] border-x-transparent border-b-ed-marker"
					style={{ left: frame * ppf - scrollLeft - 5 }}
				/>
			))}
			<div
				aria-hidden="true"
				className="pointer-events-none absolute top-0 bottom-0 left-0 w-0 border-l-2 border-ed-playhead"
				style={{
					transform: `translateX(${playheadFrame * ppf - scrollLeft}px)`,
				}}
			>
				<span className="absolute -top-px -left-[7px] size-0 border-x-[6px] border-t-[9px] border-x-transparent border-t-ed-playhead" />
			</div>
		</div>
	);
}

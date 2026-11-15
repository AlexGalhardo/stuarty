import { Magnet, Maximize2, ZoomIn, ZoomOut } from "lucide-react";
import {
	type ReactNode,
	useCallback,
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
} from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useActions } from "../commands/useActions";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import {
	selectEndFrame,
	selectFps,
	selectOrderedTrackIds,
} from "../state/selectors";
import {
	clampZoom,
	clientXToFrame,
	DEFAULT_FPS,
	extentPx,
	type Fps,
	fitZoom,
	formatTimecode,
	framesPerSecond,
	frameToContentX,
	nextZoom,
	RULER_HEIGHT_PX,
	TRACK_HEADER_PX,
	zoomAround,
} from "./geometry";
import { useMarquee } from "./gestures/marquee";
import {
	allItemIds,
	extendSelection,
	focusAfterRemoval,
	focusTarget,
	gridCommand,
	gridLayout,
	tabStop,
} from "./keyboard";
import Ruler from "./Ruler";
import SnapLine from "./SnapLine";
import { TrackHeader, TrackRow } from "./TrackRow";

function isEditable(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	return (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
	);
}

// The timeline of SDD-012 section 7 for the Prototype: tracks and items as
// labelled blocks, a ruler, a playhead, selection by click, zoom and scroll.
// It draws the read model and the UI state and never edits either.
export default function Timeline(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const trackIds = useDocumentState(selectOrderedTrackIds);
	const endFrame = useDocumentState(selectEndFrame);
	const fps: Fps = useDocumentState(selectFps) ?? DEFAULT_FPS;
	const ppf = useUiState((s) => s.timeline.pxPerFrame);
	const playhead = useUiState((s) => s.playheadFrame);
	const snapEnabled = useUiState((s) => s.timeline.snapEnabled);

	const actions = useActions();
	const gridRef = useRef<HTMLDivElement>(null);
	const marqueeBox = useRef<HTMLDivElement>(null);
	// Whether keyboard focus is on an item: when that item leaves the read model
	// the focus must land on another one instead of falling to the page.
	const focusInGrid = useRef(false);
	const scroller = useRef<HTMLDivElement>(null);
	const headers = useRef<HTMLDivElement>(null);
	const pendingScroll = useRef<number | null>(null);
	const [viewportWidth, setViewportWidth] = useState(0);
	const [scrollLeft, setScrollLeft] = useState(0);

	useEffect(() => {
		const el = scroller.current;
		if (el === null) return;
		const observer = new ResizeObserver(() => setViewportWidth(el.clientWidth));
		observer.observe(el);
		setViewportWidth(el.clientWidth);
		return () => observer.disconnect();
	}, []);

	// A zoom changes the width of the axis; the scroll position that keeps the
	// anchor still can only be applied after the new width is laid out.
	// biome-ignore lint/correctness/useExhaustiveDependencies: ppf is the trigger, not a value the effect reads.
	useLayoutEffect(() => {
		const el = scroller.current;
		if (el === null || pendingScroll.current === null) return;
		el.scrollLeft = pendingScroll.current;
		pendingScroll.current = null;
		setScrollLeft(el.scrollLeft);
	}, [ppf]);

	const marquee = useMarquee(scroller, marqueeBox, (ids) =>
		runtime.uiWriter.select(ids),
	);

	const focusItem = useCallback(
		(id: string): void => {
			runtime.uiWriter.focusItem(id);
			gridRef.current
				?.querySelector<HTMLElement>(`[data-item-id="${CSS.escape(id)}"]`)
				?.focus();
		},
		[runtime],
	);

	// The roving tab stop always names an item that exists, and keyboard focus
	// never falls to the page when its item is removed (SDD-012 section 7.10).
	useEffect(() => {
		let previous = runtime.documents.getState();
		const settle = (): void => {
			const state = runtime.documents.getState();
			const layout = gridLayout(state);
			const focused = runtime.ui.getState().focusedItem;
			const stop = tabStop(layout, focused);
			if (stop !== focused) {
				const next =
					focused === null
						? stop
						: focusAfterRemoval(gridLayout(previous), focused, layout);
				runtime.uiWriter.focusItem(next);
				if (focusInGrid.current) {
					if (next === null) gridRef.current?.focus();
					else focusItem(next);
				}
			}
			previous = state;
		};
		settle();
		return runtime.documents.subscribe(settle);
	}, [runtime, focusItem]);

	const onGridKeyDown = (event: React.KeyboardEvent<HTMLElement>): void => {
		const target = event.target;
		if (!(target instanceof HTMLElement) || isEditable(target)) return;
		const command = gridCommand(event.nativeEvent);
		if (command === null) return;
		const cell = target.closest<HTMLElement>("[data-item-id]");
		// The playhead slider and the headers keep their own keys.
		if (cell === null && command.kind !== "select-all") return;
		if (cell === null && target.closest('[role="slider"]') !== null) return;
		const state = runtime.documents.getState();
		const layout = gridLayout(state);
		const ui = runtime.ui.getState();
		const focused = cell?.dataset.itemId ?? ui.focusedItem;
		const acted =
			ui.selection.items.length > 0
				? ui.selection.items
				: focused === null
					? []
					: [focused];
		event.preventDefault();
		switch (command.kind) {
			case "select-all":
				runtime.uiWriter.select(allItemIds(layout));
				break;
			case "focus": {
				const next = focusTarget(layout, focused, command.direction);
				if (next === null) break;
				focusItem(next);
				if (command.extend) {
					const range = extendSelection(layout, ui.selection.anchor, next);
					runtime.uiWriter.select(range.items, range.anchor);
				}
				break;
			}
			case "activate": {
				if (focused === null) break;
				const only = ui.selection.items;
				if (only.length === 1 && only[0] === focused) {
					document
						.querySelector<HTMLElement>('[data-region="inspector"]')
						?.focus();
				} else {
					runtime.uiWriter.select([focused], focused);
				}
				break;
			}
			case "nudge": {
				const unit = command.unit === "second" ? framesPerSecond(fps) : 1;
				void actions.nudge(acted, command.sign * unit);
				break;
			}
			case "move-track":
				void actions.moveToAdjacentTrack(acted, command.sign);
				break;
		}
	};

	const zoomTo = useCallback(
		(next: number, anchorFrame: number, anchorViewportX: number) => {
			const clamped = clampZoom(next);
			pendingScroll.current = zoomAround(
				anchorFrame,
				anchorViewportX,
				clamped,
			).scrollLeft;
			runtime.uiWriter.setZoom(clamped);
		},
		[runtime],
	);

	const zoomStep = useCallback(
		(direction: 1 | -1) => {
			const el = scroller.current;
			const anchorX = (el?.clientWidth ?? 0) / 2;
			const anchorFrame = clientXToFrame(anchorX, 0, el?.scrollLeft ?? 0, ppf);
			zoomTo(nextZoom(ppf, direction), anchorFrame, anchorX);
		},
		[ppf, zoomTo],
	);

	const fitToView = useCallback(() => {
		const el = scroller.current;
		const next = fitZoom(el?.clientWidth ?? 0, endFrame);
		pendingScroll.current = 0;
		runtime.uiWriter.setZoom(next);
		if (next === ppf && el) el.scrollLeft = 0;
	}, [endFrame, ppf, runtime]);

	// Ctrl+wheel zooms continuously around the pointer. React attaches wheel
	// listeners as passive, which cannot cancel the browser zoom, so this one is
	// native.
	useEffect(() => {
		const el = scroller.current;
		if (el === null) return;
		const onWheel = (event: WheelEvent): void => {
			if (!event.ctrlKey) return;
			event.preventDefault();
			const rect = el.getBoundingClientRect();
			const anchorX = event.clientX - rect.left;
			const anchorFrame = clientXToFrame(
				event.clientX,
				rect.left,
				el.scrollLeft,
				ppf,
			);
			zoomTo(ppf * Math.exp(-event.deltaY * 0.01), anchorFrame, anchorX);
		};
		el.addEventListener("wheel", onWheel, { passive: false });
		return () => el.removeEventListener("wheel", onWheel);
	}, [ppf, zoomTo]);

	// The keys of SDD-012 section 10.1 for the timeline: + - and Shift+Z, and N
	// for snapping.
	useEffect(() => {
		const onKeyDown = (event: KeyboardEvent): void => {
			if (
				isEditable(event.target) ||
				event.ctrlKey ||
				event.metaKey ||
				event.altKey
			) {
				return;
			}
			if (event.key === "Escape") runtime.uiWriter.select([]);
			else if (event.key === "+" || event.key === "=") zoomStep(1);
			else if (event.key === "-") zoomStep(-1);
			else if (event.shiftKey && event.key.toLowerCase() === "z") fitToView();
			else if (event.key.toLowerCase() === "n") runtime.uiWriter.toggleSnap();
			else return;
			event.preventDefault();
		};
		window.addEventListener("keydown", onKeyDown);
		return () => window.removeEventListener("keydown", onKeyDown);
	}, [zoomStep, fitToView, runtime]);

	const axisPx = extentPx(viewportWidth, endFrame, ppf);
	const playheadX = frameToContentX(playhead, ppf);

	return (
		<section
			aria-label={t("ui.editor.timeline.label")}
			className="flex h-full min-h-0 flex-col bg-ed-timeline"
		>
			<div className="flex h-10 shrink-0 items-center gap-2 border-b border-ed-track-line bg-ed-panel px-3">
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					aria-label={t("ui.editor.timeline.zoom_out")}
					onClick={() => zoomStep(-1)}
				>
					<ZoomOut aria-hidden="true" />
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					aria-label={t("ui.editor.timeline.zoom_in")}
					onClick={() => zoomStep(1)}
				>
					<ZoomIn aria-hidden="true" />
				</Button>
				<Button type="button" variant="ghost" size="sm" onClick={fitToView}>
					<Maximize2 aria-hidden="true" />
					{t("ui.editor.timeline.fit")}
				</Button>
				<Button
					type="button"
					variant={snapEnabled ? "secondary" : "ghost"}
					size="sm"
					aria-pressed={snapEnabled}
					onClick={runtime.uiWriter.toggleSnap}
				>
					<Magnet aria-hidden="true" />
					{t("ui.editor.timeline.snap")}
				</Button>
				<span className="ml-auto font-mono text-xs text-muted-foreground tabular-nums">
					{formatTimecode(playhead, fps)}
				</span>
			</div>

			{/* biome-ignore lint/a11y/useSemanticElements: SDD-012 section 7.1 specifies a CSS-positioned grid, which a table cannot express. */}
			<div
				id="timeline"
				ref={gridRef}
				role="grid"
				aria-label={t("ui.editor.timeline.tracks")}
				aria-rowcount={trackIds.length}
				tabIndex={-1}
				onKeyDown={onGridKeyDown}
				onFocus={(event) => {
					const id = (event.target as HTMLElement).closest<HTMLElement>(
						"[data-item-id]",
					)?.dataset.itemId;
					focusInGrid.current = true;
					if (id !== undefined) runtime.uiWriter.focusItem(id);
				}}
				onBlur={() => {
					focusInGrid.current = false;
				}}
				className="grid min-h-0 flex-1"
				style={{
					["--ppf" as string]: ppf,
					gridTemplateColumns: `${TRACK_HEADER_PX}px minmax(0, 1fr)`,
					gridTemplateRows: `${RULER_HEIGHT_PX}px minmax(0, 1fr)`,
				}}
			>
				<div className="border-r border-b border-ed-track-line bg-ed-panel" />
				<Ruler
					ppf={ppf}
					fps={fps}
					scrollLeft={scrollLeft}
					viewportWidth={viewportWidth}
					playheadFrame={playhead}
					endFrame={endFrame}
					onSeek={runtime.uiWriter.setPlayhead}
				/>
				<div
					ref={headers}
					className="overflow-hidden border-r border-ed-track-line bg-ed-panel"
				>
					{trackIds.map((id) => (
						<TrackHeader key={id} id={id} />
					))}
				</div>
				{/* biome-ignore lint/a11y/noStaticElementInteractions lint/a11y/useKeyWithClickEvents: clicking empty space clears the selection; the keyboard equivalent is Esc, handled on window. */}
				<div
					ref={scroller}
					className="relative overflow-auto overscroll-contain"
					onScroll={(event) => {
						const el = event.currentTarget;
						setScrollLeft(el.scrollLeft);
						if (headers.current) headers.current.scrollTop = el.scrollTop;
					}}
					onPointerDown={marquee.onPointerDown}
					onClick={() => {
						if (!marquee.consumeClick()) runtime.uiWriter.select([]);
					}}
				>
					<div className="relative" style={{ width: axisPx }}>
						{trackIds.map((id) => (
							<TrackRow key={id} id={id} />
						))}
						{trackIds.length === 0 && (
							<p className="p-4 text-sm text-muted-foreground">
								{t("ui.editor.timeline.empty")}
							</p>
						)}
						<div
							ref={marqueeBox}
							aria-hidden="true"
							className="pointer-events-none absolute border border-ed-selection bg-ed-selection/20"
							style={{ display: "none" }}
						/>
						<SnapLine ppf={ppf} />
						<div
							aria-hidden="true"
							className="pointer-events-none absolute top-0 bottom-0 left-0 w-0 border-l-2 border-ed-playhead"
							style={{ transform: `translateX(${playheadX}px)` }}
						/>
					</div>
				</div>
			</div>
		</section>
	);
}

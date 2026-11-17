import {
	Frame,
	Maximize,
	Minimize,
	Pause,
	Play,
	SkipBack,
	SkipForward,
	StepBack,
	StepForward,
	Volume2,
	VolumeX,
} from "lucide-react";
import {
	Fragment,
	type ReactNode,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { frameUrl } from "../api/urls";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { selectEndFrame, selectFps } from "../state/selectors";
import { bufferSize, fitScale, sizedScale, truthWidth } from "./fit";
import { shouldCommitFrame } from "./mediaSync";
import Handles from "./overlays/Handles";
import { hitTest } from "./overlays/hitTest";
import InlineTextEditor from "./overlays/InlineTextEditor";
import SafeArea from "./overlays/SafeArea";
import SelectionBox from "./overlays/SelectionBox";
import { PixiExecutor } from "./PixiExecutor";
import { clampToLastFrame, PausedFrame } from "./pausedFrame";
import Timecode, { LiveTimecode } from "./Timecode";
import type { CanvasPort, LayerGeometry, PreviewState } from "./types";

const DEFAULT_FPS = { num: 30, den: 1 } as const;
// While playing the committed playhead follows at most every 250 ms, for the
// timeline and the URL (SDD-012 section 19); the timecode is written live.
const COMMIT_EVERY_MS = 250;

const SIZES = [
	["fit", "ui.editor.preview.size_fit"],
	["50", "ui.editor.preview.size_50"],
	["100", "ui.editor.preview.size_100"],
] as const;

// Full screen is offered only where the browser or webview has the API
// (SDD-012 section 21.1: feature detection, no assumption).
function canFullscreen(): boolean {
	return (
		typeof document !== "undefined" &&
		document.fullscreenEnabled === true &&
		typeof HTMLElement.prototype.requestFullscreen === "function"
	);
}

function isEditable(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	return (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes(target.tagName)
	);
}

// The preview of SDD-012 section 11: the executor draws the plan on a canvas,
// and 150 ms after a pause the core's exact frame replaces it.
export default function Preview({
	port,
	parseTime,
	trailing,
}: {
	port: CanvasPort;
	parseTime: (text: string) => number | null;
	// The aspect ratio selector at the right of the transport row, passed in by
	// the shell because it sends a command and the preview does not.
	trailing?: ReactNode;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const status = useDocumentState((s) => s.status);
	const revision = useDocumentState((s) => s.revision);
	const settings = useDocumentState((s) => s.document?.settings);
	const fps = useDocumentState(selectFps) ?? DEFAULT_FPS;
	const endFrame = useDocumentState(selectEndFrame);
	const playhead = useUiState((s) => s.playheadFrame);
	const width = settings?.width ?? 1080;
	const height = settings?.height ?? 1920;
	const ready = status === "ready" || status === "offline";

	const previewSize = useUiState((s) => s.previewSize);
	const volume = useUiState((s) => s.previewVolume);
	const muted = useUiState((s) => s.previewMuted);
	const [fullscreen, setFullscreen] = useState(false);
	const shell = useRef<HTMLDivElement>(null);
	const region = useRef<HTMLDivElement>(null);
	const host = useRef<HTMLDivElement>(null);
	const truth = useRef<HTMLImageElement>(null);
	const executor = useRef<PixiExecutor | null>(null);
	const paused = useRef<PausedFrame | null>(null);
	const committed = useRef(0);
	const playingRef = useRef(false);
	const liveRef = useRef(playhead);
	const needsLiveRef = useRef(false);
	const listeners = useRef(new Set<(frame: number) => void>());
	const subscribe = useCallback((cb: (frame: number) => void): (() => void) => {
		listeners.current.add(cb);
		return () => {
			listeners.current.delete(cb);
		};
	}, []);
	const bufferWidth = useRef(0);
	const revisionRef = useRef(0);
	const endFrameRef = useRef(0);
	const [state, setState] = useState<PreviewState>("idle");
	const [live, setLive] = useState(playhead);
	const [box, setBox] = useState({ w: 0, h: 0 });
	const selection = useUiState((s) => s.selection.items);
	const safeVisible = useUiState((s) => s.safeAreaVisible);
	const editingText = useUiState((s) => s.editingText);
	const eyedropper = useUiState((s) => s.eyedropper);
	const editedText = useDocumentState((s) =>
		editingText === null ? undefined : s.document?.items[editingText]?.text,
	);
	// Bumped when a plan window arrives, so the overlays read its geometry.
	const [, setPlanTick] = useState(0);
	// The box a transform gesture shows under the pointer, until the plan window
	// of the new revision arrives (SDD-012 section 11.6).
	const [override, setOverride] = useState<LayerGeometry | null>(null);

	revisionRef.current = revision ?? 0;
	endFrameRef.current = endFrame;
	// Overlays are drawn from `live`, so they need every frame while shown.
	needsLiveRef.current =
		selection.length > 0 || editingText !== null || safeVisible;

	// The executor lives as long as the editor; the project info and revision
	// are pushed to it as they change.
	useEffect(() => {
		const instance = new PixiExecutor(runtime.client, undefined, (asset) => {
			const found = runtime.documents.getState().document?.assets[asset];
			if (found?.width === undefined || found.height === undefined) {
				return null;
			}
			return { width: found.width, height: found.height };
		});
		executor.current = instance;
		const overlay = truth.current;
		const frames = new PausedFrame({
			setTimeout: (fn, ms) => setTimeout(fn, ms),
			clearTimeout: (handle) => clearTimeout(handle as number),
			load: async (url) => {
				if (overlay === null) throw new Error("no overlay");
				overlay.src = url;
				await overlay.decode();
			},
			show: () => overlay?.removeAttribute("hidden"),
			hide: () => overlay?.setAttribute("hidden", ""),
			urlFor: (frame, w, rev) =>
				frameUrl(
					runtime.projectId,
					clampToLastFrame(frame, endFrameRef.current),
					w,
					rev,
				),
			mark: (name) => performance.mark(name),
			measure: (name, a, b) => performance.measure(name, a, b),
			onFailure: (error) => console.debug("truth on pause failed", error),
		});
		paused.current = frames;
		let lastCommit = 0;
		const offFrame = instance.onFrame((frame) => {
			liveRef.current = frame;
			for (const listener of listeners.current) listener(frame);
			const now = performance.now();
			if (
				shouldCommitFrame(
					playingRef.current,
					needsLiveRef.current,
					now,
					lastCommit,
					COMMIT_EVERY_MS,
				)
			) {
				setLive(frame);
			}
			if (!playingRef.current || now - lastCommit >= COMMIT_EVERY_MS) {
				lastCommit = now;
				committed.current = frame;
				runtime.uiWriter.setPlayhead(frame);
			}
		});
		const offState = instance.onState((next) => {
			setState(next);
			playingRef.current = next === "playing" || next === "buffering";
			if (!playingRef.current) setLive(liveRef.current);
			if (next === "idle") {
				frames.request(
					committed.current,
					truthWidth(bufferWidth.current),
					revisionRef.current,
				);
			} else {
				frames.cancel();
			}
		});
		const host_ = host.current;
		if (host_ !== null) void instance.mount(host_);
		return () => {
			offFrame();
			offState();
			frames.cancel();
			instance.dispose();
			executor.current = null;
			paused.current = null;
		};
	}, [runtime]);

	useEffect(() => {
		executor.current?.setProject({
			projectId: runtime.projectId,
			width,
			height,
			fps,
		});
	}, [runtime, width, height, fps]);

	useEffect(() => {
		if (revision === null) return;
		paused.current?.cancel();
		executor.current?.setRevision(revision);
	}, [revision]);

	// A seek from the ruler or the buttons; the committed values that the
	// executor itself wrote while playing are not seeks.
	useEffect(() => {
		if (playhead === committed.current && playingRef.current) return;
		committed.current = playhead;
		// A seek is a frame like any other: the live ref and the timecode follow it, or
		// the next state change would put the previous frame back into `live`.
		liveRef.current = playhead;
		for (const listener of listeners.current) listener(playhead);
		setLive(playhead);
		executor.current?.seek(playhead);
	}, [playhead]);

	// The executor is rebuilt with the runtime, so the level is pushed again.
	// biome-ignore lint/correctness/useExhaustiveDependencies: `runtime` is the trigger that rebuilds the executor.
	useEffect(() => {
		executor.current?.setVolume(volume, muted);
	}, [runtime, volume, muted]);

	useEffect(() => {
		const onChange = (): void =>
			setFullscreen(document.fullscreenElement === shell.current);
		document.addEventListener("fullscreenchange", onChange);
		return () => document.removeEventListener("fullscreenchange", onChange);
	}, []);

	// Fit: the box is the available region minus 16 px of padding on each side;
	// 50% and 100% are shares of the pixel buffer and the region scrolls.
	useEffect(() => {
		const el = region.current;
		if (el === null) return;
		const observe = (): void => {
			const rect = el.getBoundingClientRect();
			const scale = sizedScale(
				previewSize,
				fitScale(rect.width, rect.height, width, height),
				width,
				height,
			);
			const w = width * scale;
			const h = height * scale;
			setBox({ w, h });
			const size = bufferSize(width, height, scale, window.devicePixelRatio);
			bufferWidth.current = size.width;
			executor.current?.resize(size.width, size.height);
		};
		const observer = new ResizeObserver(observe);
		observer.observe(el);
		observe();
		return () => observer.disconnect();
	}, [width, height, previewSize]);

	// The overlays read the plan window of the frame once it is held; while
	// playing the executor already keeps the windows and `live` re-renders.
	// biome-ignore lint/correctness/useExhaustiveDependencies: the dependencies are the triggers; the effect reads only the executor and `live`.
	useEffect(() => {
		const instance = executor.current;
		if (instance === null || !ready || state === "playing") return;
		let alive = true;
		void instance.whenPlan(live).then(() => {
			if (!alive) return;
			runtime.effectiveWriter.set(
				Object.fromEntries(
					instance.getLayerGeometry(live).map((g) => [
						g.item,
						{
							x: g.cx,
							y: g.cy,
							rotation: g.rotation,
							scale: g.scale,
							opacity: g.opacity,
						},
					]),
				),
			);
			setPlanTick((n) => n + 1);
			setOverride(null);
		});
		return () => {
			alive = false;
		};
	}, [live, revision, ready, state, selection, safeVisible]);

	const togglePlay = (): void => {
		const instance = executor.current;
		if (instance === null) return;
		if (playingRef.current) instance.pause();
		else void instance.play();
	};

	const togglePlayRef = useRef(togglePlay);
	togglePlayRef.current = togglePlay;
	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (isEditable(event.target)) return;
			if (event.code === "Space") {
				event.preventDefault();
				togglePlayRef.current();
				return;
			}
			// J, K and L (VV-035): back one second, pause, play. The preview has
			// no reverse or fast playback, so J steps back instead of shuttling.
			if (event.ctrlKey || event.metaKey || event.altKey || event.shiftKey) {
				return;
			}
			if (event.key.toLowerCase() === "k") {
				executor.current?.pause();
			} else if (event.key.toLowerCase() === "l") {
				if (!playingRef.current) void executor.current?.play();
			} else if (event.key.toLowerCase() === "j") {
				backOneSecondRef.current();
			} else return;
			event.preventDefault();
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, []);

	// Esc cancels the eyedropper pick only: the capture phase keeps it from also
	// clearing the selection the pick belongs to.
	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (event.key !== "Escape" || runtime.ui.getState().eyedropper === null)
				return;
			event.preventDefault();
			event.stopPropagation();
			runtime.uiWriter.setEyedropper(null);
		};
		window.addEventListener("keydown", onKey, true);
		return () => window.removeEventListener("keydown", onKey, true);
	}, [runtime]);

	const seekTo = (frame: number): void => {
		executor.current?.pause();
		runtime.uiWriter.setPlayhead(Math.max(frame, 0));
	};
	const backOneSecondRef = useRef(() => {});
	backOneSecondRef.current = () =>
		seekTo(
			runtime.ui.getState().playheadFrame -
				Math.max(1, Math.round(fps.num / fps.den)),
		);
	const playing = state === "playing" || state === "buffering";
	const scale = width > 0 ? box.w / width : 0;
	const layers = executor.current?.getLayerGeometry(live) ?? [];
	const safeArea = safeVisible
		? (executor.current?.getSafeArea(live) ?? null)
		: null;
	const unsupported = state === "unsupported";

	return (
		<div
			ref={shell}
			className="flex min-h-0 flex-1 flex-col gap-2 [&:fullscreen]:bg-ed-preview-surround [&:fullscreen]:p-4"
		>
			<div ref={region} className="flex min-h-0 flex-1 overflow-auto">
				<div
					className="relative m-auto shrink-0"
					style={{ width: box.w, height: box.h }}
				>
					<div className="absolute inset-0 overflow-hidden bg-black">
						<div ref={host} className="absolute inset-0" />
						<img
							ref={truth}
							hidden
							alt={t("ui.editor.preview.frame")}
							className="absolute inset-0 size-full"
						/>
					</div>
					{/* biome-ignore lint/a11y/noStaticElementInteractions lint/a11y/useKeyWithClickEvents: picking on the canvas is a pointer affordance; the keyboard selects in the timeline. */}
					<div
						data-testid="canvas-overlay"
						className={cn(
							"absolute inset-0",
							eyedropper !== null && "cursor-crosshair",
						)}
						onDoubleClick={(event) => {
							if (scale === 0) return;
							const rect = event.currentTarget.getBoundingClientRect();
							const hit = hitTest(layers, {
								x: (event.clientX - rect.left) / scale,
								y: (event.clientY - rect.top) / scale,
							});
							const state = runtime.documents.getState();
							if (
								hit === null ||
								state.state === "trashed" ||
								state.document?.items[hit]?.type !== "text"
							)
								return;
							runtime.uiWriter.select([hit]);
							runtime.uiWriter.startTextEdit(hit);
						}}
						onClick={(event) => {
							if (scale === 0 || port.consumeClick()) return;
							const rect = event.currentTarget.getBoundingClientRect();
							if (eyedropper !== null) {
								// The eyedropper (VV-186): read the colour under the pointer
								// and hand it to the inspector, which sends the chroma key command.
								const hex = executor.current?.readColour(
									(event.clientX - rect.left) / scale,
									(event.clientY - rect.top) / scale,
								);
								runtime.uiWriter.setEyedropper(null);
								if (hex !== null && hex !== undefined) eyedropper.onPick(hex);
								return;
							}
							const hit = hitTest(layers, {
								x: (event.clientX - rect.left) / scale,
								y: (event.clientY - rect.top) / scale,
							});
							const known =
								hit !== null &&
								runtime.documents.getState().document?.items[hit] !== undefined;
							runtime.uiWriter.select(known && hit !== null ? [hit] : []);
						}}
					>
						{safeArea !== null && <SafeArea rect={safeArea} scale={scale} />}
						{editingText !== null && editedText !== undefined && (
							<InlineTextEditor
								key={editingText}
								itemId={editingText}
								stored={editedText}
								geometry={
									layers.find((layer) => layer.item === editingText) ?? null
								}
								scale={scale}
								size={box}
								port={port}
								onClose={() => runtime.uiWriter.startTextEdit(null)}
							/>
						)}
						{layers
							.filter((layer) => selection.includes(layer.item))
							.map((layer) => {
								const shown = override?.item === layer.item ? override : layer;
								return (
									<Fragment key={layer.item}>
										<SelectionBox geometry={shown} scale={scale} />
										{selection.length === 1 && (
											<Handles
												geometry={shown}
												scale={scale}
												port={port}
												onPreview={setOverride}
											/>
										)}
									</Fragment>
								);
							})}
					</div>
				</div>
			</div>
			{unsupported && (
				<p role="status" className="text-xs text-warning">
					{t("ui.editor.preview.unsupported")}
				</p>
			)}
			<div
				role="toolbar"
				aria-label={t("ui.editor.preview.transport")}
				className="flex flex-wrap items-center gap-x-3 gap-y-1"
			>
				<div className="flex shrink-0 items-center">
					<span className="flex items-center gap-1 text-muted-foreground">
						<LiveTimecode
							initial={live}
							subscribe={subscribe}
							fps={fps}
							label={t("ui.editor.preview.current")}
							parse={parseTime}
							onSeek={(frame) => seekTo(Math.min(frame, endFrame))}
						/>
						<span aria-hidden="true">/</span>
						<Timecode
							frame={endFrame}
							fps={fps}
							label={t("ui.editor.preview.total")}
						/>
					</span>
				</div>
				<div className="mx-auto flex shrink-0 items-center gap-1">
					<Button
						type="button"
						variant="ghost"
						size="icon-sm"
						disabled={!ready}
						aria-label={t("ui.editor.preview.to_start")}
						onClick={() => seekTo(0)}
					>
						<SkipBack aria-hidden="true" />
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="icon-sm"
						disabled={!ready}
						aria-label={t("ui.editor.preview.step_back")}
						onClick={() => seekTo(live - 1)}
					>
						<StepBack aria-hidden="true" />
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="icon-sm"
						disabled={!ready || unsupported}
						aria-label={
							playing
								? t("ui.editor.preview.pause")
								: t("ui.editor.preview.play")
						}
						onClick={togglePlay}
					>
						{playing ? (
							<Pause aria-hidden="true" />
						) : (
							<Play aria-hidden="true" />
						)}
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="icon-sm"
						disabled={!ready}
						aria-label={t("ui.editor.preview.step_forward")}
						onClick={() => seekTo(live + 1)}
					>
						<StepForward aria-hidden="true" />
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="icon-sm"
						disabled={!ready}
						aria-label={t("ui.editor.preview.to_end")}
						onClick={() => seekTo(endFrame)}
					>
						<SkipForward aria-hidden="true" />
					</Button>
				</div>
				<div className="ml-auto flex shrink-0 items-center gap-1">
					{trailing}
					<Button
						type="button"
						variant={safeVisible ? "secondary" : "ghost"}
						size="icon-sm"
						aria-pressed={safeVisible}
						aria-label={t("ui.editor.preview.safe_area")}
						onClick={runtime.uiWriter.toggleSafeArea}
					>
						<Frame aria-hidden="true" />
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="icon-sm"
						aria-pressed={muted}
						aria-label={
							muted
								? t("ui.editor.preview.unmute")
								: t("ui.editor.preview.mute")
						}
						onClick={runtime.uiWriter.togglePreviewMuted}
					>
						{muted ? (
							<VolumeX aria-hidden="true" />
						) : (
							<Volume2 aria-hidden="true" />
						)}
					</Button>
					<input
						type="range"
						min={0}
						max={1}
						step={0.05}
						value={muted ? 0 : volume}
						aria-label={t("ui.editor.preview.volume")}
						className="h-6 w-20 accent-primary"
						onChange={(event) => {
							runtime.uiWriter.setPreviewVolume(
								Number(event.currentTarget.value),
							);
							if (muted) runtime.uiWriter.togglePreviewMuted();
						}}
					/>
					{/* biome-ignore lint/a11y/useSemanticElements: a fieldset would draw a border and legend around buttons in a toolbar. */}
					<div
						role="group"
						aria-label={t("ui.editor.preview.size")}
						className="ml-2 flex items-center gap-0.5"
					>
						{SIZES.map(([value, key]) => (
							<Button
								key={value}
								type="button"
								variant={previewSize === value ? "secondary" : "ghost"}
								size="sm"
								aria-pressed={previewSize === value}
								onClick={() => runtime.uiWriter.setPreviewSize(value)}
							>
								{t(key)}
							</Button>
						))}
					</div>
					{canFullscreen() && (
						<Button
							type="button"
							variant="ghost"
							size="icon-sm"
							aria-pressed={fullscreen}
							aria-label={
								fullscreen
									? t("ui.editor.preview.exit_fullscreen")
									: t("ui.editor.preview.fullscreen")
							}
							onClick={() => {
								if (document.fullscreenElement !== null) {
									void document.exitFullscreen();
								} else {
									void shell.current
										?.requestFullscreen()
										.catch(() => undefined);
								}
							}}
						>
							{fullscreen ? (
								<Minimize aria-hidden="true" />
							) : (
								<Maximize aria-hidden="true" />
							)}
						</Button>
					)}
				</div>
			</div>
		</div>
	);
}

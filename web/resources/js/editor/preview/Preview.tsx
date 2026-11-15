import {
	Frame,
	Pause,
	Play,
	SkipBack,
	SkipForward,
	StepBack,
	StepForward,
} from "lucide-react";
import { Fragment, type ReactNode, useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { frameUrl } from "../api/urls";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { selectEndFrame, selectFps } from "../state/selectors";
import { bufferSize, fitScale, truthWidth } from "./fit";
import Handles from "./overlays/Handles";
import { hitTest } from "./overlays/hitTest";
import InlineTextEditor from "./overlays/InlineTextEditor";
import SafeArea from "./overlays/SafeArea";
import SelectionBox from "./overlays/SelectionBox";
import { PixiExecutor } from "./PixiExecutor";
import { PausedFrame } from "./pausedFrame";
import Timecode from "./Timecode";
import type { CanvasPort, LayerGeometry, PreviewState } from "./types";

const DEFAULT_FPS = { num: 30, den: 1 } as const;
// While playing the committed playhead follows at most every 250 ms, for the
// timeline and the URL (SDD-012 section 19); the timecode is written live.
const COMMIT_EVERY_MS = 250;

function isEditable(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	return (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes(target.tagName)
	);
}

// The preview of SDD-012 section 11: the executor draws the plan on a canvas,
// and 150 ms after a pause the core's exact frame replaces it.
export default function Preview({ port }: { port: CanvasPort }): ReactNode {
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

	const region = useRef<HTMLDivElement>(null);
	const host = useRef<HTMLDivElement>(null);
	const truth = useRef<HTMLImageElement>(null);
	const executor = useRef<PixiExecutor | null>(null);
	const paused = useRef<PausedFrame | null>(null);
	const committed = useRef(0);
	const playingRef = useRef(false);
	const bufferWidth = useRef(0);
	const revisionRef = useRef(0);
	const [state, setState] = useState<PreviewState>("idle");
	const [live, setLive] = useState(playhead);
	const [box, setBox] = useState({ w: 0, h: 0 });
	const selection = useUiState((s) => s.selection.items);
	const safeVisible = useUiState((s) => s.safeAreaVisible);
	const editingText = useUiState((s) => s.editingText);
	const editedText = useDocumentState((s) =>
		editingText === null ? undefined : s.document?.items[editingText]?.text,
	);
	// Bumped when a plan window arrives, so the overlays read its geometry.
	const [, setPlanTick] = useState(0);
	// The box a transform gesture shows under the pointer, until the plan window
	// of the new revision arrives (SDD-012 section 11.6).
	const [override, setOverride] = useState<LayerGeometry | null>(null);

	revisionRef.current = revision ?? 0;

	// The executor lives as long as the editor; the project info and revision
	// are pushed to it as they change.
	useEffect(() => {
		const instance = new PixiExecutor(runtime.client);
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
			urlFor: (frame, w, rev) => frameUrl(runtime.projectId, frame, w, rev),
			mark: (name) => performance.mark(name),
			measure: (name, a, b) => performance.measure(name, a, b),
			onFailure: (error) => console.debug("truth on pause failed", error),
		});
		paused.current = frames;
		let lastCommit = 0;
		const offFrame = instance.onFrame((frame) => {
			setLive(frame);
			const now = performance.now();
			if (!playingRef.current || now - lastCommit >= COMMIT_EVERY_MS) {
				lastCommit = now;
				committed.current = frame;
				runtime.uiWriter.setPlayhead(frame);
			}
		});
		const offState = instance.onState((next) => {
			setState(next);
			playingRef.current = next === "playing" || next === "buffering";
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
		setLive(playhead);
		executor.current?.seek(playhead);
	}, [playhead]);

	// Fit: the box is the available region minus 16 px of padding on each side.
	useEffect(() => {
		const el = region.current;
		if (el === null) return;
		const observe = (): void => {
			const rect = el.getBoundingClientRect();
			const scale = fitScale(rect.width, rect.height, width, height);
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
	}, [width, height]);

	// The overlays read the plan window of the frame once it is held; while
	// playing the executor already keeps the windows and `live` re-renders.
	// biome-ignore lint/correctness/useExhaustiveDependencies: the dependencies are the triggers; the effect reads only the executor and `live`.
	useEffect(() => {
		const instance = executor.current;
		if (instance === null || !ready || state === "playing") return;
		let alive = true;
		void instance.whenPlan(live).then(() => {
			if (!alive) return;
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

	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (event.code !== "Space" || isEditable(event.target)) return;
			event.preventDefault();
			togglePlay();
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	});

	const seekTo = (frame: number): void => {
		executor.current?.pause();
		runtime.uiWriter.setPlayhead(Math.max(frame, 0));
	};
	const playing = state === "playing" || state === "buffering";
	const scale = width > 0 ? box.w / width : 0;
	const layers = executor.current?.getLayerGeometry(live) ?? [];
	const safeArea = safeVisible
		? (executor.current?.getSafeArea(live) ?? null)
		: null;
	const unsupported = state === "unsupported";

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-2">
			<div
				ref={region}
				className="flex min-h-0 flex-1 items-center justify-center"
			>
				<div className="relative" style={{ width: box.w, height: box.h }}>
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
						className="absolute inset-0"
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
				className="flex items-center justify-center gap-1"
			>
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
						playing ? t("ui.editor.preview.pause") : t("ui.editor.preview.play")
					}
					onClick={togglePlay}
				>
					{playing ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}
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
				<span className="ml-3 flex items-center gap-1 text-muted-foreground">
					<Timecode
						frame={live}
						fps={fps}
						label={t("ui.editor.preview.current")}
					/>
					<span aria-hidden="true">/</span>
					<Timecode
						frame={endFrame}
						fps={fps}
						label={t("ui.editor.preview.total")}
					/>
				</span>
			</div>
		</div>
	);
}

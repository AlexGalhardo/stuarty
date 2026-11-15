import {
	Pause,
	Play,
	SkipBack,
	SkipForward,
	StepBack,
	StepForward,
} from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

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
import { PixiExecutor } from "./PixiExecutor";
import { PausedFrame } from "./pausedFrame";
import Timecode from "./Timecode";
import type { PreviewState } from "./types";

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
export default function Preview(): ReactNode {
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
	const unsupported = state === "unsupported";

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-2">
			<div
				ref={region}
				className="flex min-h-0 flex-1 items-center justify-center"
			>
				<div
					className="relative overflow-hidden bg-black"
					style={{ width: box.w, height: box.h }}
				>
					<div ref={host} className="absolute inset-0" />
					<img
						ref={truth}
						hidden
						alt={t("ui.editor.preview.frame")}
						className="absolute inset-0 size-full"
					/>
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

import type { FrameRate } from "../state/selectors";

export type Unsubscribe = () => void;

export type PreviewState =
	| "idle"
	| "playing"
	| "seeking"
	| "buffering"
	| "unsupported";

// The plan geometry of one layer at one frame, in project pixels, for the
// selection box and hit tests (SDD-012 section 11.6).
export interface LayerGeometry {
	item: string;
	cx: number;
	cy: number;
	w: number;
	h: number;
	rotation: number;
	// What the plan reports at the frame: keyframes and presets applied.
	scale: number;
	opacity: number;
}

export type HandleKind = "body" | "scale" | "rotate";

// What a transform gesture is started with (SDD-012 section 11.6).
export interface TransformBegin {
	event: React.PointerEvent<HTMLElement>;
	kind: HandleKind;
	itemId: string;
	// The box at pointer down, from the plan.
	start: LayerGeometry;
	// Screen pixels per project pixel.
	scale: number;
	// The element the box is positioned in, for the centre on the screen.
	overlay: HTMLElement;
	// The box under the pointer; null drops it.
	onPreview(geometry: LayerGeometry | null): void;
}

// The seam to the gesture and command code, handed down by the layout because
// `preview` must not import `commands` or `timeline` (SDD-012 section 3.3).
export interface CanvasPort {
	begin(options: TransformBegin): void;
	// Whether a click is the one that follows a drag, and so selects nothing.
	consumeClick(): boolean;
	// The inline text editor committed: sends `text.set {item_id, text}`.
	commitText(itemId: string, text: string): void;
}

export interface ProjectInfo {
	projectId: string;
	width: number;
	height: number;
	fps: FrameRate;
}

// The seam ADR-0013 requires, so PixiJS can be replaced (SDD-012 section 11.1).
export interface PreviewExecutor {
	mount(host: HTMLElement): Promise<void>;
	dispose(): void;
	setProject(info: ProjectInfo): void;
	// Invalidates the plan windows, keeps the media elements.
	setRevision(revision: number): void;
	// A timeline this one uses as a composition changed (nested.updated): the
	// revision is the same, the plan windows are dropped and the frame drawn again.
	refresh(): void;
	// Paused or scrubbing.
	seek(frame: number, options?: { scrubbing?: boolean }): void;
	// Resumes the AudioContext, so it must run inside a user gesture.
	play(): Promise<void>;
	pause(): void;
	onFrame(cb: (frame: number) => void): Unsubscribe;
	onState(cb: (state: PreviewState) => void): Unsubscribe;
	// The level of the whole preview, 0 to 1, and the mute. Screen state only.
	setVolume(volume: number, muted: boolean): void;
	// Sets the size of the pixel buffer; the caller decides the fit.
	resize(bufferWidth: number, bufferHeight: number): void;
	getLayerGeometry(frame: number): LayerGeometry[];
	// The `safe_area` of the plan window of that frame, `[x, y, w, h]` in project
	// pixels, once the window is held (SDD-012 section 11.6).
	getSafeArea(frame: number): [number, number, number, number] | null;
	// Resolves when the plan window of the frame is held, so a caller can ask for
	// its geometry; never rejects.
	whenPlan(frame: number): Promise<void>;
	// The colour the preview canvas shows at a point of the frame, `x` and `y`
	// in project pixels, as `#RRGGBB`; null when nothing is drawn yet. A read of
	// pixels for the eyedropper: no edit result is worked out from it.
	readColour(x: number, y: number): string | null;
	readonly stats: {
		videoElements: number;
		planWindows: number;
		droppedFrames: number;
		// VideoFrames open now and at most, and assets drawn by <video> because
		// WebCodecs cannot decode them (SDD-013 section 5.3).
		openFrames: number;
		peakOpenFrames: number;
		fallbackAssets: number;
	};
}

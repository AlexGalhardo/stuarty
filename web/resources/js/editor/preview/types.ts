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

// The seam to the gesture code of `timeline/gestures`, handed down by the
// layout because `preview` must not import `commands` or `timeline` (SDD-012
// section 3.3).
export interface TransformPort {
	begin(options: TransformBegin): void;
	// Whether a click is the one that follows a drag, and so selects nothing.
	consumeClick(): boolean;
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
	// Paused or scrubbing.
	seek(frame: number, options?: { scrubbing?: boolean }): void;
	// Resumes the AudioContext, so it must run inside a user gesture.
	play(): Promise<void>;
	pause(): void;
	onFrame(cb: (frame: number) => void): Unsubscribe;
	onState(cb: (state: PreviewState) => void): Unsubscribe;
	// Sets the size of the pixel buffer; the caller decides the fit.
	resize(bufferWidth: number, bufferHeight: number): void;
	getLayerGeometry(frame: number): LayerGeometry[];
	// The `safe_area` of the plan window of that frame, `[x, y, w, h]` in project
	// pixels, once the window is held (SDD-012 section 11.6).
	getSafeArea(frame: number): [number, number, number, number] | null;
	// Resolves when the plan window of the frame is held, so a caller can ask for
	// its geometry; never rejects.
	whenPlan(frame: number): Promise<void>;
	readonly stats: {
		videoElements: number;
		planWindows: number;
		droppedFrames: number;
	};
}

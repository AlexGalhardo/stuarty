// Which executor draws the preview (SDD-012 section 11.1, SDD-013 section 4.7):
// the renderer of the export in a worker (`rust`) or PixiJS (`pixi`). The
// choice is by feature detection, probed once, and a setting that forces either
// one, for comparison and for bug reports.

export type ExecutorKind = "rust" | "pixi";
export type ExecutorSetting = ExecutorKind | "auto";

// The query parameter and the stored value (`stuarty.preview.executor`, read by
// state/prefs.ts) that force an executor. The query wins, so a link reproduces
// a report; `auto` (or no value) lets detection and DEFAULT_EXECUTOR decide.
export const EXECUTOR_PARAM = "executor";

// What the executor chosen without a setting is when the browser can run the
// worker. PixiJS stays the default until the measurements of epic 26 task 8
// show the renderer is not slower on the reference project.
export const DEFAULT_EXECUTOR: ExecutorKind = "pixi";

// The features the render worker and its feeding need. Each name is what the
// diagnostic line prints.
export interface FeatureProbe {
	Worker: boolean;
	WebAssembly: boolean;
	OffscreenCanvas: boolean;
	createImageBitmap: boolean;
	VideoDecoder: boolean;
	VideoFrame: boolean;
	"crypto.subtle": boolean;
}

export function probeFeatures(scope: Record<string, unknown>): FeatureProbe {
	const subtle = (scope.crypto as { subtle?: unknown } | undefined)?.subtle;
	return {
		Worker: typeof scope.Worker === "function",
		WebAssembly: typeof scope.WebAssembly === "object",
		OffscreenCanvas: typeof scope.OffscreenCanvas === "function",
		createImageBitmap: typeof scope.createImageBitmap === "function",
		VideoDecoder: typeof scope.VideoDecoder === "function",
		VideoFrame: typeof scope.VideoFrame === "function",
		"crypto.subtle": subtle !== undefined && subtle !== null,
	};
}

export function missingFeatures(probe: FeatureProbe): string[] {
	return Object.entries(probe)
		.filter(([, present]) => !present)
		.map(([name]) => name);
}

export function readSetting(
	search: string,
	stored: string | null,
): ExecutorSetting {
	const value = new URLSearchParams(search).get(EXECUTOR_PARAM) ?? stored;
	return value === "rust" || value === "pixi" ? value : "auto";
}

export interface ExecutorChoice {
	kind: ExecutorKind;
	// Why the other executor is in use, for the diagnostic line; null when the
	// choice is the plain one.
	reason: string | null;
}

export function chooseExecutor(
	setting: ExecutorSetting,
	missing: readonly string[],
	fallbackDefault: ExecutorKind = DEFAULT_EXECUTOR,
): ExecutorChoice {
	const wanted = setting === "auto" ? fallbackDefault : setting;
	if (wanted === "pixi") {
		return {
			kind: "pixi",
			reason: setting === "pixi" ? "forced by the setting" : null,
		};
	}
	if (missing.length > 0) {
		return {
			kind: "pixi",
			reason: `the browser lacks ${missing.join(", ")}`,
		};
	}
	return { kind: "rust", reason: null };
}

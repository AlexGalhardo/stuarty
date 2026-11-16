export type RendererPreference = "webgpu" | "webgl";

interface GpuLike {
	requestAdapter(): Promise<unknown>;
}

// The query parameter that forces one renderer, so the end-to-end specs can run
// the same scenario on both (SDD-012 section 11.1).
export const RENDERER_PARAM = "renderer";

// WebGPU when the browser gives an adapter, else WebGL; `forced` (the value of
// the query parameter) overrides the detection when it names one of the two.
export async function chooseRenderer(
	forced: string | null,
	gpu: GpuLike | undefined,
): Promise<RendererPreference> {
	if (forced === "webgl" || forced === "webgpu") return forced;
	if (gpu === undefined) return "webgl";
	try {
		return (await gpu.requestAdapter()) === null ? "webgl" : "webgpu";
	} catch {
		return "webgl";
	}
}

export function detectRenderer(): Promise<RendererPreference> {
	const forced = new URLSearchParams(window.location.search).get(
		RENDERER_PARAM,
	);
	const gpu = (navigator as Navigator & { gpu?: GpuLike }).gpu;
	return chooseRenderer(forced, gpu);
}

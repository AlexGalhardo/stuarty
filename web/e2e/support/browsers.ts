import type { Page } from "@playwright/test";

// Specs that run in no project of the browser matrix (SDD-012 section 20.1):
// the load-sensitive budgets, the soak and the failing-encoder and detection
// servers.
export const NOT_IN_MATRIX =
	/(perf|video-layers|soak-5min|export-failure|desktop-detect|desktop-detect-webkit)\.spec\.ts/;

// A test whose title carries this tag runs in Chromium only. Each use has the
// reason in a comment next to it: an engine limit, never a defect to hide.
export const CHROMIUM_ONLY = "@chromium-only";

// WebKit reports one style-src-attr violation each time React mounts elements
// that carry a style prop, although the styles are applied (the elements read
// back their widths and positions) and Chromium and Firefox report nothing on
// the same pages. The CSP specs still run in WebKit and drop only this
// directive there; Chromium and Firefox keep the check complete.
export function cspViolations(browserName: string, seen: string[]): string[] {
	return browserName === "webkit"
		? seen.filter((directive) => directive !== "style-src-attr")
		: seen;
}

// The playback clock of the preview is the AudioContext clock (SDD-012 section
// 11.2). The WebKit build of Playwright on Windows has no Web Audio, so the
// preview there cannot play; a scenario that plays skips with this reason.
export async function skipWithoutWebAudio(
	page: Page,
	skip: (condition: boolean, reason: string) => void,
): Promise<void> {
	const has = await page.evaluate(() => typeof AudioContext === "function");
	skip(!has, "this browser build has no Web Audio, so the preview cannot play");
}

// The WebKit build of Playwright on Windows (WinCairo) answers
// MEDIA_ERR_SRC_NOT_SUPPORTED for the VP9 MP4 proxy of SDD-011 section 4.2 even
// though canPlayType says "probably", so a video layer never decodes there. The
// WebKit builds of Linux and macOS (the CI jobs) decode it, and the specs that
// compare video pixels run on them.
export function noVideoDecode(browserName: string): boolean {
	return browserName === "webkit" && process.platform === "win32";
}

export const NO_VIDEO_DECODE_REASON =
	"the WebKit build of Playwright on Windows cannot decode the VP9 MP4 proxy (MEDIA_ERR_SRC_NOT_SUPPORTED)";

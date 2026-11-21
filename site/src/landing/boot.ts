// The tiny first script of a landing page: it waits until the text has painted and the browser is idle, then the
// page loads its three.js and GSAP islands with a dynamic import (design-system section 11.9).
export function reducedMotion(): boolean {
	return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

export function afterPaint(fn: () => void): void {
	const run = (): void => {
		if ("requestIdleCallback" in window) window.requestIdleCallback(fn, { timeout: 1200 });
		else setTimeout(fn, 200);
	};
	if (document.readyState === "complete") requestAnimationFrame(() => requestAnimationFrame(run));
	else window.addEventListener("load", () => requestAnimationFrame(run), { once: true });
}

// The pause control (WCAG 2.2.2, design-system section 11.8): html[data-motion="paused"] freezes every animation
// of a landing page. The choice lives in localStorage; reduced motion counts as paused for the scripts.
export const MOTION_KEY = "stuarty-motion";
export const MOTION_EVENT = "stuarty:motion";

export function motionPaused(): boolean {
	return reducedMotion() || document.documentElement.dataset.motion === "paused";
}

/** Calls `fn` with the new state each time the visitor pauses or resumes the animations. */
export function onMotionChange(fn: (paused: boolean) => void): void {
	document.addEventListener(MOTION_EVENT, () => fn(motionPaused()));
}

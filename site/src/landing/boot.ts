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

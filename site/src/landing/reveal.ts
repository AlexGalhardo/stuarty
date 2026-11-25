/**
 * The entrance of the sections and cards of the landing as the visitor scrolls (epic 95, author review). Browsers with
 * scroll-driven animations run it in CSS alone (`animation-timeline: view()` in home.css); this is the fallback for the
 * others. It hides only what is still below the window when it runs, and shows each element as it enters, so the page
 * stays complete without JavaScript, with reduced motion and with the motion paused.
 */
export const REVEAL_SELECTOR = "main[data-page='home'] [data-reveal]";

export function enhanceReveal(doc: Document): void {
	const win = doc.defaultView;
	if (!win || typeof win.IntersectionObserver !== "function") return;
	if (win.CSS?.supports?.("animation-timeline: view()")) return;
	if (win.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
	if (doc.documentElement.dataset.motion === "paused") return;

	const below = [...doc.querySelectorAll<HTMLElement>(REVEAL_SELECTOR)].filter((el) => el.getBoundingClientRect().top > win.innerHeight);
	if (below.length === 0) return;
	const observer = new win.IntersectionObserver(
		(entries) => {
			for (const entry of entries) {
				if (!entry.isIntersecting) continue;
				(entry.target as HTMLElement).classList.remove("l-reveal-out");
				observer.unobserve(entry.target);
			}
		},
		{ rootMargin: "0px 0px -10% 0px" },
	);
	for (const el of below) {
		el.classList.add("l-reveal-out");
		observer.observe(el);
	}
}

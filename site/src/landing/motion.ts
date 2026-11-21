// Shared motion of the landing pages (design-system section 11.8): letter-by-letter titles, reveals on scroll, SVG
// lines that draw themselves and typed terminals. Every helper checks reduced motion and then shows the final state
// at once (or with a short crossfade): motion never carries information alone.
import { gsap } from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { SplitText } from "gsap/SplitText";
import { reducedMotion } from "./boot";

gsap.registerPlugin(ScrollTrigger, SplitText);

export { gsap, ScrollTrigger, SplitText, reducedMotion };

export const EASE_OUT_EXPO = "expo.out";
export const SCENE_SECONDS = 0.7;
export const REVEAL_SECONDS = 0.5;
export const STAGGER_SECONDS = 0.06;


/**
 * Splits a title into characters and returns a paused timeline that brings them in. Screen readers keep reading
 * the original text: SplitText sets aria-label on the element and hides the pieces.
 */
export function letters(el: Element, options: { delay?: number; y?: number } = {}): gsap.core.Timeline {
	const tl = gsap.timeline({ paused: true, delay: options.delay ?? 0 });
	if (reducedMotion()) {
		tl.fromTo(el, { autoAlpha: 0 }, { autoAlpha: 1, duration: 0.15 });
		return tl;
	}
	// A [data-split-skip] part stays whole: its text is painted by background-clip: text, which does not reach the
	// per-letter boxes SplitText makes, so split it would vanish. It comes in as one piece after the letters.
	const skip = [...el.querySelectorAll("[data-split-skip]")];
	const split = SplitText.create(el, { type: "words,chars", wordsClass: "split-word", aria: "auto", ignore: skip });
	tl.set(el, { autoAlpha: 1 });
	tl.from(split.chars, {
		yPercent: options.y ?? 110,
		rotateX: -70,
		opacity: 0,
		duration: 0.9,
		ease: EASE_OUT_EXPO,
		stagger: 0.022,
		transformOrigin: "50% 100%",
	});
	if (skip.length) tl.from(skip, { opacity: 0, y: 24, duration: 0.9, ease: EASE_OUT_EXPO, clearProps: "transform" }, "-=0.6");
	return tl;
}

/** Fades and lifts children in, one after another. */
export function rise(targets: gsap.TweenTarget, options: { delay?: number; y?: number } = {}): gsap.core.Tween {
	if (reducedMotion()) return gsap.fromTo(targets, { autoAlpha: 0 }, { autoAlpha: 1, duration: 0.15, delay: options.delay });
	return gsap.fromTo(
		targets,
		{ autoAlpha: 0, y: options.y ?? 24 },
		{ autoAlpha: 1, y: 0, duration: REVEAL_SECONDS * 1.4, ease: EASE_OUT_EXPO, stagger: STAGGER_SECONDS * 1.5, delay: options.delay },
	);
}

/** Reveals every `[data-reveal]` group when it scrolls into view (classic page). */
export function revealOnScroll(root: ParentNode = document): void {
	for (const group of root.querySelectorAll<HTMLElement>("[data-reveal]")) {
		const items = group.querySelectorAll<HTMLElement>("[data-reveal-item]");
		const targets = items.length > 0 ? items : [group];
		gsap.set(targets, { autoAlpha: 0 });
		ScrollTrigger.create({
			trigger: group,
			start: "top 82%",
			once: true,
			onEnter: () => rise(targets),
		});
	}
}

/** Prepares SVG paths to draw themselves: returns a tween from hidden to drawn. */
export function drawPaths(paths: SVGPathElement[] | NodeListOf<SVGGeometryElement>, duration = 1.2): gsap.core.Tween {
	const list = [...paths];
	for (const p of list) {
		const len = p.getTotalLength();
		p.style.strokeDasharray = `${len}`;
		p.style.strokeDashoffset = reducedMotion() ? "0" : `${len}`;
	}
	if (reducedMotion()) return gsap.to(list, { duration: 0 });
	return gsap.to(list, { strokeDashoffset: 0, duration, ease: "power2.inOut", stagger: 0.12 });
}

/** Hides the rows of every terminal under `root` until it is typed (never under reduced motion). */
export function prepareTerminals(root: ParentNode): void {
	if (reducedMotion()) return;
	for (const row of root.querySelectorAll<HTMLElement>("[data-terminal] [data-row]")) row.classList.add("t-pending");
}

/**
 * Types a terminal out line by line, like a typewriter. Each row is an element with `data-row` (its kind) holding a
 * `[data-type]` element with the full text already in the HTML; rows wait hidden and each one is typed after the
 * previous, commands and prompts slower than the answers. Returns a paused timeline; under reduced motion it shows
 * every row at once.
 *
 * The rows are hidden with a class, not with GSAP: a tween "from" a hidden state onto a row GSAP had hidden itself
 * ends hidden, which is how the answers (and the whole MCP tab) once stayed invisible.
 */
export function typeLines(terminal: Element): gsap.core.Timeline {
	const tl = gsap.timeline({ paused: true });
	const rows = [...terminal.querySelectorAll<HTMLElement>("[data-row]")];
	if (reducedMotion()) {
		tl.call(() => rows.forEach((r) => r.classList.remove("t-pending")));
		return tl;
	}
	for (const row of rows) {
		const target = row.querySelector<HTMLElement>("[data-type]");
		const text = target?.textContent ?? "";
		const slow = row.dataset.row === "cmd" || row.dataset.row === "prompt";
		const perChar = slow ? 0.028 : 0.006;
		const duration = Math.min(slow ? 1.4 : 0.7, Math.max(0.12, text.length * perChar));
		const counter = { n: 0 };
		tl.call(() => {
			row.classList.remove("t-pending");
			if (target) target.textContent = "";
		});
		if (target) {
			tl.to(counter, {
				n: text.length,
				duration,
				ease: "none",
				onUpdate: () => {
					target.textContent = text.slice(0, Math.round(counter.n));
				},
				onComplete: () => {
					target.textContent = text;
				},
			});
		}
		tl.to({}, { duration: slow ? 0.35 : 0.08 });
	}
	return tl;
}

/** WebGL fallback: the still mascot belongs to the first screen and goes once the reader scrolls past it. */
export function hideStillAfter(trigger: string): void {
	const still = document.querySelector("[data-stage-still]");
	if (!still) return;
	ScrollTrigger.create({
		trigger,
		start: "top 60%",
		onEnter: () => still.toggleAttribute("hidden", true),
		onLeaveBack: () => still.toggleAttribute("hidden", false),
	});
}

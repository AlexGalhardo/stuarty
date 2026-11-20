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
	const split = SplitText.create(el, { type: "words,chars", wordsClass: "split-word", aria: "auto" });
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

/**
 * Types the lines of a terminal. Each line is an element with `data-line` holding its full text already (so the
 * text is in the HTML); the animation hides it and types it back. Returns a timeline.
 */
export function typeLines(terminal: Element, options: { speed?: number } = {}): gsap.core.Timeline {
	const tl = gsap.timeline({ paused: true });
	const lines = [...terminal.querySelectorAll<HTMLElement>("[data-line]")];
	if (reducedMotion()) {
		tl.set(lines, { autoAlpha: 1 });
		return tl;
	}
	const speed = options.speed ?? 0.018;
	gsap.set(lines, { autoAlpha: 0 });
	for (const line of lines) {
		const text = line.textContent ?? "";
		const isCommand = line.dataset.line === "cmd";
		const counter = { n: 0 };
		tl.set(line, { autoAlpha: 1 });
		if (isCommand) {
			line.setAttribute("aria-label", text);
			tl.fromTo(
				counter,
				{ n: 0 },
				{
					n: text.length,
					duration: Math.min(1.6, text.length * speed),
					ease: "none",
					onUpdate: () => {
						line.textContent = text.slice(0, Math.round(counter.n));
					},
					onComplete: () => {
						line.textContent = text;
					},
				},
			);
			tl.to({}, { duration: 0.25 });
		} else {
			tl.from(line, { autoAlpha: 0, y: 4, duration: 0.25 }, "<");
			tl.to({}, { duration: 0.08 });
		}
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

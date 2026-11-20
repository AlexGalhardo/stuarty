// The viewport landing (brainstorming section 5.1): six takes, one at a time, on a large screen. Input: wheel,
// touch swipe, arrows, Page Up/Down, Space, Home/End, the named rail and the URL hash. Focus moves to the heading of
// the take that comes in. Under reduced motion the takes crossfade and the 3D stays on its first frame. Without
// takes mode (a small screen) the page is a normal scroll with reveals.
import { afterPaint, reducedMotion } from "./boot";
import { gsap, letters, revealOnScroll, typeLines } from "./motion";
import type { MascotPose, Stage } from "./stage";
import { enhanceTabs } from "./tabs";

// Where the mascot stands in each take: in the empty band above the content, never over the text.
const POSES: Omit<MascotPose, "opacity">[] = [
	{ x: 0.52, y: 0.02, scale: 0.95, turn: -0.45 },
	{ x: 0.62, y: 0.74, scale: 0.22, turn: -0.5 },
	{ x: 0.72, y: 0.72, scale: 0.22, turn: -0.6 },
	{ x: -0.52, y: 0.72, scale: 0.22, turn: 0.5 },
	{ x: 0.74, y: 0.7, scale: 0.24, turn: -0.55 },
	{ x: 0, y: 0.56, scale: 0.42, turn: 0 },
];

export function start(): void {
	const root = document.querySelector<HTMLElement>("main[data-page='viewport']");
	if (!root) return;
	const reduced = reducedMotion();
	const takes = [...root.querySelectorAll<HTMLElement>("[data-take]")];
	const links = [...root.querySelectorAll<HTMLAnchorElement>("[data-take-link]")];
	const progress = root.querySelector<HTMLElement>("[data-rail-progress]");
	const hint = root.querySelector<HTMLElement>("[data-hint]");
	const typed = new WeakSet<Element>();
	const typePanel = (panel: Element): void => {
		const terminal = panel.querySelector("[data-terminal]");
		if (!terminal || typed.has(terminal)) return;
		typed.add(terminal);
		typeLines(terminal).play();
	};
	enhanceTabs(root, typePanel);

	let stage: Stage | null = null;
	const takesOn = (): boolean => document.documentElement.classList.contains("takes-on");

	afterPaint(() => {
		void import("./stage").then(({ createStage }) => {
			const canvas = document.getElementById("viewport-stage") as HTMLCanvasElement | null;
			if (!canvas) return;
			stage = createStage(canvas);
			if (!stage) return;
			Object.assign(stage.pose, takesOn() ? POSES[current] : { x: 0.62, y: 0.82, scale: 0.3, turn: -0.4 }, { opacity: 1 });
			if (!takesOn() && !reduced) {
				// The plain scroll: the mascot greets the reader in the first take and fades when it scrolls away.
				gsap.to(stage.pose, { opacity: 0, ease: "none", immediateRender: false, scrollTrigger: { trigger: takes[1], start: "top 90%", end: "top 40%", scrub: true } });
			}
			if (reduced) stage.renderOnce();
			else gsap.from(stage.pose, { scale: 0, turn: -2, duration: 1.4, ease: "back.out(1.4)", onComplete: () => stage?.blink() });
		});
	});

	let current = 0;

	if (!takesOn()) {
		// Normal scroll: reveal each take as it enters, type terminals when seen.
		for (const take of takes.slice(1)) take.setAttribute("data-reveal", "");
		for (const el of root.querySelectorAll("[data-in], [data-in-3d]")) el.setAttribute("data-reveal-item", "");
		revealOnScroll(root);
		const panel = root.querySelector("[data-drive] [role='tabpanel']:not([hidden])");
		if (panel) {
			const io = new IntersectionObserver((entries) => {
				if (entries.some((e) => e.isIntersecting)) {
					typePanel(panel);
					io.disconnect();
				}
			});
			io.observe(panel);
		}
		return;
	}

	const setRail = (i: number): void => {
		links.forEach((a, k) => {
			if (k === i) a.setAttribute("aria-current", "step");
			else a.removeAttribute("aria-current");
		});
		if (progress) progress.style.transform = `scaleY(${(i + 1) / takes.length})`;
	};

	const splits = new Map<HTMLElement, gsap.core.Timeline>();
	const show = (i: number, opts: { focus: boolean; instant?: boolean }): void => {
		const prev = takes[current];
		const next = takes[i];
		current = i;
		for (const t of takes) {
			const on = t === next;
			t.toggleAttribute("data-active", on);
			t.toggleAttribute("inert", !on);
			if (!on) t.setAttribute("aria-hidden", "true");
			else t.removeAttribute("aria-hidden");
		}
		setRail(i);
		// The still mascot of the WebGL fallback belongs to the first take.
		document.querySelector("[data-stage-still]")?.toggleAttribute("hidden", i !== 0);
		history.replaceState(null, "", `#${next.id}`);
		if (hint) hint.style.opacity = i === 0 ? "1" : "0";
		// Focus after two frames: the take has just become visible, and the global reduced-motion rule gives
		// every element a tiny transition, visibility included, so the heading is not focusable in this frame.
		if (opts.focus) {
			const heading = next.querySelector<HTMLElement>("h1, h2");
			requestAnimationFrame(() => requestAnimationFrame(() => heading?.focus({ preventScroll: true })));
		}

		const heading = next.querySelector<HTMLElement>("h2[data-in]");
		const items = [...next.querySelectorAll<HTMLElement>("[data-in]")].filter((el) => el !== heading);
		const frames = next.querySelectorAll<HTMLElement>("[data-in-3d]");
		if (opts.instant || reduced) {
			// Opacity only on the takes: hiding the incoming take, even for a frame, would blur the focused heading.
			if (prev && prev !== next) {
				gsap.fromTo(prev, { opacity: 1, visibility: "visible" }, { opacity: 0, duration: reduced ? 0.15 : 0, clearProps: "opacity,visibility" });
			}
			gsap.fromTo(next, { opacity: 0 }, { opacity: 1, duration: reduced ? 0.15 : 0, clearProps: "opacity" });
			if (items.length + frames.length > 0) gsap.set([...items, ...frames], { clearProps: "all" });
		} else {
			const tl = gsap.timeline();
			if (prev && prev !== next) {
				tl.fromTo(prev, { opacity: 1, visibility: "visible" }, { opacity: 0, y: -30, duration: 0.35, ease: "power2.in", clearProps: "opacity,visibility,transform" }, 0);
			}
			tl.fromTo(next, { opacity: 0 }, { opacity: 1, duration: 0.01, clearProps: "opacity" }, 0.25);
			if (heading) {
				// Split once per heading; later visits replay the same letters.
				let split = splits.get(heading);
				if (!split) {
					split = letters(heading);
					splits.set(heading, split);
				}
				const run = split;
				run.pause(0);
				gsap.delayedCall(0.28, () => void run.restart());
			}
			if (items.length > 0) tl.fromTo(items, { opacity: 0, y: 36 }, { opacity: 1, y: 0, duration: 0.9, ease: "expo.out", stagger: 0.07 }, 0.3);
			if (frames.length > 0) tl.fromTo(
				frames,
				{ autoAlpha: 0, rotateY: (k: number) => (k % 2 ? 18 : -22), rotateX: 8, z: -200, y: 60 },
				{ autoAlpha: 1, rotateY: (k: number) => (k % 2 ? 4 : -6), rotateX: 2, z: 0, y: 0, duration: 1.2, ease: "expo.out", stagger: 0.12 },
				0.32,
			);
		}
		if (next.id === "agents") {
			const panel = next.querySelector("[data-drive] [role='tabpanel']:not([hidden])");
			if (panel) setTimeout(() => typePanel(panel), reduced ? 0 : 700);
		}
		if (stage) {
			const pose = { ...POSES[i], opacity: 1 };
			if (reduced) {
				Object.assign(stage.pose, pose);
				stage.renderOnce();
			} else {
				stage.clap();
				stage.stir(0.8);
				stage.setMood(i / (takes.length - 1));
				gsap.to(stage.pose, { ...pose, duration: 1.1, ease: "power3.inOut" });
			}
		}
	};

	let locked = false;
	const go = (i: number, focus: boolean): void => {
		const target = Math.max(0, Math.min(takes.length - 1, i));
		if (target === current || locked) return;
		locked = true;
		show(target, { focus });
		setTimeout(() => (locked = false), reduced ? 250 : 900);
	};

	// The first take from the hash, if any.
	const fromHash = takes.findIndex((t) => `#${t.id}` === location.hash);
	current = fromHash > 0 ? fromHash : 0;
	show(current, { focus: false, instant: true });

	// Wheel: one take per gesture. Trackpads send many small events: they add up until a threshold, then rest.
	let acc = 0;
	let wheelTimer = 0;
	window.addEventListener(
		"wheel",
		(e) => {
			if ((e.target as Element | null)?.closest("[role='dialog']")) return;
			e.preventDefault();
			acc += e.deltaY;
			clearTimeout(wheelTimer);
			wheelTimer = window.setTimeout(() => (acc = 0), 160);
			if (Math.abs(acc) > 60) {
				go(current + Math.sign(acc), false);
				acc = 0;
			}
		},
		{ passive: false },
	);

	// Touch: a vertical swipe of 50 px.
	let touchY: number | null = null;
	window.addEventListener("touchstart", (e) => (touchY = e.touches[0]?.clientY ?? null), { passive: true });
	window.addEventListener(
		"touchend",
		(e) => {
			if (touchY === null) return;
			const dy = touchY - (e.changedTouches[0]?.clientY ?? touchY);
			if (Math.abs(dy) > 50) go(current + Math.sign(dy), false);
			touchY = null;
		},
		{ passive: true },
	);

	// Keyboard. Arrows belong to a tablist or a field when the focus is there.
	window.addEventListener("keydown", (e) => {
		const t = e.target as HTMLElement | null;
		if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey) return;
		if (t?.closest("input, textarea, select, [role='tablist'], [role='dialog'], [contenteditable='true']")) return;
		const space = e.key === " " && !t?.closest("a, button");
		if (e.key === "ArrowDown" || e.key === "PageDown" || (space && !e.shiftKey)) go(current + 1, true);
		else if (e.key === "ArrowUp" || e.key === "PageUp" || (space && e.shiftKey)) go(current - 1, true);
		else if (e.key === "Home") go(0, true);
		else if (e.key === "End") go(takes.length - 1, true);
		else return;
		e.preventDefault();
	});

	// The rail and every in-page link to a take.
	root.addEventListener("click", (e) => {
		const a = (e.target as Element | null)?.closest<HTMLAnchorElement>("a[href^='#']");
		if (!a) return;
		const i = takes.findIndex((t) => `#${t.id}` === a.getAttribute("href"));
		if (i < 0) return;
		e.preventDefault();
		locked = false;
		go(i, true);
	});
	window.addEventListener("hashchange", () => {
		const i = takes.findIndex((t) => `#${t.id}` === location.hash);
		if (i >= 0) go(i, true);
	});

	// Tab moves between the real focusable elements of the active take; a focus that lands in another take (the
	// rail is outside the takes) brings that take in.
	root.addEventListener("focusin", (e) => {
		const take = (e.target as Element | null)?.closest<HTMLElement>("[data-take]");
		if (!take) return;
		const i = takes.indexOf(take);
		if (i >= 0 && i !== current) {
			locked = false;
			go(i, false);
		}
	});

	// Leaving takes mode (the window becomes small) reloads the plain layout.
	const big = window.matchMedia("(min-width: 64rem) and (min-height: 45rem)");
	big.addEventListener("change", () => location.reload());
}

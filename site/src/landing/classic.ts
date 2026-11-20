// Motion of the classic landing (brainstorming section 5.2). GSAP loads with this module; three.js loads after the
// first paint, when the browser is idle. Under reduced motion every scroll-linked effect is off and the page is a
// plain document with the stage frozen on its first frame.
import { afterPaint, reducedMotion } from "./boot";
import { drawPaths, gsap, letters, revealOnScroll, rise, ScrollTrigger, typeLines } from "./motion";
import type { Stage } from "./stage";
import { enhanceTabs } from "./tabs";

export function start(): void {
	const reduced = reducedMotion();
	const root = document.querySelector<HTMLElement>("main[data-page='classic']");
	if (!root) return;
	document.documentElement.classList.add("motion-live");

	const tilt = root.querySelector<HTMLElement>("[data-tilt]");
	// The tilt is set by GSAP from here on (its inline CSS is for the page without script).
	if (tilt && !reduced) {
		tilt.style.transform = "";
		gsap.set(tilt, { rotateY: -14, rotateX: 6, transformPerspective: 1600 });
	}

	// Hero: the title letter by letter, then the lead and the actions.
	const title = root.querySelector("[data-split]");
	if (!reduced && title) {
		const tl = gsap.timeline();
		tl.add(letters(title).play(), 0);
		tl.add(rise(root.querySelectorAll("[data-hero-item]")), 0.35);
		tl.from("[data-tilt]", { autoAlpha: 0, y: 60, rotateY: -30, duration: 1.4, ease: "expo.out" }, 0.2);
	}

	// Tilted product window follows the pointer a little.
	if (tilt && !reduced && window.matchMedia("(pointer: fine)").matches) {
		const rx = gsap.quickTo(tilt, "rotateX", { duration: 0.8, ease: "power3.out" });
		const ry = gsap.quickTo(tilt, "rotateY", { duration: 0.8, ease: "power3.out" });
		window.addEventListener("pointermove", (e) => {
			const x = e.clientX / window.innerWidth - 0.5;
			const y = e.clientY / window.innerHeight - 0.5;
			ry(-14 + x * 10);
			rx(6 - y * 8);
		});
	}

	// Scroll cue: the dot runs down the mouse.
	if (!reduced) gsap.to("[data-cue-dot]", { y: 6, opacity: 0, duration: 1.2, repeat: -1, ease: "power1.in" });

	revealOnScroll(root);

	// Trust icons draw themselves when the grid enters.
	const icons = root.querySelectorAll<SVGPathElement>("[data-trust] [data-draw]");
	if (icons.length > 0) {
		const draw = drawPaths([...icons], 1.4).pause();
		ScrollTrigger.create({ trigger: "[data-trust]", start: "top 80%", once: true, onEnter: () => void draw.play() });
	}

	// Last call to action: letters again, on scroll.
	const ctaTitle = root.querySelector("[data-split-scroll]");
	if (ctaTitle && !reduced) {
		gsap.set(ctaTitle, { autoAlpha: 0 });
		const tl = letters(ctaTitle);
		ScrollTrigger.create({ trigger: ctaTitle, start: "top 85%", once: true, onEnter: () => void tl.play() });
	}

	// Terminals type their lines when their tab is shown and when they first scroll into view.
	const typed = new WeakSet<Element>();
	const typePanel = (panel: Element): void => {
		const terminal = panel.querySelector("[data-terminal]");
		if (!terminal || typed.has(terminal)) return;
		typed.add(terminal);
		typeLines(terminal).play();
	};
	enhanceTabs(root, typePanel);
	const firstPanel = root.querySelector("[data-drive] [role='tabpanel']:not([hidden])");
	if (firstPanel) ScrollTrigger.create({ trigger: firstPanel, start: "top 75%", once: true, onEnter: () => typePanel(firstPanel) });

	const mm = gsap.matchMedia();

	// Screens drift sideways with the scroll.
	mm.add("(prefers-reduced-motion: no-preference)", () => {
		gsap.fromTo(
			"[data-drift]",
			{ x: () => window.innerWidth * 0.1 },
			{
				x: () => -(document.querySelector<HTMLElement>("[data-drift]")?.scrollWidth ?? 0) + window.innerWidth * 0.9,
				ease: "none",
				scrollTrigger: { trigger: "[data-drift]", start: "top bottom", end: "bottom top", scrub: 0.6, invalidateOnRefresh: true },
			},
		);
	});

	afterPaint(() => {
		void import("./stage").then(({ createStage }) => {
			const canvas = document.getElementById("classic-stage") as HTMLCanvasElement | null;
			if (!canvas) return;
			const stage = createStage(canvas);
			if (!stage) return;
			choreograph(stage, root, reduced, mm);
		});
	});
}

/** The mascot's path down the page: beside the hero, centre stage while the tools merge into it, then aside. */
function choreograph(stage: Stage, root: HTMLElement, reduced: boolean, mm: gsap.MatchMedia): void {
	const wide = window.matchMedia("(min-width: 64rem)").matches;
	Object.assign(stage.pose, wide ? { x: 0.72, y: 0.62, scale: 0.42, turn: -0.5, opacity: 1 } : { x: 0.66, y: 0.84, scale: 0.26, turn: -0.4, opacity: 1 });
	if (reduced) {
		stage.renderOnce();
		return;
	}
	gsap.from(stage.pose, { scale: 0, duration: 1.2, ease: "back.out(1.6)", delay: 0.2, onComplete: () => stage.blink() });

	// Scroll velocity stirs the background.
	ScrollTrigger.create({
		trigger: root,
		start: 0,
		end: "max",
		onUpdate: (self) => stage.stir(Math.min(1, Math.abs(self.getVelocity()) / 3000)),
	});

	// Small screens: no pin; the mascot fades once the hero has scrolled away.
	mm.add("(max-width: 47.99rem) and (prefers-reduced-motion: no-preference)", () => {
		gsap.to(stage.pose, { opacity: 0, ease: "none", scrollTrigger: { trigger: "#problem-title", start: "top 90%", end: "top 40%", scrub: true } });
	});

	// The problem: pinned on wide screens; the five tools converge on the mascot, which snaps its clapper.
	mm.add("(min-width: 48rem) and (prefers-reduced-motion: no-preference)", () => {
		const tools = gsap.utils.toArray<HTMLElement>("[data-tool]");
		const answer = root.querySelector("[data-answer]");
		const slot = root.querySelector<HTMLElement>("[data-mascot-slot]");
		gsap.set(answer, { autoAlpha: 0, y: 20 });
		// The slot is measured when the pin starts: the mascot stands in it and the tools converge on it.
		const slotCenter = (): { x: number; y: number } => {
			const r = slot?.getBoundingClientRect();
			const pinTop = root.querySelector<HTMLElement>("[data-problem-pin]")?.getBoundingClientRect().top ?? 0;
			if (!r) return { x: window.innerWidth / 2, y: window.innerHeight * 0.7 };
			return { x: r.left + r.width / 2, y: r.top - pinTop + r.height / 2 };
		};
		const toolDelta = (el: HTMLElement): { x: number; y: number } => {
			const c = slotCenter();
			const r = el.getBoundingClientRect();
			const pinTop = root.querySelector<HTMLElement>("[data-problem-pin]")?.getBoundingClientRect().top ?? 0;
			return { x: c.x - (r.left + r.width / 2), y: c.y - (r.top - pinTop + r.height / 2) };
		};
		const scatter = [
			{ x: -380, y: 30, r: -14 },
			{ x: -170, y: 110, r: 8 },
			{ x: 0, y: -10, r: -4 },
			{ x: 190, y: 120, r: 11 },
			{ x: 380, y: 20, r: -9 },
		];
		const tl = gsap.timeline({
			scrollTrigger: {
				trigger: "[data-problem]",
				start: "top top",
				end: "+=140%",
				pin: "[data-problem-pin]",
				scrub: 0.8,
				invalidateOnRefresh: true,
				onLeave: () => stage.clap(),
				onEnterBack: () => stage.blink(),
			},
		});
		const slotPose = (): { x: number; y: number } => {
			const c = slotCenter();
			return { x: (c.x / window.innerWidth) * 2 - 1, y: -((c.y / window.innerHeight) * 2 - 1) };
		};
		tl.fromTo(
			tools,
			{ x: (i: number) => scatter[i]?.x ?? 0, y: (i: number) => scatter[i]?.y ?? 0, rotate: (i: number) => scatter[i]?.r ?? 0 },
			{ x: (i: number) => (scatter[i]?.x ?? 0) * 1.15, y: (i: number) => (scatter[i]?.y ?? 0) + 20, duration: 0.3 },
		)
			.to(stage.pose, { x: () => slotPose().x, y: () => slotPose().y, scale: 0.55, turn: 0, duration: 0.5, ease: "power2.inOut" }, 0)
			.to(
				tools,
				{
					x: (_i: number, el: HTMLElement) => `+=${toolDelta(el).x}`,
					y: (_i: number, el: HTMLElement) => `+=${toolDelta(el).y}`,
					scale: 0.2,
					autoAlpha: 0,
					rotate: 0,
					duration: 0.45,
					ease: "power3.in",
					stagger: 0.03,
				},
				0.35,
			)
			.add(() => stage.clap(), 0.8)
			.to(answer, { autoAlpha: 1, y: 0, duration: 0.25 }, 0.8)
			.to(stage.pose, { scale: 0.48, duration: 0.2 }, 0.8);

		// After the problem, the mascot steps aside and fades behind the content.
		gsap.to(stage.pose, {
			x: 0.8,
			y: 0.7,
			scale: 0.3,
			opacity: 0,
			ease: "none",
			scrollTrigger: { trigger: "#features-title", start: "top 90%", end: "top 30%", scrub: 0.8 },
		});
		// It comes back for the last call to action.
		gsap.fromTo(
			stage.pose,
			{ opacity: 0 },
			{
				x: 0,
				y: 0.55,
				scale: 0.45,
				opacity: 1,
				turn: 0,
				immediateRender: false,
				ease: "none",
				scrollTrigger: { trigger: "#cta-title", start: "top 95%", end: "top 45%", scrub: 0.8, onLeave: () => stage.clap() },
			},
		);
	});
}

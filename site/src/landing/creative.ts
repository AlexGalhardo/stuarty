// The creative landing (brainstorming section 5.3): a pinned editor where the scroll moves the playhead. One
// ScrollTrigger maps the scroll to a progress from 0 to 1; everything else (the lane, the timecode, the preview, the
// agent cursor, the MCP log, the collapse at the end) is drawn from that one number, so scrolling back plays it in
// reverse. Timeline mode is off on small screens and under reduced motion: the clips are a plain list.
import { afterPaint, reducedMotion } from "./boot";
import { gsap, letters, revealOnScroll, rise, ScrollTrigger, typeLines } from "./motion";
import type { Stage } from "./stage";
import { enhanceTabs } from "./tabs";

const FPS = 30;
const SECONDS = 60;

function timecode(progress: number): string {
	const frames = Math.round(progress * SECONDS * FPS);
	const s = Math.floor(frames / FPS);
	const f = frames % FPS;
	return `00:${String(s).padStart(2, "0")}:${String(f).padStart(2, "0")}`;
}

export function start(): void {
	const root = document.querySelector<HTMLElement>("main[data-page='creative']");
	if (!root) return;
	document.documentElement.classList.add("motion-live");
	const reduced = reducedMotion();
	const timelineOn = document.documentElement.classList.contains("timeline-on");

	const title = root.querySelector("[data-split]");
	if (title && !reduced) {
		const tl = gsap.timeline();
		tl.add(letters(title).play(), 0.1);
		tl.add(rise(root.querySelectorAll("[data-hero-item]")), 0.45);
	}

	revealOnScroll(root);
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

	setupCli(root);

	let stage: Stage | null = null;
	afterPaint(() => {
		void import("./stage").then(({ createStage }) => {
			const canvas = document.getElementById("creative-stage") as HTMLCanvasElement | null;
			if (!canvas) return;
			stage = createStage(canvas);
			if (!stage) return;
			// The mascot waits off stage until the export; in the plain layout it greets the reader in the intro.
			// The mascot greets the reader beside the intro, then steps off stage while the timeline plays and comes
			// back at the export.
			Object.assign(stage.pose, timelineOn ? { x: 0.62, y: 0.12, scale: 0.62, turn: -0.5, opacity: 1 } : { x: 0.66, y: 0.84, scale: 0.26, turn: -0.4, opacity: 1 });
			if (!reduced) {
				gsap.from(stage.pose, { scale: 0, turn: -2.5, duration: 1.4, ease: "back.out(1.4)", onComplete: () => stage?.blink() });
				gsap.to(stage.pose, {
					opacity: 0,
					y: timelineOn ? 0.6 : 0.8,
					scale: 0.2,
					ease: "none",
					immediateRender: false,
					scrollTrigger: { trigger: "#timeline", start: "top 85%", end: "top 20%", scrub: true },
				});
			}
			if (reduced) {
				ScrollTrigger.create({
					trigger: "#timeline",
					start: "top 60%",
					onEnter: () => {
						if (!stage) return;
						stage.pose.opacity = 0;
						stage.renderOnce();
					},
					onLeaveBack: () => {
						if (!stage) return;
						stage.pose.opacity = 1;
						stage.renderOnce();
					},
				});
			}
			stage.renderOnce();
		});
	});

	if (!timelineOn) {
		const items = root.querySelectorAll<HTMLElement>("[data-preview-item]");
		for (const item of items) {
			item.setAttribute("data-reveal", "");
			for (const child of item.children) child.setAttribute("data-reveal-item", "");
		}
		revealOnScroll(root);
		return;
	}

	const lane = root.querySelector<HTMLElement>("[data-lane]");
	const lanes = root.querySelector<HTMLElement>(".editor-lanes");
	const tc = root.querySelector<HTMLElement>("[data-timecode]");
	const previews = [...root.querySelectorAll<HTMLElement>("[data-preview-item]")];
	const clipEls = [...root.querySelectorAll<HTMLElement>("[data-clip]")];
	const logs = [...root.querySelectorAll<HTMLElement>("[data-log-at]")];
	const caption = root.querySelector<HTMLElement>("[data-caption-clip]");
	const cursor = root.querySelector<HTMLElement>("[data-agent-cursor]");
	const outro = root.querySelector<HTMLElement>("[data-outro]");
	const top = root.querySelector<HTMLElement>(".editor-top");
	const bottom = root.querySelector<HTMLElement>(".editor-bottom");
	const exportBtn = root.querySelector<HTMLElement>("[data-export]");
	if (!lane || !lanes || !cursor) return;

	const moveCursorX = gsap.quickTo(cursor, "x", { duration: 0.6, ease: "power3.out" });
	const moveCursorY = gsap.quickTo(cursor, "y", { duration: 0.6, ease: "power3.out" });

	let active = -1;
	let exported = false;
	const setActive = (i: number): void => {
		if (i === active) return;
		const prev = previews[active];
		const next = previews[i];
		active = i;
		clipEls.forEach((c, k) => c.toggleAttribute("data-active", k === i));
		if (prev) {
			gsap.to(prev, { autoAlpha: 0, y: -24, duration: 0.35, ease: "power2.in", onComplete: () => prev.removeAttribute("data-active") });
		}
		if (next) {
			next.setAttribute("data-active", "");
			gsap.fromTo(next.querySelector(".preview-text"), { autoAlpha: 0, y: 30 }, { autoAlpha: 1, y: 0, duration: 0.8, ease: "expo.out", delay: 0.15 });
			gsap.fromTo(
				next.querySelector(".preview-media"),
				{ autoAlpha: 0, rotateY: -18, z: -160, transformPerspective: 1400 },
				{ autoAlpha: 1, rotateY: 0, z: 0, duration: 1, ease: "expo.out", delay: 0.2 },
			);
			gsap.set(next, { autoAlpha: 1, y: 0 });
		}
		stage?.blink();
	};

	const laneWidth = (): number => lane.offsetWidth;

	const render = (p: number): void => {
		const run = Math.min(1, p / 0.88);
		gsap.set(lane, { x: -run * laneWidth() });
		if (tc) tc.textContent = timecode(run);
		setActive(Math.min(previews.length - 1, Math.floor(run * previews.length)));
		for (const l of logs) l.toggleAttribute("data-on", run >= Number(l.dataset.logAt));

		// The agent adds a caption between 30% and 40% of the run, and trims the third clip around 50%.
		if (caption) gsap.set(caption, { scaleX: gsap.utils.clamp(0, 1, (run - 0.3) / 0.08) });
		const trim = gsap.utils.clamp(0, 1, (run - 0.46) / 0.06);
		if (clipEls[2]) gsap.set(clipEls[2], { scaleX: 1 - trim * 0.18 });

		// The cursor follows its work: the caption, then the trim edge, then the export button.
		const lanesBox = lanes.getBoundingClientRect();
		let target: HTMLElement | null = clipEls[Math.min(clipEls.length - 1, Math.floor(run * clipEls.length))] ?? null;
		if (run > 0.28 && run < 0.42) target = caption;
		if (run >= 0.42 && run < 0.56) target = clipEls[2] ?? null;
		if (target) {
			const r = target.getBoundingClientRect();
			const edge = run >= 0.42 && run < 0.56 ? r.right - 8 : r.left + r.width * 0.3;
			moveCursorX(Math.max(16, Math.min(lanesBox.width - 140, edge - lanesBox.left)));
			moveCursorY(r.top - lanesBox.top - 14);
		}

		// The export: past 88% the editor folds into the mascot and the outro shows.
		const out = gsap.utils.clamp(0, 1, (p - 0.88) / 0.1);
		gsap.set(bottom, { autoAlpha: 1 - out, scaleY: 1 - out * 0.6, transformOrigin: "50% 100%" });
		gsap.set(top, { autoAlpha: 1 - out, scale: 1 - out * 0.12 });
		if (exportBtn) exportBtn.textContent = out > 0 ? "Exported" : "Export";
		gsap.set(outro, { autoAlpha: gsap.utils.clamp(0, 1, (out - 0.5) / 0.5) });
		if (stage && p > 0) {
			stage.setMood(run);
			if (!reduced) {
				stage.pose.opacity = out;
				stage.pose.scale = out * 0.7;
				stage.pose.x = 0;
				stage.pose.y = 0.1;
				stage.pose.turn = (1 - out) * 3;
			}
			if (out >= 1 && !exported) {
				exported = true;
				stage.clap();
			} else if (out < 0.5) exported = false;
		}
	};

	ScrollTrigger.create({
		trigger: "[data-editor]",
		start: "top top",
		end: "+=520%",
		pin: "[data-editor-pin]",
		scrub: true,
		onUpdate: (self) => {
			render(self.progress);
			stage?.stir(Math.min(1, Math.abs(self.getVelocity()) / 2500));
		},
		onRefresh: (self) => render(self.progress),
	});
	render(0);
}

/** The command line at the end: a few real answers, nothing sent anywhere. */
function setupCli(root: HTMLElement): void {
	const form = root.querySelector<HTMLFormElement>("[data-cli]");
	const out = root.querySelector<HTMLElement>("[data-cli-out]");
	const input = form?.querySelector<HTMLInputElement>("input");
	if (!form || !out || !input) return;
	const install = form.dataset.install ?? "";
	const quick = form.dataset.quick ?? "";
	const print = (lines: string[], kind: "cmd" | "out" = "out"): void => {
		for (const text of lines) {
			const p = document.createElement("p");
			p.className = kind === "cmd" ? "text-white" : "t-out";
			p.style.whiteSpace = "pre-wrap";
			p.textContent = text;
			out.append(p);
		}
		out.scrollTop = out.scrollHeight;
	};
	const answers: Record<string, () => string[]> = {
		help: () => ["Commands of this page:", "  help       this list", "  features   what Stuarty does", "  install    start the latest release with Docker", "  clear      empty the screen", "The real binary has many more: run stuarty --help."],
		features: () => [
			"create    a guided brief, scripts with revisions, copy and blog posts",
			"edit      a video editor (captions, cuts, colour, keyframes) and an image editor",
			"manage    projects as productions, a library of assets, prompts, skills and memory",
			"publish   a calendar for your own accounts; every post waits for your approval",
			"agents    everything above through MCP, the CLI and the HTTP API",
		],
		install: () => ["Start the latest release with Docker:", install, `Then read the quick start: ${location.origin}${quick}`],
	};
	form.addEventListener("submit", (e) => {
		e.preventDefault();
		const cmd = input.value.trim().replace(/^stuarty\s+/, "");
		input.value = "";
		if (!cmd) return;
		print([`$ ${cmd}`], "cmd");
		if (cmd === "clear") {
			out.replaceChildren();
			return;
		}
		const answer = answers[cmd];
		print(answer ? answer() : [`stuarty: no command "${cmd}" on this page. Try help.`]);
	});
}

// The creative landing (brainstorming section 5.3): a pinned editor where the scroll moves the playhead. One
// ScrollTrigger maps the scroll to a progress from 0 to 1; everything else (the lane, the timecode, the preview, the
// agent cursor, the MCP log, the collapse at the end) is drawn from that one number, so scrolling back plays it in
// reverse. Timeline mode is off on small screens and under reduced motion: the clips are a plain list.
import { afterPaint, reducedMotion } from "./boot";
import { gsap, hideStillAfter, revealOnScroll, ScrollTrigger, prepareTerminals, typeLines } from "./motion";
import type { Stage } from "./stage";
import { enhanceTabs } from "./tabs";
import { mountTimeline } from "./timeline";

export function start(): void {
	const root = document.querySelector<HTMLElement>("main[data-page='creative']");
	if (!root) return;
	const reduced = reducedMotion();
	const timelineOn = document.documentElement.classList.contains("timeline-on");

	revealOnScroll(root);
	const typed = new WeakSet<Element>();
	const typePanel = (panel: Element): void => {
		const terminal = panel.querySelector("[data-terminal]");
		if (!terminal || typed.has(terminal)) return;
		typed.add(terminal);
		typeLines(terminal).play();
	};
	prepareTerminals(root);
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
			if (!stage) {
				hideStillAfter("#timeline");
				return;
			}
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

	const render = mountTimeline(root, { reduced, stage: () => stage });
	if (!render) return;

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

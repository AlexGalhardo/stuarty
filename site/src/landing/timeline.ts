// The timeline editor shared by the creative and the viewport landings: a playhead over tracks of clips, the
// preview of the clip under it, an agent cursor that adds a caption and trims a clip, the MCP log, and the export
// that folds the editor away. Everything is drawn from one progress value from 0 to 1, so the caller decides what
// drives it (the scroll in the creative page, the wheel and the keys in the viewport page).
import { gsap } from "./motion";
import type { Stage } from "./stage";

const FPS = 30;
const SECONDS = 60;

function timecode(progress: number): string {
	const frames = Math.round(progress * SECONDS * FPS);
	const s = Math.floor(frames / FPS);
	const f = frames % FPS;
	return `00:${String(s).padStart(2, "0")}:${String(f).padStart(2, "0")}`;
}

/** Mounts the timeline found under `root`; returns its render function, or null when its markup is missing. */
export function mountTimeline(
	root: ParentNode,
	options: { reduced: boolean; stage?: () => Stage | null; /** The export folds the editor into the 3D mascot. */ mascot?: boolean },
): ((p: number) => void) | null {
	const { reduced } = options;
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
	if (!lane || !lanes || !cursor) return null;

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
		options.stage?.()?.blink();
	};

	const laneWidth = (): number => lane.offsetWidth;

	const render = (p: number): void => {
		const run = Math.min(1, p / 0.88);
		gsap.set(lane, { x: -run * laneWidth() });
		if (tc) tc.textContent = timecode(run);
		setActive(Math.min(previews.length - 1, Math.floor(run * previews.length)));
		for (const l of logs) l.toggleAttribute("data-on", run >= Number(l.dataset.logAt));

		// The agent adds a caption while the playhead crosses the clip under it, then trims the clip marked
		// data-trim. Their places on the run come from the markup (the fraction of the lane where each starts).
		const captionAt = Number(caption?.dataset.at ?? 0.3);
		const trimEl = root.querySelector<HTMLElement>("[data-trim]");
		const trimAt = Number(trimEl?.dataset.at ?? 0.46);
		if (caption) gsap.set(caption, { scaleX: gsap.utils.clamp(0, 1, (run - captionAt) / 0.08) });
		const trim = gsap.utils.clamp(0, 1, (run - trimAt) / 0.06);
		if (trimEl) gsap.set(trimEl, { scaleX: 1 - trim * 0.18 });

		// The cursor follows its work: the caption, then the trim edge, then the clip under the playhead.
		const lanesBox = lanes.getBoundingClientRect();
		let target: HTMLElement | null = clipEls[Math.min(clipEls.length - 1, Math.floor(run * clipEls.length))] ?? null;
		const onCaption = run > captionAt - 0.02 && run < captionAt + 0.12;
		const onTrim = run >= trimAt - 0.04 && run < trimAt + 0.1;
		if (onCaption) target = caption;
		if (onTrim) target = trimEl;
		if (target) {
			const r = target.getBoundingClientRect();
			const edge = onTrim ? r.right - 8 : r.left + r.width * 0.3;
			moveCursorX(Math.max(16, Math.min(lanesBox.width - 140, edge - lanesBox.left)));
			moveCursorY(r.top - lanesBox.top - 14);
		}

		// The export: past 88% the editor folds into the mascot and the outro shows.
		const out = gsap.utils.clamp(0, 1, (p - 0.88) / 0.1);
		gsap.set(bottom, { autoAlpha: 1 - out, scaleY: 1 - out * 0.6, transformOrigin: "50% 100%" });
		gsap.set(top, { autoAlpha: 1 - out, scale: 1 - out * 0.12 });
		if (exportBtn) exportBtn.textContent = out > 0 ? "Exported" : "Export";
		gsap.set(outro, { autoAlpha: gsap.utils.clamp(0, 1, (out - 0.5) / 0.5) });
		const stage = options.stage?.() ?? null;
		if (stage && p > 0) {
			stage.setMood(run);
			if (!reduced && options.mascot !== false) {
				stage.pose.opacity = out;
				stage.pose.scale = out * 0.7;
				stage.pose.x = 0;
				stage.pose.y = 0.1;
				stage.pose.turn = (1 - out) * 3;
			}
			if (out >= 1 && !exported && options.mascot !== false) {
				exported = true;
				stage.clap();
			} else if (out < 0.5) exported = false;
		}
	};
	return render;
}

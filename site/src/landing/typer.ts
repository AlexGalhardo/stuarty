// The typewriter of the hero title (src/components/landing/HeroTyper.astro). Types each phrase, holds it 3 s, erases
// it and types the next, forever. Under reduced motion it does nothing: the first phrase stays as it is.
import { reducedMotion } from "./boot";

type Segment = { text: string; tone?: "orange" | "pink" | "green" };

const TYPE_MS = 42;
const ERASE_MS = 18;
const HOLD_MS = 3000;
const GAP_MS = 450;

/** The first `n` characters of a phrase, as text nodes and accent spans, then the caret. */
function draw(live: HTMLElement, phrase: Segment[], n: number): void {
	const nodes: Node[] = [];
	let left = n;
	for (const s of phrase) {
		if (left <= 0) break;
		const part = s.text.slice(0, left);
		left -= part.length;
		if (s.tone) {
			const span = document.createElement("span");
			span.className = `typer-${s.tone}`;
			span.textContent = part;
			nodes.push(span);
		} else nodes.push(document.createTextNode(part));
	}
	const caret = document.createElement("span");
	caret.className = "typer-caret";
	nodes.push(caret);
	live.replaceChildren(...nodes);
}

export function mountTyper(): void {
	if (reducedMotion()) return;
	const title = document.querySelector<HTMLElement>("[data-typer]");
	const live = title?.querySelector<HTMLElement>("[data-typer-live]");
	if (!title || !live) return;
	let phrases: Segment[][];
	try {
		phrases = JSON.parse(title.dataset.typer ?? "[]") as Segment[][];
	} catch {
		return;
	}
	if (phrases.length === 0) return;
	const lengths = phrases.map((p) => p.reduce((sum, s) => sum + s.text.length, 0));

	// The first phrase is already on screen: hold it, then erase it and go on. The position follows the clock, not
	// the number of frames, so a busy main thread (the WebGL stage) slows nothing down, it only skips letters.
	let index = 0;
	let phase: "hold" | "erase" | "gap" | "type" = "hold";
	let since = performance.now();
	let shown = -1;
	title.classList.add("is-typing");
	const show = (n: number): void => {
		if (n === shown) return;
		shown = n;
		draw(live, phrases[index] ?? [], n);
	};
	show(lengths[0] ?? 0);

	const frame = (now: number): void => {
		const len = lengths[index] ?? 0;
		const t = now - since;
		if (phase === "hold") {
			if (t >= HOLD_MS) [phase, since] = ["erase", now];
		} else if (phase === "erase") {
			const n = Math.max(0, len - Math.floor(t / ERASE_MS));
			show(n);
			if (n === 0) [phase, since] = ["gap", now];
		} else if (phase === "gap") {
			if (t >= GAP_MS) {
				index = (index + 1) % phrases.length;
				shown = -1;
				[phase, since] = ["type", now];
			}
		} else {
			const n = Math.min(lengths[index] ?? 0, Math.floor(t / TYPE_MS));
			show(n);
			if (n >= (lengths[index] ?? 0)) [phase, since] = ["hold", now];
		}
		requestAnimationFrame(frame);
	};
	requestAnimationFrame(frame);
}

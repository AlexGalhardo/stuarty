// The scroll of the teleprompter (SDD-025 section 20.3): the text moves up at the speed the person set, in pixels a
// second, until the end of the script. It is a pure step, so the speed is tested without a screen; the page calls it
// once for every frame it draws.
export interface Scrolled {
	offset: number;
	// The text reached its end: the scroll stops there.
	done: boolean;
}

export function advance(
	offset: number,
	elapsedMs: number,
	speed: number,
	limit: number,
): Scrolled {
	const next = offset + (Math.max(0, elapsedMs) / 1000) * speed;
	const clamped = Math.min(Math.max(0, limit), Math.max(0, next));
	return { offset: clamped, done: clamped >= limit };
}

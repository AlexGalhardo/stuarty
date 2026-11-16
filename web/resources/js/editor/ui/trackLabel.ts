// The short name of a track without a name of its own: V1, V2 for the visual
// tracks and A1, A2 for the audio ones, counted from the bottom of the stack as
// in a video editor. `order` is the ordered ids, top first (selectOrderedTrackIds).
export function trackLabel(
	id: string,
	order: readonly string[],
	kindOf: (id: string) => "audio" | "visual",
): string {
	const kind = kindOf(id);
	const same = order.filter((other) => kindOf(other) === kind);
	const position = same.length - same.indexOf(id);
	return `${kind === "audio" ? "A" : "V"}${position}`;
}

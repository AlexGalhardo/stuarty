// Double click on a composition, or its menu entry: the timeline it uses opens
// in the editor in place of this one, with the id of this one in `from`, which
// the top bar shows as a way back (SDD-012 section 7.4). The edits of the
// nested timeline reach this one through nested.updated.
export function openComposition(fromTimeline: string, timeline: string): void {
	const target = encodeURIComponent(timeline);
	const from = encodeURIComponent(fromTimeline);
	window.location.assign(`/editor/${target}?from=${from}`);
}

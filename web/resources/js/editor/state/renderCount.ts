// Render counts of the timeline items for the profile of SDD-012 section 19.2
// (`window.__vv.renders`, only under ?debug=1). A component calls
// `countRender` in its body; without the hook the call is one property read.

export function countRender(name: string): void {
	const counts =
		typeof window === "undefined" ? undefined : window.__vv?.renderCounts;
	if (counts !== undefined) counts[name] = (counts[name] ?? 0) + 1;
}

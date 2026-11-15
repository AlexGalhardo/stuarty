import type { ReactNode } from "react";

// The safe-area guide (SDD-012 section 11.6): the rectangle `[x, y, w, h]` of
// the plan window in project pixels, as a dashed outline with the band outside
// it dimmed. The numbers are the core's; the percentages are never here.
export default function SafeArea({
	rect,
	scale,
}: {
	rect: readonly [number, number, number, number];
	scale: number;
}): ReactNode {
	const [x, y, w, h] = rect;
	return (
		<div
			aria-hidden="true"
			data-testid="safe-area"
			className="pointer-events-none absolute border border-dashed border-ed-handle shadow-[0_0_0_100vmax_rgb(0_0_0/0.4)]"
			style={{
				left: x * scale,
				top: y * scale,
				width: w * scale,
				height: h * scale,
			}}
		/>
	);
}

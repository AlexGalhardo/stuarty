import type { ReactNode } from "react";

import type { LayerGeometry } from "../types";

// The box of a selected item at the playhead (SDD-012 section 11.6), drawn from
// the geometry of the plan layer. It sits over the canvas; the picture is not
// touched. `scale` is screen pixels per project pixel.
export default function SelectionBox({
	geometry,
	scale,
}: {
	geometry: LayerGeometry;
	scale: number;
}): ReactNode {
	return (
		<div
			aria-hidden="true"
			data-testid="selection-box"
			data-item={geometry.item}
			className="pointer-events-none absolute border-2 border-ed-handle"
			style={{
				left: (geometry.cx - geometry.w / 2) * scale,
				top: (geometry.cy - geometry.h / 2) * scale,
				width: geometry.w * scale,
				height: geometry.h * scale,
				transform: `rotate(${geometry.rotation}deg)`,
			}}
		/>
	);
}

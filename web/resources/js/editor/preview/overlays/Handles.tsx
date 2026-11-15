import type { ReactNode } from "react";

import type { HandleKind, LayerGeometry, TransformPort } from "../types";

const CORNERS = [
	"left-0 top-0 -translate-x-1/2 -translate-y-1/2 cursor-nwse-resize",
	"right-0 top-0 translate-x-1/2 -translate-y-1/2 cursor-nesw-resize",
	"left-0 bottom-0 -translate-x-1/2 translate-y-1/2 cursor-nesw-resize",
	"right-0 bottom-0 translate-x-1/2 translate-y-1/2 cursor-nwse-resize",
] as const;

// The transform handles of the selected item (SDD-012 sections 8.2 and 11.6):
// four corners for a uniform scale, one rotation handle above the box and the
// body for a move. `geometry` is the box to draw, the gesture's own while one
// runs. They are editor chrome: never part of the picture.
export default function Handles({
	geometry,
	scale,
	port,
	onPreview,
}: {
	geometry: LayerGeometry;
	scale: number;
	port: TransformPort;
	onPreview(geometry: LayerGeometry | null): void;
}): ReactNode {
	const begin =
		(kind: HandleKind) =>
		(event: React.PointerEvent<HTMLElement>): void => {
			if (event.button !== 0) return;
			event.stopPropagation();
			const overlay = event.currentTarget.closest<HTMLElement>(
				"[data-testid=canvas-overlay]",
			);
			if (overlay === null) return;
			port.begin({
				event,
				kind,
				itemId: geometry.item,
				start: geometry,
				scale,
				overlay,
				onPreview,
			});
		};
	return (
		<div
			aria-hidden="true"
			data-testid="transform-handles"
			data-item={geometry.item}
			className="absolute"
			style={{
				left: (geometry.cx - geometry.w / 2) * scale,
				top: (geometry.cy - geometry.h / 2) * scale,
				width: geometry.w * scale,
				height: geometry.h * scale,
				transform: `rotate(${geometry.rotation}deg)`,
			}}
		>
			<div
				data-handle="body"
				className="absolute inset-0 cursor-move"
				onPointerDown={begin("body")}
			/>
			{CORNERS.map((place) => (
				<div
					key={place}
					data-handle="scale"
					className={`absolute size-3 rounded-sm border border-ed-handle bg-white ${place}`}
					onPointerDown={begin("scale")}
				/>
			))}
			<div className="pointer-events-none absolute left-1/2 top-0 h-6 w-px -translate-x-1/2 -translate-y-full bg-ed-handle" />
			<div
				data-handle="rotate"
				className="absolute left-1/2 top-0 size-3 -translate-x-1/2 -translate-y-[calc(100%+24px)] cursor-grab rounded-full border border-ed-handle bg-white"
				onPointerDown={begin("rotate")}
			/>
		</div>
	);
}
